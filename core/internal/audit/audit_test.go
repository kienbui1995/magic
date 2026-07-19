package audit

import (
	"context"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/store"
)

func newTestDeps() (store.Store, *events.Bus) {
	return store.NewMemoryStore(), events.NewBus()
}

func TestAudit_Record_WritesToStore(t *testing.T) {
	s, bus := newTestDeps()
	defer bus.Stop()

	l := New(s, bus)
	l.Record("org1", "worker1", "login", "session", "req1", "success", map[string]any{"ip": "1.2.3.4"})

	entries := s.QueryAudit(context.Background(), store.AuditFilter{})
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	e := entries[0]
	if e.OrgID != "org1" {
		t.Errorf("expected org_id=org1, got %s", e.OrgID)
	}
	if e.WorkerID != "worker1" {
		t.Errorf("expected worker_id=worker1, got %s", e.WorkerID)
	}
	if e.Action != "login" {
		t.Errorf("expected action=login, got %s", e.Action)
	}
	if e.Resource != "session" {
		t.Errorf("expected resource=session, got %s", e.Resource)
	}
	if e.RequestID != "req1" {
		t.Errorf("expected request_id=req1, got %s", e.RequestID)
	}
	if e.Outcome != "success" {
		t.Errorf("expected outcome=success, got %s", e.Outcome)
	}
	if e.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestAudit_Record_PublishesEvent(t *testing.T) {
	s, bus := newTestDeps()
	defer bus.Stop()

	l := New(s, bus)

	received := make(chan events.Event, 1)
	bus.Subscribe("audit.login", func(e events.Event) {
		received <- e
	})

	l.Record("org1", "worker1", "login", "session", "req1", "success", nil)

	select {
	case e := <-received:
		if e.Type != "audit.login" {
			t.Errorf("expected event type audit.login, got %s", e.Type)
		}
		if e.Severity != "info" {
			t.Errorf("expected severity info, got %s", e.Severity)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for audit event")
	}
}

func TestAudit_Query_FilterByWorker(t *testing.T) {
	s, bus := newTestDeps()
	defer bus.Stop()

	l := New(s, bus)
	l.Record("org1", "worker-A", "task.completed", "task/t1", "", "success", nil)
	l.Record("org1", "worker-B", "task.completed", "task/t2", "", "success", nil)
	l.Record("org1", "worker-A", "task.failed", "task/t3", "", "error", nil)

	entries := l.Query(store.AuditFilter{WorkerID: "worker-A"})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for worker-A, got %d", len(entries))
	}
	for _, e := range entries {
		if e.WorkerID != "worker-A" {
			t.Errorf("expected worker-A, got %s", e.WorkerID)
		}
	}
}

func TestAudit_Query_FilterByAction(t *testing.T) {
	s, bus := newTestDeps()
	defer bus.Stop()

	l := New(s, bus)
	l.Record("org1", "worker1", "task.completed", "task/t1", "", "success", nil)
	l.Record("org1", "worker1", "task.failed", "task/t2", "", "error", nil)
	l.Record("org1", "worker1", "task.completed", "task/t3", "", "success", nil)

	entries := l.Query(store.AuditFilter{Action: "task.completed"})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries with action=task.completed, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Action != "task.completed" {
			t.Errorf("expected action task.completed, got %s", e.Action)
		}
	}
}

func TestAudit_SubscribeToEvents_WorkerRegistered(t *testing.T) {
	s, bus := newTestDeps()
	defer bus.Stop()

	l := New(s, bus)
	l.SubscribeToEvents()

	bus.Publish(events.Event{
		Type:   "worker.registered",
		Source: "registry",
		Payload: map[string]any{
			"org_id":    "org1",
			"worker_id": "worker-x",
		},
	})

	// Give the async bus time to process
	time.Sleep(100 * time.Millisecond)

	entries := s.QueryAudit(context.Background(), store.AuditFilter{Action: "worker.registered"})
	if len(entries) == 0 {
		t.Fatal("expected audit entry for worker.registered, got none")
	}
	e := entries[0]
	if e.Action != "worker.registered" {
		t.Errorf("expected action=worker.registered, got %s", e.Action)
	}
	if e.WorkerID != "worker-x" {
		t.Errorf("expected worker_id=worker-x, got %s", e.WorkerID)
	}
}

func TestAudit_SubscribeToEvents_TaskRouted(t *testing.T) {
	s, bus := newTestDeps()
	defer bus.Stop()

	l := New(s, bus)
	l.SubscribeToEvents()

	bus.Publish(events.Event{
		Type:   "task.routed",
		Source: "router",
		Payload: map[string]any{
			"task_id":   "task-99",
			"worker_id": "worker-y",
			// org_id intentionally omitted — should not block
		},
	})

	time.Sleep(100 * time.Millisecond)

	entries := s.QueryAudit(context.Background(), store.AuditFilter{Action: "task.routed"})
	if len(entries) == 0 {
		t.Fatal("expected audit entry for task.routed, got none")
	}
	e := entries[0]
	if e.Action != "task.routed" {
		t.Errorf("expected action=task.routed, got %s", e.Action)
	}
	if e.OrgID != "" {
		t.Errorf("expected empty org_id for missing key, got %s", e.OrgID)
	}
	if e.Resource != "task/task-99" {
		t.Errorf("expected resource=task/task-99, got %s", e.Resource)
	}
}
