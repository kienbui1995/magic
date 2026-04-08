package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
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
	store  store.Store
	client *http.Client
	stop   chan struct{}
}

func newSender(s store.Store) *Sender {
	return &Sender{
		store:  s,
		client: &http.Client{Timeout: 10 * time.Second},
		stop:   make(chan struct{}),
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
	deliveries := s.store.ListPendingWebhookDeliveries()
	for _, d := range deliveries {
		// Skip deliveries not yet ready for retry
		if d.NextRetry != nil && time.Now().Before(*d.NextRetry) {
			continue
		}
		hook, err := s.store.GetWebhook(d.WebhookID)
		if err != nil {
			// Webhook deleted — mark dead
			s.markDead(d)
			continue
		}
		s.deliver(d, hook)
	}
}

func (s *Sender) deliver(d *protocol.WebhookDelivery, hook *protocol.Webhook) {
	req, err := http.NewRequest("POST", hook.URL, bytes.NewReader([]byte(d.Payload)))
	if err != nil {
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

	resp, err := s.client.Do(req)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			resp.Body.Close()
		}
		log.Printf("[webhook] delivery %s failed (attempt %d): status=%d err=%v",
			d.ID, d.Attempts+1, statusCode, err)
		s.markFailed(d)
		return
	}
	resp.Body.Close()

	d.Status = protocol.DeliveryDelivered
	d.Attempts++
	d.UpdatedAt = time.Now()
	s.store.UpdateWebhookDelivery(d) //nolint:errcheck
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
	s.store.UpdateWebhookDelivery(d) //nolint:errcheck
}

func (s *Sender) markDead(d *protocol.WebhookDelivery) {
	d.Status = protocol.DeliveryDead
	d.UpdatedAt = time.Now()
	s.store.UpdateWebhookDelivery(d) //nolint:errcheck
}

func computeHMAC(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
