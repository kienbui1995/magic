package store_test

import (
	"os"
	"testing"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

func TestSQLiteStore_Workers(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()

	w := &protocol.Worker{
		ID:     "worker_001",
		Name:   "TestBot",
		Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{{Name: "greeting"}},
	}

	if err := s.AddWorker(w); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}

	got, err := s.GetWorker("worker_001")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Name != "TestBot" {
		t.Errorf("Name: got %q", got.Name)
	}

	w.Status = protocol.StatusPaused
	if err := s.UpdateWorker(w); err != nil {
		t.Fatalf("UpdateWorker: %v", err)
	}

	workers := s.ListWorkers()
	if len(workers) != 1 {
		t.Errorf("ListWorkers: got %d", len(workers))
	}

	found := s.FindWorkersByCapability("greeting")
	// Paused worker should not be found
	if len(found) != 0 {
		t.Errorf("FindByCapability paused: got %d, want 0", len(found))
	}

	if err := s.RemoveWorker("worker_001"); err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}
	if _, err := s.GetWorker("worker_001"); err == nil {
		t.Error("should fail after remove")
	}
}

func TestSQLiteStore_TasksAndWorkflows(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()

	task := &protocol.Task{ID: "task_001", Type: "greeting", Status: protocol.TaskPending}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	got, _ := s.GetTask("task_001")
	if got.Type != "greeting" {
		t.Errorf("Type: got %q", got.Type)
	}

	wf := &protocol.Workflow{ID: "wf_001", Name: "Test", Status: protocol.WorkflowPending}
	if err := s.AddWorkflow(wf); err != nil {
		t.Fatalf("AddWorkflow: %v", err)
	}
	gotWf, _ := s.GetWorkflow("wf_001")
	if gotWf.Name != "Test" {
		t.Errorf("Name: got %q", gotWf.Name)
	}
}

func TestSQLiteStore_Persistence(t *testing.T) {
	path := "/tmp/magic_test.db"
	os.Remove(path)
	defer os.Remove(path)

	// Write
	s1, _ := store.NewSQLiteStore(path)
	s1.AddWorker(&protocol.Worker{ID: "w1", Name: "Bot", Status: protocol.StatusActive})
	s1.Close()

	// Read in new connection
	s2, _ := store.NewSQLiteStore(path)
	defer s2.Close()
	got, err := s2.GetWorker("w1")
	if err != nil {
		t.Fatalf("should persist: %v", err)
	}
	if got.Name != "Bot" {
		t.Errorf("Name: got %q", got.Name)
	}
}
