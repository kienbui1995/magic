package monitor_test

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/monitor"
)

// safeBuffer is a thread-safe bytes.Buffer for testing.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func (sb *safeBuffer) Bytes() []byte {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return append([]byte(nil), sb.buf.Bytes()...)
}

func TestMonitor_CapturesEvents(t *testing.T) {
	bus := events.NewBus()
	buf := &safeBuffer{}
	mon := monitor.New(bus, buf)
	mon.Start()

	bus.Publish(events.Event{
		Type:    "task.completed",
		Source:  "router",
		Payload: map[string]any{"task_id": "task_001"},
	})

	time.Sleep(50 * time.Millisecond)

	stats := mon.Stats()
	if stats.TotalEvents == 0 {
		t.Error("should have captured at least 1 event")
	}
}

func TestMonitor_WritesJSON(t *testing.T) {
	bus := events.NewBus()
	buf := &safeBuffer{}
	mon := monitor.New(bus, buf)
	mon.Start()

	bus.Publish(events.Event{
		Type:   "worker.registered",
		Source: "registry",
	})

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	if output == "" {
		t.Fatal("no output written")
	}

	var logEntry map[string]any
	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	if err := json.Unmarshal(lines[0], &logEntry); err != nil {
		t.Fatalf("invalid JSON log: %v\nOutput: %s", err, output)
	}

	if logEntry["event_type"] != "worker.registered" {
		t.Errorf("event_type: got %v", logEntry["event_type"])
	}
}
