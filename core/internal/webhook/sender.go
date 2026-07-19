package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
	"github.com/kienbui1995/magic/core/internal/tracing"
)

// retrySchedule defines wait duration before each retry attempt (index = attempt number - 1).
var retrySchedule = []time.Duration{
	30 * time.Second,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	8 * time.Hour,
}

const maxAttempts = 5

// Sender processes pending WebhookDelivery records from the store every 5s.
type Sender struct {
	store       store.Store
	client      *http.Client
	stop        chan struct{}
	// validateURL is the SSRF guard applied before each delivery attempt.
	// Tests may replace this with a no-op to reach a local httptest server.
	validateURL func(rawURL string) error
}

func newSender(s store.Store) *Sender {
	return &Sender{
		store:       s,
		client:      &http.Client{Timeout: 10 * time.Second},
		stop:        make(chan struct{}),
		validateURL: validateDeliveryURL,
	}
}

func (s *Sender) start() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.processQueue()
			case <-s.stop:
				return
			}
		}
	}()
}

// Stop shuts down the sender goroutine.
func (s *Sender) Stop() {
	close(s.stop)
}

func (s *Sender) processQueue() {
	// TODO(ctx): tie to sender lifecycle once API accepts ctx.
	ctx := context.TODO()
	deliveries := s.store.ListPendingWebhookDeliveries(ctx)
	for _, d := range deliveries {
		// Skip deliveries not yet ready for retry
		if d.NextRetry != nil && time.Now().Before(*d.NextRetry) {
			continue
		}
		hook, err := s.store.GetWebhook(ctx, d.WebhookID)
		if err != nil {
			// Webhook deleted — mark dead
			s.markDead(d)
			continue
		}
		s.deliver(d, hook)
	}
}

func (s *Sender) deliver(d *protocol.WebhookDelivery, hook *protocol.Webhook) {
	// TODO(ctx): propagate from event bus once delivery dispatch carries ctx.
	ctx := context.TODO()
	ctx, span := tracing.StartSpan(ctx, "webhook.Deliver")
	defer span.End()
	span.SetAttr("webhook.id", hook.ID)
	span.SetAttr("webhook.url", hook.URL)
	span.SetAttr("webhook.event_type", d.EventType)
	span.SetAttr("delivery.attempt", d.Attempts+1)

	// SSRF defense-in-depth: validate URL before delivery
	if err := s.validateURL(hook.URL); err != nil {
		span.SetError(err)
		log.Printf("[webhook] delivery %s blocked: %v", d.ID, err)
		s.markDead(d)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader([]byte(d.Payload)))
	if err != nil {
		span.SetError(err)
		s.markFailed(d)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MagiC-Event", d.EventType)
	req.Header.Set("X-MagiC-Delivery", d.ID)

	if hook.Secret != "" {
		sig := computeHMAC(hook.Secret, d.Payload)
		req.Header.Set("X-MagiC-Signature", "sha256="+sig)
	}

	start := time.Now()
	resp, err := s.client.Do(req)
	monitor.MetricWebhookDeliveryDuration.Observe(time.Since(start).Seconds())

	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			resp.Body.Close()
		}
		span.SetAttr("http.status_code", statusCode)
		if err != nil {
			span.SetError(err)
		}
		log.Printf("[webhook] delivery %s failed (attempt %d): status=%d err=%v",
			d.ID, d.Attempts+1, statusCode, err)
		monitor.MetricWebhookDeliveriesTotal.WithLabelValues("failed").Inc()
		s.markFailed(d)
		return
	}
	span.SetAttr("http.status_code", resp.StatusCode)
	resp.Body.Close()

	monitor.MetricWebhookDeliveriesTotal.WithLabelValues("delivered").Inc()
	d.Status = protocol.DeliveryDelivered
	d.Attempts++
	d.UpdatedAt = time.Now()
	s.store.UpdateWebhookDelivery(context.TODO(), d) //nolint:errcheck
}

func (s *Sender) markFailed(d *protocol.WebhookDelivery) {
	d.Attempts++
	now := time.Now()
	d.UpdatedAt = now

	if d.Attempts >= maxAttempts {
		d.Status = protocol.DeliveryDead
		d.NextRetry = nil
	} else {
		d.Status = protocol.DeliveryFailed
		backoff := retrySchedule[d.Attempts-1]
		next := now.Add(backoff)
		d.NextRetry = &next
	}
	s.store.UpdateWebhookDelivery(context.TODO(), d) //nolint:errcheck
}

func (s *Sender) markDead(d *protocol.WebhookDelivery) {
	monitor.MetricWebhookDeliveriesTotal.WithLabelValues("dead").Inc()
	d.Status = protocol.DeliveryDead
	d.UpdatedAt = time.Now()
	s.store.UpdateWebhookDelivery(context.TODO(), d) //nolint:errcheck
}

func computeHMAC(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func validateDeliveryURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	host := u.Hostname()
	// Check literal IP
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("private/loopback IP blocked")
		}
		if host == "169.254.169.254" {
			return fmt.Errorf("metadata endpoint blocked")
		}
		return nil
	}
	// Resolve hostname and check all resolved IPs
	if host == "metadata.google.internal" {
		return fmt.Errorf("metadata endpoint blocked")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil // DNS failure — allow, will fail at delivery
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("hostname resolves to private/loopback IP")
		}
	}
	return nil
}
