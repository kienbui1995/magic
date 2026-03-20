package events_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
)

func TestEventBus_PubSub(t *testing.T) {
	bus := events.NewBus()
	defer bus.Stop()

	var received []events.Event
	var mu sync.Mutex

	_ = bus.Subscribe("task.completed", func(e events.Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(events.Event{
		Type:    "task.completed",
		Source:  "router",
		Payload: map[string]any{"task_id": "task_001"},
	})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("received: got %d, want 1", len(received))
	}
	if received[0].Type != "task.completed" {
		t.Errorf("type: got %q", received[0].Type)
	}
}

func TestEventBus_WildcardSubscribe(t *testing.T) {
	bus := events.NewBus()
	defer bus.Stop()

	var received []events.Event
	var mu sync.Mutex

	_ = bus.Subscribe("*", func(e events.Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(events.Event{Type: "task.completed"})
	bus.Publish(events.Event{Type: "worker.registered"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Errorf("received: got %d, want 2", len(received))
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := events.NewBus()
	defer bus.Stop()

	var count int64
	cancel := bus.Subscribe("test", func(e events.Event) {
		atomic.AddInt64(&count, 1)
	})

	bus.Publish(events.Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	cancel() // unsubscribe

	bus.Publish(events.Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt64(&count) != 1 {
		t.Errorf("count: got %d, want 1 (second event should be ignored)", atomic.LoadInt64(&count))
	}
}

func TestEventBus_Stop(t *testing.T) {
	bus := events.NewBus()

	var count int64
	_ = bus.Subscribe("test", func(e events.Event) {
		atomic.AddInt64(&count, 1)
	})

	// Publish some events
	for i := 0; i < 100; i++ {
		bus.Publish(events.Event{Type: "test"})
	}

	bus.Stop() // should drain all events

	if atomic.LoadInt64(&count) != 100 {
		t.Errorf("count: got %d, want 100 (all events should be drained)", atomic.LoadInt64(&count))
	}
}
