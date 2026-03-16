package events_test

import (
	"sync"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
)

func TestEventBus_PubSub(t *testing.T) {
	bus := events.NewBus()
	var received []events.Event
	var mu sync.Mutex

	bus.Subscribe("task.completed", func(e events.Event) {
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
	var received []events.Event
	var mu sync.Mutex

	bus.Subscribe("*", func(e events.Event) {
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
