package orchestrator_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/orchestrator"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/store"
)

func setupOrchestrator(t *testing.T) (*orchestrator.Orchestrator, store.Store) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)

	reg.Register(protocol.RegisterPayload{
		Name: "MultiBot",
		Capabilities: []protocol.Capability{
			{Name: "market_research"},
			{Name: "content_writing"},
			{Name: "seo_optimization"},
		},
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
		Limits:   protocol.WorkerLimits{MaxConcurrentTasks: 10},
	})

	orch := orchestrator.New(s, rt, bus, nil) // nil dispatcher for unit tests
	return orch, s
}

func TestOrchestrator_SubmitWorkflow(t *testing.T) {
	orch, s := setupOrchestrator(t)

	wf, err := orch.Submit("Campaign", []protocol.WorkflowStep{
		{ID: "research", TaskType: "market_research", Input: json.RawMessage(`{"topic": "AI"}`)},
		{ID: "content", TaskType: "content_writing", DependsOn: []string{"research"}},
	}, protocol.TaskContext{OrgID: "org_magic"})

	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if wf.Status != protocol.WorkflowRunning {
		t.Errorf("Status: got %q, want running", wf.Status)
	}

	got, err := s.GetWorkflow(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Name != "Campaign" {
		t.Errorf("Name: got %q", got.Name)
	}
}

func TestOrchestrator_InvalidDAG(t *testing.T) {
	orch, _ := setupOrchestrator(t)

	_, err := orch.Submit("Bad", []protocol.WorkflowStep{
		{ID: "a", TaskType: "t", DependsOn: []string{"b"}},
		{ID: "b", TaskType: "t", DependsOn: []string{"a"}},
	}, protocol.TaskContext{})

	if err == nil {
		t.Error("should reject cyclic workflow")
	}
}

func TestOrchestrator_CompleteStep(t *testing.T) {
	orch, s := setupOrchestrator(t)

	wf, _ := orch.Submit("Campaign", []protocol.WorkflowStep{
		{ID: "research", TaskType: "market_research", Input: json.RawMessage(`{}`)},
		{ID: "content", TaskType: "content_writing", DependsOn: []string{"research"}, Input: json.RawMessage(`{}`)},
	}, protocol.TaskContext{})

	got, _ := s.GetWorkflow(context.Background(), wf.ID)
	researchTaskID := got.Steps[0].TaskID

	err := orch.CompleteStep(wf.ID, researchTaskID, json.RawMessage(`{"data": "results"}`))
	if err != nil {
		t.Fatalf("CompleteStep: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	got, _ = s.GetWorkflow(context.Background(), wf.ID)
	if got.Steps[0].Status != protocol.StepCompleted {
		t.Errorf("research status: got %q", got.Steps[0].Status)
	}
	if got.Steps[1].Status != protocol.StepRunning {
		t.Errorf("content status: got %q, want running", got.Steps[1].Status)
	}
}

func TestOrchestrator_WorkflowCompletion(t *testing.T) {
	orch, s := setupOrchestrator(t)

	wf, _ := orch.Submit("Simple", []protocol.WorkflowStep{
		{ID: "only", TaskType: "market_research", Input: json.RawMessage(`{}`)},
	}, protocol.TaskContext{})

	got, _ := s.GetWorkflow(context.Background(), wf.ID)
	taskID := got.Steps[0].TaskID

	orch.CompleteStep(wf.ID, taskID, json.RawMessage(`{"done": true}`))
	time.Sleep(50 * time.Millisecond)

	got, _ = s.GetWorkflow(context.Background(), wf.ID)
	if got.Status != protocol.WorkflowCompleted {
		t.Errorf("workflow status: got %q, want completed", got.Status)
	}
}

func TestOrchestrator_FailStepSkip(t *testing.T) {
	orch, s := setupOrchestrator(t)

	wf, _ := orch.Submit("WithSkip", []protocol.WorkflowStep{
		{ID: "a", TaskType: "market_research", Input: json.RawMessage(`{}`)},
		{ID: "b", TaskType: "content_writing", DependsOn: []string{"a"}, OnFailure: "skip", Input: json.RawMessage(`{}`)},
	}, protocol.TaskContext{})

	got, _ := s.GetWorkflow(context.Background(), wf.ID)
	taskIDA := got.Steps[0].TaskID

	orch.CompleteStep(wf.ID, taskIDA, json.RawMessage(`{}`))
	time.Sleep(100 * time.Millisecond)

	got, _ = s.GetWorkflow(context.Background(), wf.ID)
	taskIDB := got.Steps[1].TaskID
	orch.FailStep(wf.ID, taskIDB, protocol.TaskError{Code: "err", Message: "failed"})
	time.Sleep(50 * time.Millisecond)

	got, _ = s.GetWorkflow(context.Background(), wf.ID)
	if got.Steps[1].Status != protocol.StepSkipped {
		t.Errorf("step B status: got %q, want skipped", got.Steps[1].Status)
	}
	if got.Status != protocol.WorkflowCompleted {
		t.Errorf("workflow status: got %q, want completed", got.Status)
	}
}

func TestOrchestrator_StepOutputFlowsToNext(t *testing.T) {
	orch, s := setupOrchestrator(t)

	wf, _ := orch.Submit("Pipeline", []protocol.WorkflowStep{
		{ID: "step1", TaskType: "market_research", Input: json.RawMessage(`{"topic": "AI"}`)},
		{ID: "step2", TaskType: "content_writing", DependsOn: []string{"step1"}, Input: json.RawMessage(`{"tone": "formal"}`)},
	}, protocol.TaskContext{})

	got, _ := s.GetWorkflow(context.Background(), wf.ID)
	task1ID := got.Steps[0].TaskID

	// Complete step1 with output
	orch.CompleteStep(wf.ID, task1ID, json.RawMessage(`{"research_data": "AI is growing"}`))
	time.Sleep(100 * time.Millisecond)

	// Check step2's task has merged input with _deps
	got, _ = s.GetWorkflow(context.Background(), wf.ID)
	task2ID := got.Steps[1].TaskID
	if task2ID == "" {
		t.Fatal("step2 should have been dispatched")
	}

	task2, _ := s.GetTask(context.Background(), task2ID)
	var input map[string]any
	json.Unmarshal(task2.Input, &input)

	if input["tone"] != "formal" {
		t.Errorf("original input lost: got %v", input)
	}
	if input["_deps"] == nil {
		t.Error("_deps should contain step1 output")
	}
}

func TestOrchestrator_ApprovalGate(t *testing.T) {
	orch, s := setupOrchestrator(t)

	wf, _ := orch.Submit("WithApproval", []protocol.WorkflowStep{
		{ID: "auto", TaskType: "market_research", Input: json.RawMessage(`{}`)},
		{ID: "manual", TaskType: "content_writing", DependsOn: []string{"auto"}, ApprovalRequired: true, Input: json.RawMessage(`{}`)},
	}, protocol.TaskContext{})

	// Complete auto step
	got, _ := s.GetWorkflow(context.Background(), wf.ID)
	orch.CompleteStep(wf.ID, got.Steps[0].TaskID, json.RawMessage(`{}`))
	time.Sleep(100 * time.Millisecond)

	// manual step should be awaiting approval, not running
	got, _ = s.GetWorkflow(context.Background(), wf.ID)
	if got.Steps[1].Status != protocol.StepAwaitApproval {
		t.Errorf("step status: got %q, want awaiting_approval", got.Steps[1].Status)
	}

	// Approve it
	err := orch.ApproveStep(wf.ID, "manual")
	if err != nil {
		t.Fatalf("ApproveStep: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Now it should be running
	got, _ = s.GetWorkflow(context.Background(), wf.ID)
	if got.Steps[1].Status != protocol.StepRunning {
		t.Errorf("step status after approval: got %q, want running", got.Steps[1].Status)
	}
}

func TestOrchestrator_CancelWorkflow(t *testing.T) {
	orch, s := setupOrchestrator(t)

	wf, _ := orch.Submit("Cancellable", []protocol.WorkflowStep{
		{ID: "step1", TaskType: "market_research", Input: json.RawMessage(`{}`)},
		{ID: "step2", TaskType: "content_writing", DependsOn: []string{"step1"}, Input: json.RawMessage(`{}`)},
	}, protocol.TaskContext{})

	err := orch.CancelWorkflow(wf.ID)
	if err != nil {
		t.Fatalf("CancelWorkflow: %v", err)
	}

	got, _ := s.GetWorkflow(context.Background(), wf.ID)
	if got.Status != protocol.WorkflowAborted {
		t.Errorf("status: got %q, want aborted", got.Status)
	}
}
