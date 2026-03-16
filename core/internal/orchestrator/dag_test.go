package orchestrator_test

import (
	"testing"

	"github.com/kienbm/magic-claw/core/internal/orchestrator"
	"github.com/kienbm/magic-claw/core/internal/protocol"
)

func TestDAG_FindReady_NoDeps(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", TaskType: "task_a", Status: protocol.StepPending},
		{ID: "b", TaskType: "task_b", Status: protocol.StepPending, DependsOn: []string{"a"}},
	}
	ready := orchestrator.FindReadySteps(steps)
	if len(ready) != 1 || ready[0] != "a" {
		t.Errorf("ready: got %v, want [a]", ready)
	}
}

func TestDAG_FindReady_DepCompleted(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", TaskType: "task_a", Status: protocol.StepCompleted},
		{ID: "b", TaskType: "task_b", Status: protocol.StepPending, DependsOn: []string{"a"}},
		{ID: "c", TaskType: "task_c", Status: protocol.StepPending, DependsOn: []string{"a"}},
	}
	ready := orchestrator.FindReadySteps(steps)
	if len(ready) != 2 {
		t.Errorf("ready: got %v, want [b, c]", ready)
	}
}

func TestDAG_FindReady_PartialDeps(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", TaskType: "task_a", Status: protocol.StepCompleted},
		{ID: "b", TaskType: "task_b", Status: protocol.StepRunning},
		{ID: "c", TaskType: "task_c", Status: protocol.StepPending, DependsOn: []string{"a", "b"}},
	}
	ready := orchestrator.FindReadySteps(steps)
	if len(ready) != 0 {
		t.Errorf("ready: got %v, want [] (b not done)", ready)
	}
}

func TestDAG_FindReady_SkippedDep(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", TaskType: "task_a", Status: protocol.StepSkipped},
		{ID: "b", TaskType: "task_b", Status: protocol.StepPending, DependsOn: []string{"a"}},
	}
	ready := orchestrator.FindReadySteps(steps)
	if len(ready) != 1 || ready[0] != "b" {
		t.Errorf("ready: got %v, want [b]", ready)
	}
}

func TestDAG_IsComplete(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", Status: protocol.StepCompleted},
		{ID: "b", Status: protocol.StepCompleted},
	}
	if !orchestrator.IsWorkflowDone(steps) {
		t.Error("should be done")
	}
}

func TestDAG_IsComplete_WithSkipped(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", Status: protocol.StepCompleted},
		{ID: "b", Status: protocol.StepSkipped},
	}
	if !orchestrator.IsWorkflowDone(steps) {
		t.Error("should be done (skipped counts)")
	}
}

func TestDAG_IsComplete_NotYet(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", Status: protocol.StepCompleted},
		{ID: "b", Status: protocol.StepRunning},
	}
	if orchestrator.IsWorkflowDone(steps) {
		t.Error("should not be done")
	}
}

func TestDAG_ValidateNoCycle(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}
	if err := orchestrator.ValidateDAG(steps); err == nil {
		t.Error("should detect cycle")
	}
}

func TestDAG_ValidateValid(t *testing.T) {
	steps := []protocol.WorkflowStep{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}
	if err := orchestrator.ValidateDAG(steps); err != nil {
		t.Errorf("valid DAG should pass: %v", err)
	}
}
