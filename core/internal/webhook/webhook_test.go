package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// --- helpers ---

func newTestWebhook(url string, eventTypes []string, secret string, active bool) *protocol.Webhook {
	return &protocol.Webhook{
		ID:        protocol.GenerateID("wh"),
		OrgID:     "org-1",
		URL:       url,
		Events:    eventTypes,
		Secret:    secret,
		Active:    active,
		CreatedAt: time.Now(),
	}
}

func newTestDelivery(webhookID, payload string, attempts int) *protocol.WebhookDelivery {
	return &protocol.WebhookDelivery{
		ID:        protocol.GenerateID("wd"),
		WebhookID: webhookID,
		EventType: "task.completed",
		Payload:   payload,
		Status:    protocol.DeliveryPending,
		Attempts:  attempts,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// waitFor polls fn every 5ms until it returns true or timeout expires.
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

// --- Manager tests ---

func TestManager_OnEvent_EnqueuesDelivery(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	defer bus.Stop()

	hook := newTestWebhook("http://example.com/hook", []string{"task.completed"}, "", true)
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	mgr := New(s, bus)
	mgr.Start()
	defer mgr.Stop()

	bus.Publish(events.Event{
		Type:      "task.completed",
		Source:    "test",
		Payload:   map[string]any{"task_id": "t-123"},
		Timestamp: time.Now(),
	})

	waitFor(t, 500*time.Millisecond, func() bool {
		deliveries := s.ListPendingWebhookDeliveries(context.Background())
		return len(deliveries) > 0
	})

	deliveries := s.ListPendingWebhookDeliveries(context.Background())
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	d := deliveries[0]
	if d.WebhookID != hook.ID {
		t.Errorf("expected WebhookID=%s, got %s", hook.ID, d.WebhookID)
	}
	if d.EventType != "task.completed" {
		t.Errorf("expected EventType=task.completed, got %s", d.EventType)
	}
	if d.Status != protocol.DeliveryPending {
		t.Errorf("expected status=%s, got %s", protocol.DeliveryPending, d.Status)
	}
}

func TestManager_OnEvent_IgnoresInactiveWebhook(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	defer bus.Stop()

	hook := newTestWebhook("http://example.com/hook", []string{"task.completed"}, "", false) // Active=false
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	mgr := New(s, bus)
	mgr.Start()
	defer mgr.Stop()

	bus.Publish(events.Event{
		Type:      "task.completed",
		Source:    "test",
		Timestamp: time.Now(),
	})

	// Give bus time to process
	time.Sleep(100 * time.Millisecond)

	deliveries := s.ListPendingWebhookDeliveries(context.Background())
	if len(deliveries) != 0 {
		t.Errorf("expected no deliveries for inactive webhook, got %d", len(deliveries))
	}
}

func TestManager_OnEvent_IgnoresNonMatchingEvent(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	defer bus.Stop()

	hook := newTestWebhook("http://example.com/hook", []string{"task.completed"}, "", true)
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	mgr := New(s, bus)
	mgr.Start()
	defer mgr.Stop()

	bus.Publish(events.Event{
		Type:      "worker.registered",
		Source:    "test",
		Timestamp: time.Now(),
	})

	// Give bus time to process
	time.Sleep(100 * time.Millisecond)

	deliveries := s.ListPendingWebhookDeliveries(context.Background())
	if len(deliveries) != 0 {
		t.Errorf("expected no deliveries for non-matching event, got %d", len(deliveries))
	}
}

// newTestSender returns a Sender with SSRF validation disabled.
// Tests that call deliver() with a local httptest.Server URL need this
// because validateDeliveryURL now correctly blocks loopback addresses.
func newTestSender(s store.Store) *Sender {
	sender := newSender(s)
	sender.validateURL = func(string) error { return nil }
	return sender
}

// --- Sender tests ---

func TestSender_Deliver_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := store.NewMemoryStore()
	hook := newTestWebhook(srv.URL, []string{"task.completed"}, "", true)
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	d := newTestDelivery(hook.ID, `{"type":"task.completed"}`, 0)
	if err := s.AddWebhookDelivery(context.Background(), d); err != nil {
		t.Fatalf("AddWebhookDelivery: %v", err)
	}

	sender := newTestSender(s)
	sender.deliver(d, hook)

	// The delivery object should be updated in memory (deliver modifies d directly)
	if d.Status != protocol.DeliveryDelivered {
		t.Errorf("expected status=%s, got %s", protocol.DeliveryDelivered, d.Status)
	}
	if d.Attempts != 1 {
		t.Errorf("expected Attempts=1, got %d", d.Attempts)
	}
}

func TestSender_Deliver_HMACSignature(t *testing.T) {
	var capturedSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSig = r.Header.Get("X-MagiC-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := store.NewMemoryStore()
	hook := newTestWebhook(srv.URL, []string{"task.completed"}, "mysecret", true)
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	payload := `{"type":"task.completed","data":"test"}`
	d := newTestDelivery(hook.ID, payload, 0)
	if err := s.AddWebhookDelivery(context.Background(), d); err != nil {
		t.Fatalf("AddWebhookDelivery: %v", err)
	}

	sender := newTestSender(s)
	sender.deliver(d, hook)

	if capturedSig == "" {
		t.Fatal("X-MagiC-Signature header not present")
	}
	if !strings.HasPrefix(capturedSig, "sha256=") {
		t.Errorf("expected signature to start with 'sha256=', got %q", capturedSig)
	}

	// Compute expected HMAC
	mac := hmac.New(sha256.New, []byte("mysecret"))
	mac.Write([]byte(payload))
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if capturedSig != expected {
		t.Errorf("HMAC mismatch: expected %q, got %q", expected, capturedSig)
	}
}

func TestSender_Deliver_NoSignatureWhenNoSecret(t *testing.T) {
	var capturedSig string
	sigHeaderPresent := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["X-Magic-Signature"]; ok {
			sigHeaderPresent = true
		}
		// Also check canonical form used in sender
		capturedSig = r.Header.Get("X-MagiC-Signature")
		if capturedSig != "" {
			sigHeaderPresent = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := store.NewMemoryStore()
	hook := newTestWebhook(srv.URL, []string{"task.completed"}, "", true) // empty secret
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	d := newTestDelivery(hook.ID, `{"type":"task.completed"}`, 0)
	if err := s.AddWebhookDelivery(context.Background(), d); err != nil {
		t.Fatalf("AddWebhookDelivery: %v", err)
	}

	sender := newTestSender(s)
	sender.deliver(d, hook)

	if sigHeaderPresent {
		t.Errorf("expected no X-MagiC-Signature header when secret is empty, got %q", capturedSig)
	}
}

// --- markFailed / backoff tests ---

func TestSender_MarkFailed_ExponentialBackoff(t *testing.T) {
	s := store.NewMemoryStore()
	hook := newTestWebhook("http://example.com", []string{"task.completed"}, "", true)
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	// First failure (Attempts was 0)
	d := newTestDelivery(hook.ID, `{}`, 0)
	if err := s.AddWebhookDelivery(context.Background(), d); err != nil {
		t.Fatalf("AddWebhookDelivery: %v", err)
	}

	sender := newSender(s)

	before := time.Now()
	sender.markFailed(d)

	if d.Attempts != 1 {
		t.Errorf("expected Attempts=1 after first failure, got %d", d.Attempts)
	}
	if d.Status != protocol.DeliveryFailed {
		t.Errorf("expected status=%s, got %s", protocol.DeliveryFailed, d.Status)
	}
	if d.NextRetry == nil {
		t.Fatal("expected NextRetry to be set after first failure")
	}
	// retrySchedule[0] = 30s; allow 1s tolerance
	expectedRetry := before.Add(30 * time.Second)
	diff := d.NextRetry.Sub(expectedRetry)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("expected NextRetry ≈ now+30s, got diff=%v", diff)
	}

	// Second failure (Attempts was 1)
	before2 := time.Now()
	sender.markFailed(d)

	if d.Attempts != 2 {
		t.Errorf("expected Attempts=2 after second failure, got %d", d.Attempts)
	}
	if d.NextRetry == nil {
		t.Fatal("expected NextRetry to be set after second failure")
	}
	// retrySchedule[1] = 5min
	expectedRetry2 := before2.Add(5 * time.Minute)
	diff2 := d.NextRetry.Sub(expectedRetry2)
	if diff2 < -time.Second || diff2 > time.Second {
		t.Errorf("expected NextRetry ≈ now+5min, got diff=%v", diff2)
	}
}

func TestSender_MarkFailed_MaxAttempts_Dead(t *testing.T) {
	s := store.NewMemoryStore()
	hook := newTestWebhook("http://example.com", []string{"task.completed"}, "", true)
	if err := s.AddWebhook(context.Background(), hook); err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}

	// Set Attempts to maxAttempts-1 (4) so the next failure hits maxAttempts (5)
	d := newTestDelivery(hook.ID, `{}`, maxAttempts-1)
	if err := s.AddWebhookDelivery(context.Background(), d); err != nil {
		t.Fatalf("AddWebhookDelivery: %v", err)
	}

	sender := newSender(s)
	sender.markFailed(d)

	if d.Status != protocol.DeliveryDead {
		t.Errorf("expected status=%s after max attempts, got %s", protocol.DeliveryDead, d.Status)
	}
	if d.NextRetry != nil {
		t.Errorf("expected NextRetry=nil after dead status, got %v", d.NextRetry)
	}
	if d.Attempts != maxAttempts {
		t.Errorf("expected Attempts=%d, got %d", maxAttempts, d.Attempts)
	}
}

// TestComputeHMAC tests HMAC computation directly (same-package access).
func TestComputeHMAC(t *testing.T) {
	cases := []struct {
		secret  string
		payload string
	}{
		{"mysecret", "hello world"},
		{"", "payload"},
		{"key", ""},
		{"unicode-key-🔑", `{"type":"task.completed"}`},
	}

	for _, tc := range cases {
		mac := hmac.New(sha256.New, []byte(tc.secret))
		mac.Write([]byte(tc.payload))
		expected := hex.EncodeToString(mac.Sum(nil))

		got := computeHMAC(tc.secret, tc.payload)
		if got != expected {
			t.Errorf("computeHMAC(%q, %q): expected %q, got %q", tc.secret, tc.payload, expected, got)
		}
	}
}
