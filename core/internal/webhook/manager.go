package webhook

import (
	"encoding/json"
	"log"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// supportedEvents is the set of event types that trigger webhook delivery.
// These must match the Type strings published to the event bus.
var supportedEvents = []string{
	"task.completed", "task.failed", "task.dispatched",
	"worker.registered", "worker.deregistered",
	"workflow.completed", "workflow.failed",
	"budget.threshold", "budget.exceeded",
}

// Manager subscribes to the event bus and enqueues WebhookDelivery records.
// Sender goroutine processes the queue.
type Manager struct {
	store  store.Store
	bus    *events.Bus
	sender *Sender
}

// New creates a Manager. Call Start() to begin processing.
func New(s store.Store, bus *events.Bus) *Manager {
	return &Manager{
		store:  s,
		bus:    bus,
		sender: newSender(s),
	}
}

// Start subscribes to all supported events and starts the retry sender goroutine.
func (m *Manager) Start() {
	m.sender.start()
	for _, eventType := range supportedEvents {
		et := eventType // capture loop variable
		m.bus.Subscribe(et, func(e events.Event) {
			m.onEvent(e)
		})
	}
}

// Stop shuts down the sender goroutine.
func (m *Manager) Stop() {
	m.sender.Stop()
}

func (m *Manager) onEvent(e events.Event) {
	hooks := m.store.FindWebhooksByEvent(e.Type)
	if len(hooks) == 0 {
		return
	}

	payloadBytes, err := json.Marshal(map[string]any{
		"type":      e.Type,
		"timestamp": e.Timestamp,
		"data":      e.Payload,
	})
	if err != nil {
		log.Printf("[webhook] failed to marshal event %s: %v", e.Type, err)
		return
	}
	payload := string(payloadBytes)

	for _, hook := range hooks {
		d := &protocol.WebhookDelivery{
			ID:        protocol.GenerateID("wd"),
			WebhookID: hook.ID,
			EventType: e.Type,
			Payload:   payload,
			Status:    protocol.DeliveryPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := m.store.AddWebhookDelivery(d); err != nil {
			log.Printf("[webhook] failed to enqueue delivery for hook %s: %v", hook.ID, err)
		}
	}
}

// CreateWebhook registers a new webhook.
func (m *Manager) CreateWebhook(orgID, url string, eventTypes []string, secret string) (*protocol.Webhook, error) {
	hook := &protocol.Webhook{
		ID:        protocol.GenerateID("wh"),
		OrgID:     orgID,
		URL:       url,
		Events:    eventTypes,
		Secret:    secret,
		Active:    true,
		CreatedAt: time.Now(),
	}
	if err := m.store.AddWebhook(hook); err != nil {
		return nil, err
	}
	return hook, nil
}

// DeleteWebhook removes a webhook.
func (m *Manager) DeleteWebhook(id string) error {
	return m.store.DeleteWebhook(id)
}

// ListWebhooks returns all webhooks for an org. Secrets are redacted.
func (m *Manager) ListWebhooks(orgID string) []*protocol.Webhook {
	hooks := m.store.ListWebhooksByOrg(orgID)
	for _, h := range hooks {
		h.Secret = "" // never expose secret
	}
	return hooks
}

// ListDeliveries returns pending/failed deliveries for a webhook.
func (m *Manager) ListDeliveries(webhookID string) []*protocol.WebhookDelivery {
	all := m.store.ListPendingWebhookDeliveries()
	var result []*protocol.WebhookDelivery
	for _, d := range all {
		if d.WebhookID == webhookID {
			result = append(result, d)
		}
	}
	return result
}
