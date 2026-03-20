package events

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Event represents a system event.
type Event struct {
	Type      string         `json:"type"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Severity  string         `json:"severity"`
}

// Handler processes an Event.
type Handler func(Event)

const (
	defaultPoolSize   = 64
	defaultBufferSize = 4096
)

// Bus is a bounded, ordered publish-subscribe event bus.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]*subscription
	eventCh  chan Event
	stopCh   chan struct{}
	stopped  bool
	wg       sync.WaitGroup
}

type subscription struct {
	handler  Handler
	canceled atomic.Bool
}

// NewBus creates a new event bus with default pool size (64) and buffer (4096).
func NewBus() *Bus {
	return NewBusWithConfig(defaultPoolSize, defaultBufferSize)
}

// NewBusWithConfig creates a bus with custom pool size and buffer capacity.
func NewBusWithConfig(poolSize, bufferSize int) *Bus {
	b := &Bus{
		handlers: make(map[string][]*subscription),
		eventCh:  make(chan Event, bufferSize),
		stopCh:   make(chan struct{}),
	}
	// Start worker pool
	for i := 0; i < poolSize; i++ {
		b.wg.Add(1)
		go b.worker()
	}
	return b
}

func (b *Bus) worker() {
	defer b.wg.Done()
	for {
		select {
		case e := <-b.eventCh:
			b.dispatch(e)
		case <-b.stopCh:
			// Drain remaining events
			for {
				select {
				case e := <-b.eventCh:
					b.dispatch(e)
				default:
					return
				}
			}
		}
	}
}

func (b *Bus) dispatch(e Event) {
	b.mu.RLock()
	// Specific handlers
	subs := b.handlers[e.Type]
	// Wildcard handlers
	wildcards := b.handlers["*"]
	b.mu.RUnlock()

	for _, s := range subs {
		if !s.canceled.Load() {
			b.safeCall(s.handler, e)
		}
	}
	if e.Type != "*" {
		for _, s := range wildcards {
			if !s.canceled.Load() {
				b.safeCall(s.handler, e)
			}
		}
	}
}

func (b *Bus) safeCall(h Handler, e Event) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[events] panic in handler for %q: %v", e.Type, r)
		}
	}()
	h(e)
}

// Subscribe registers a handler. Returns a cancel function to unsubscribe.
func (b *Bus) Subscribe(eventType string, handler Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub := &subscription{handler: handler}
	b.handlers[eventType] = append(b.handlers[eventType], sub)
	return func() { sub.canceled.Store(true) }
}

// Publish sends an event to the processing queue.
// Non-blocking: drops event if buffer is full (logs warning).
func (b *Bus) Publish(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	if e.Severity == "" {
		e.Severity = "info"
	}

	select {
	case b.eventCh <- e:
	default:
		log.Printf("[events] WARNING: event buffer full, dropping event type=%s", e.Type)
	}
}

// Stop gracefully shuts down the event bus, draining pending events.
func (b *Bus) Stop() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	b.stopped = true
	b.mu.Unlock()

	close(b.stopCh)
	b.wg.Wait()
}
