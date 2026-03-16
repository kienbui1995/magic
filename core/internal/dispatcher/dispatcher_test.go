package dispatcher_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kienbm/magic-claw/core/internal/costctrl"
	"github.com/kienbm/magic-claw/core/internal/dispatcher"
	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/store"
)

func TestDispatcher_Success(t *testing.T) {
	// Mock worker that returns task.complete
	mockWorker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg protocol.Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.Type != protocol.MsgTaskAssign {
			t.Errorf("expected task.assign, got %s", msg.Type)
		}

		resp := map[string]any{
			"type": "task.complete",
			"payload": map[string]any{
				"task_id": "task_001",
				"output":  map[string]string{"result": "Hello, World!"},
				"cost":    0.05,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockWorker.Close()

	s := store.NewMemoryStore()
	bus := events.NewBus()
	cc := costctrl.New(s, bus)

	worker := &protocol.Worker{
		ID:       "worker_001",
		Name:     "TestBot",
		Status:   protocol.StatusActive,
		Endpoint: protocol.Endpoint{Type: "http", URL: mockWorker.URL},
	}
	s.AddWorker(worker)

	task := &protocol.Task{
		ID:       "task_001",
		Type:     "greeting",
		Status:   protocol.TaskAssigned,
		Input:    json.RawMessage(`{"name":"Kien"}`),
		Contract: protocol.Contract{TimeoutMs: 30000},
	}
	s.AddTask(task)

	d := dispatcher.New(s, bus, cc)
	err := d.Dispatch(task, worker)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Verify task is completed
	got, _ := s.GetTask("task_001")
	if got.Status != protocol.TaskCompleted {
		t.Errorf("task status: got %q, want completed", got.Status)
	}
	if got.Cost != 0.05 {
		t.Errorf("task cost: got %f, want 0.05", got.Cost)
	}
	if got.Output == nil {
		t.Error("task output should not be nil")
	}
}

func TestDispatcher_WorkerFails(t *testing.T) {
	// Mock worker that returns task.fail
	mockWorker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"type": "task.fail",
			"payload": map[string]any{
				"task_id": "task_002",
				"error":   map[string]string{"code": "handler_error", "message": "something went wrong"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockWorker.Close()

	s := store.NewMemoryStore()
	bus := events.NewBus()

	worker := &protocol.Worker{
		ID:       "worker_001",
		Name:     "TestBot",
		Status:   protocol.StatusActive,
		Endpoint: protocol.Endpoint{Type: "http", URL: mockWorker.URL},
	}
	s.AddWorker(worker)

	task := &protocol.Task{
		ID:     "task_002",
		Type:   "greeting",
		Status: protocol.TaskAssigned,
		Input:  json.RawMessage(`{}`),
	}
	s.AddTask(task)

	d := dispatcher.New(s, bus, nil)
	d.Dispatch(task, worker)

	got, _ := s.GetTask("task_002")
	if got.Status != protocol.TaskFailed {
		t.Errorf("task status: got %q, want failed", got.Status)
	}
	if got.Error == nil {
		t.Error("task error should not be nil")
	}
}

func TestDispatcher_WorkerUnreachable(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()

	worker := &protocol.Worker{
		ID:       "worker_001",
		Name:     "TestBot",
		Status:   protocol.StatusActive,
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:1"}, // unreachable
	}
	s.AddWorker(worker)

	task := &protocol.Task{
		ID:     "task_003",
		Type:   "greeting",
		Status: protocol.TaskAssigned,
		Input:  json.RawMessage(`{}`),
	}
	s.AddTask(task)

	d := dispatcher.New(s, bus, nil)
	err := d.Dispatch(task, worker)
	if err == nil {
		t.Error("should fail when worker is unreachable")
	}

	got, _ := s.GetTask("task_003")
	if got.Status != protocol.TaskFailed {
		t.Errorf("task status: got %q, want failed", got.Status)
	}
}

func TestDispatcher_CostTracking(t *testing.T) {
	mockWorker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"type": "task.complete",
			"payload": map[string]any{
				"task_id": "task_004",
				"output":  map[string]string{"result": "done"},
				"cost":    0.15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockWorker.Close()

	s := store.NewMemoryStore()
	bus := events.NewBus()
	cc := costctrl.New(s, bus)

	worker := &protocol.Worker{
		ID:       "worker_001",
		Name:     "TestBot",
		Status:   protocol.StatusActive,
		Endpoint: protocol.Endpoint{Type: "http", URL: mockWorker.URL},
		Limits:   protocol.WorkerLimits{MaxCostPerDay: 10.0},
	}
	s.AddWorker(worker)

	task := &protocol.Task{
		ID:     "task_004",
		Type:   "test",
		Status: protocol.TaskAssigned,
		Input:  json.RawMessage(`{}`),
	}
	s.AddTask(task)

	d := dispatcher.New(s, bus, cc)
	d.Dispatch(task, worker)

	time.Sleep(50 * time.Millisecond)

	report := cc.WorkerReport("worker_001")
	if report.TotalCost != 0.15 {
		t.Errorf("cost: got %f, want 0.15", report.TotalCost)
	}
}
