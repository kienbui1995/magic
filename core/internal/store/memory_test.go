package store_test

import (
	"testing"

	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/store"
)

func TestMemoryStore_Workers(t *testing.T) {
	s := store.NewMemoryStore()

	w := &protocol.Worker{
		ID:     "worker_001",
		Name:   "TestBot",
		Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{
			{Name: "greeting"},
		},
	}

	if err := s.AddWorker(w); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}

	got, err := s.GetWorker("worker_001")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Name != "TestBot" {
		t.Errorf("Name: got %q, want TestBot", got.Name)
	}

	workers := s.ListWorkers()
	if len(workers) != 1 {
		t.Errorf("ListWorkers: got %d, want 1", len(workers))
	}

	found := s.FindWorkersByCapability("greeting")
	if len(found) != 1 {
		t.Errorf("FindByCapability: got %d, want 1", len(found))
	}

	found = s.FindWorkersByCapability("nonexistent")
	if len(found) != 0 {
		t.Errorf("FindByCapability nonexistent: got %d, want 0", len(found))
	}

	if err := s.RemoveWorker("worker_001"); err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}
	if _, err := s.GetWorker("worker_001"); err == nil {
		t.Error("GetWorker after remove should fail")
	}
}

func TestMemoryStore_Tasks(t *testing.T) {
	s := store.NewMemoryStore()

	task := &protocol.Task{
		ID:     "task_001",
		Type:   "greeting",
		Status: protocol.TaskPending,
	}

	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	got, err := s.GetTask("task_001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Type != "greeting" {
		t.Errorf("Type: got %q, want greeting", got.Type)
	}

	task.Status = protocol.TaskCompleted
	if err := s.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	got, _ = s.GetTask("task_001")
	if got.Status != protocol.TaskCompleted {
		t.Errorf("Status: got %q, want completed", got.Status)
	}
}

func TestMemoryStore_Workflows(t *testing.T) {
	s := store.NewMemoryStore()
	wf := &protocol.Workflow{ID: "wf_001", Name: "Test Workflow", Status: protocol.WorkflowPending,
		Steps: []protocol.WorkflowStep{{ID: "step1", TaskType: "greeting", Status: protocol.StepPending}}}
	if err := s.AddWorkflow(wf); err != nil {
		t.Fatalf("AddWorkflow: %v", err)
	}
	got, err := s.GetWorkflow("wf_001")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Name != "Test Workflow" {
		t.Errorf("Name: got %q", got.Name)
	}
	wf.Status = protocol.WorkflowRunning
	if err := s.UpdateWorkflow(wf); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	got, _ = s.GetWorkflow("wf_001")
	if got.Status != protocol.WorkflowRunning {
		t.Errorf("Status: got %q", got.Status)
	}
	if len(s.ListWorkflows()) != 1 {
		t.Errorf("ListWorkflows: got %d", len(s.ListWorkflows()))
	}
}

func TestMemoryStore_Teams(t *testing.T) {
	s := store.NewMemoryStore()
	team := &protocol.Team{ID: "team_001", Name: "Marketing", OrgID: "org_magic", DailyBudget: 10.0}
	if err := s.AddTeam(team); err != nil {
		t.Fatalf("AddTeam: %v", err)
	}
	got, err := s.GetTeam("team_001")
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.Name != "Marketing" {
		t.Errorf("Name: got %q", got.Name)
	}
	team.Workers = []string{"worker_001"}
	if err := s.UpdateTeam(team); err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}
	if len(s.ListTeams()) != 1 {
		t.Errorf("ListTeams: got %d", len(s.ListTeams()))
	}
	if err := s.RemoveTeam("team_001"); err != nil {
		t.Fatalf("RemoveTeam: %v", err)
	}
	if _, err := s.GetTeam("team_001"); err == nil {
		t.Error("should fail after remove")
	}
}

func TestMemoryStore_Knowledge(t *testing.T) {
	s := store.NewMemoryStore()

	entry := &protocol.KnowledgeEntry{
		ID:      "kb_001",
		Title:   "API Guidelines",
		Content: "Use REST conventions for all endpoints",
		Tags:    []string{"api", "rest"},
		Scope:   "org",
		ScopeID: "org_magic",
	}

	if err := s.AddKnowledge(entry); err != nil {
		t.Fatalf("AddKnowledge: %v", err)
	}

	got, err := s.GetKnowledge("kb_001")
	if err != nil {
		t.Fatalf("GetKnowledge: %v", err)
	}
	if got.Title != "API Guidelines" {
		t.Errorf("Title: got %q", got.Title)
	}

	entry.Content = "Updated content"
	if err := s.UpdateKnowledge(entry); err != nil {
		t.Fatalf("UpdateKnowledge: %v", err)
	}

	if len(s.ListKnowledge()) != 1 {
		t.Errorf("ListKnowledge: got %d", len(s.ListKnowledge()))
	}

	// Search by title substring
	results := s.SearchKnowledge("API")
	if len(results) != 1 {
		t.Errorf("SearchKnowledge 'API': got %d, want 1", len(results))
	}

	// Search by tag
	results = s.SearchKnowledge("rest")
	if len(results) != 1 {
		t.Errorf("SearchKnowledge 'rest': got %d, want 1", len(results))
	}

	// Search no match
	results = s.SearchKnowledge("nonexistent")
	if len(results) != 0 {
		t.Errorf("SearchKnowledge 'nonexistent': got %d, want 0", len(results))
	}

	if err := s.DeleteKnowledge("kb_001"); err != nil {
		t.Fatalf("DeleteKnowledge: %v", err)
	}
	if _, err := s.GetKnowledge("kb_001"); err == nil {
		t.Error("should fail after delete")
	}
}
