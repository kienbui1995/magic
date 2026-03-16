package events

import (
	"sync"
	"time"
)

// Event represents a system event published through the event bus.
type Event struct {
	Type      string         `json:"type"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Severity  string         `json:"severity"`
}

// Handler is a function that processes an Event.
type Handler func(Event)

// Bus is a publish-subscribe event bus for inter-module communication.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string][]Handler),
	}
}

// Subscribe registers a handler for events of the given type. Use "*" to subscribe to all events.
func (b *Bus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish sends an event to all subscribed handlers asynchronously with panic recovery.
func (b *Bus) Publish(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	if e.Severity == "" {
		e.Severity = "info"
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, h := range b.handlers[e.Type] {
		go func(handler Handler) {
			defer func() { recover() }()
			handler(e)
		}(h)
	}
	if e.Type != "*" {
		for _, h := range b.handlers["*"] {
			go func(handler Handler) {
				defer func() { recover() }()
				handler(e)
			}(h)
		}
	}
}
