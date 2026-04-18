package benchmarks

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/costctrl"
	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/store"
)

// suppressDispatchLogs silences log output for dispatcher/router/store benchmarks.
func suppressDispatchLogs(b *testing.B) {
	b.Helper()
	orig := log.Writer()
	log.SetOutput(io.Discard)
	b.Cleanup(func() { log.SetOutput(orig) })
}

// newDispatcherStack returns a dispatcher wired to an in-memory store + bus
// plus a mock HTTP worker that immediately returns a `complete` message.
func newDispatcherStack(b *testing.B) (*dispatcher.Dispatcher, *protocol.Worker, *protocol.Task, func()) {
	b.Helper()

	// Mock worker: returns `complete` for whatever task_id it receives.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"task.complete","payload":{"task_id":"bench","output":{},"cost":0.001}}`))
	}))

	bus := events.NewBusWithConfig(64, 1<<20)
	s := store.NewMemoryStore()
	cc := costctrl.New(s, bus)
	ev := evaluator.New(bus)
	d := dispatcher.New(s, bus, cc, ev)

	worker := &protocol.Worker{
		ID:       "bench-worker",
		Name:     "bench-worker",
		Endpoint: protocol.Endpoint{Type: "http", URL: srv.URL},
		Status:   "online",
	}
	if err := s.AddWorker(worker); err != nil {
		b.Fatalf("AddWorker: %v", err)
	}

	task := &protocol.Task{
		ID:        "bench",
		Type:      "echo",
		Status:    protocol.TaskPending,
		Input:     []byte(`{}`),
		CreatedAt: time.Now(),
	}
	if err := s.AddTask(task); err != nil {
		b.Fatalf("AddTask: %v", err)
	}

	cleanup := func() {
		srv.Close()
		bus.Stop()
	}
	return d, worker, task, cleanup
}

// BenchmarkDispatcher_Dispatch measures the cost of one full dispatch round-trip
// (HTTP POST to a local mock worker + parse `complete` + store update + event publish).
func BenchmarkDispatcher_Dispatch(b *testing.B) {
	suppressDispatchLogs(b)
	d, worker, task, cleanup := newDispatcherStack(b)
	defer cleanup()

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset state each iteration so dispatcher accepts repeated runs.
		task.Status = protocol.TaskPending
		if err := d.Dispatch(ctx, task, worker); err != nil {
			b.Fatalf("Dispatch: %v", err)
		}
	}
}

// BenchmarkRouter_RouteTask measures route selection with 100 registered workers.
// This is a focused complement to the existing routing_test.go micro-benchmarks:
// it exercises the same pipeline but keeps the test here for bench-file locality.
func BenchmarkRouter_RouteTask(b *testing.B) {
	suppressDispatchLogs(b)
	rtr, cleanup := newRoutingStack(100)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := &protocol.Task{
			ID:        protocol.GenerateID("task"),
			Type:      "text-gen",
			Priority:  protocol.PriorityNormal,
			Status:    protocol.TaskPending,
			Routing:   protocol.RoutingConfig{Strategy: "best_match", RequiredCapabilities: []string{"text-gen"}},
			CreatedAt: time.Now(),
		}
		if _, err := rtr.RouteTask(task); err != nil {
			b.Fatalf("RouteTask: %v", err)
		}
	}
}

// BenchmarkStore_MemoryAddTask measures AddTask throughput on the in-memory store.
// Useful as a hardware-independent baseline for storage-layer regression detection.
func BenchmarkStore_MemoryAddTask(b *testing.B) {
	s := store.NewMemoryStore()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := &protocol.Task{
			ID:        fmt.Sprintf("t-%d", i),
			Type:      "echo",
			Status:    protocol.TaskPending,
			CreatedAt: time.Now(),
		}
		if err := s.AddTask(t); err != nil {
			b.Fatalf("AddTask: %v", err)
		}
	}
}

// Silence unused import warnings from router when this file is compiled alone.
var _ = router.New
