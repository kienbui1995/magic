package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/store"
	"github.com/kienbui1995/magic/core/internal/tracing"
)

type Orchestrator struct {
	store      store.Store
	router     *router.Router
	bus        *events.Bus
	dispatcher *dispatcher.Dispatcher
	mu         sync.Mutex // protects workflow state transitions
	ctx        context.Context // shutdown context
	wg         sync.WaitGroup  // tracks in-flight step dispatches
}

func New(s store.Store, rt *router.Router, bus *events.Bus, disp *dispatcher.Dispatcher) *Orchestrator {
	return &Orchestrator{store: s, router: rt, bus: bus, dispatcher: disp, ctx: context.Background()}
}

// SetShutdownContext sets the context used for step dispatches.
func (o *Orchestrator) SetShutdownContext(ctx context.Context) { o.ctx = ctx }

// Wait blocks until all in-flight step dispatches complete.
func (o *Orchestrator) Wait() { o.wg.Wait() }

func (o *Orchestrator) Submit(name string, steps []protocol.WorkflowStep, ctx protocol.TaskContext) (*protocol.Workflow, error) {
	_, span := tracing.StartSpan(o.ctx, "orchestrator.Submit")
	defer span.End()
	span.SetAttr("workflow.name", name)
	span.SetAttr("workflow.steps", len(steps))
	if ctx.OrgID != "" {
		span.SetAttr("org.id", ctx.OrgID)
	}

	if err := ValidateDAG(steps); err != nil {
		span.SetError(err)
		return nil, fmt.Errorf("invalid workflow: %w", err)
	}

	for i := range steps {
		steps[i].Status = protocol.StepPending
	}

	wf := &protocol.Workflow{
		ID:        protocol.GenerateID("wf"),
		Name:      name,
		Steps:     steps,
		Status:    protocol.WorkflowRunning,
		Context:   ctx,
		CreatedAt: time.Now(),
	}
	span.SetAttr("workflow.id", wf.ID)

	if err := o.store.AddWorkflow(o.ctx, wf); err != nil {
		return nil, err
	}

	o.bus.Publish(events.Event{
		Type:   "workflow.started",
		Source: "orchestrator",
		Payload: map[string]any{
			"workflow_id": wf.ID,
			"name":        wf.Name,
			"steps":       len(wf.Steps),
		},
	})

	o.advanceWorkflow(wf)
	return wf, nil
}

func (o *Orchestrator) CompleteStep(workflowID, taskID string, output json.RawMessage) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	wf, err := o.store.GetWorkflow(o.ctx, workflowID)
	if err != nil {
		return err
	}

	for i := range wf.Steps {
		if wf.Steps[i].TaskID == taskID {
			wf.Steps[i].Status = protocol.StepCompleted
			wf.Steps[i].Output = output
			break
		}
	}

	if err := o.store.UpdateWorkflow(o.ctx, wf); err != nil {
		return err
	}

	o.bus.Publish(events.Event{
		Type:   "workflow.step_completed",
		Source: "orchestrator",
		Payload: map[string]any{"workflow_id": workflowID, "task_id": taskID},
	})

	o.advanceWorkflowLocked(wf)
	return nil
}

func (o *Orchestrator) FailStep(workflowID, taskID string, taskErr protocol.TaskError) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	wf, err := o.store.GetWorkflow(o.ctx, workflowID)
	if err != nil {
		return err
	}

	for i := range wf.Steps {
		if wf.Steps[i].TaskID == taskID {
			step := &wf.Steps[i]
			step.Error = &taskErr

			switch step.OnFailure {
			case "skip":
				step.Status = protocol.StepSkipped
			case "abort":
				step.Status = protocol.StepFailed
				wf.Status = protocol.WorkflowAborted
				o.store.UpdateWorkflow(o.ctx, wf) //nolint:errcheck
				o.bus.Publish(events.Event{
					Type:     "workflow.aborted",
					Source:   "orchestrator",
					Severity: "error",
					Payload:  map[string]any{"workflow_id": workflowID, "failed_step": step.ID},
				})
				return nil
			default:
				step.Status = protocol.StepFailed
			}
			break
		}
	}

	if err := o.store.UpdateWorkflow(o.ctx, wf); err != nil {
		return err
	}

	o.advanceWorkflowLocked(wf)
	return nil
}

func (o *Orchestrator) advanceWorkflow(wf *protocol.Workflow) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.advanceWorkflowLocked(wf)
}

func (o *Orchestrator) advanceWorkflowLocked(wf *protocol.Workflow) {
	// Don't advance if workflow is already in a terminal state
	if wf.Status == protocol.WorkflowAborted || wf.Status == protocol.WorkflowCompleted || wf.Status == protocol.WorkflowFailed {
		return
	}

	if IsWorkflowDone(wf.Steps) {
		if HasFailed(wf.Steps) {
			wf.Status = protocol.WorkflowFailed
		} else {
			wf.Status = protocol.WorkflowCompleted
		}
		now := time.Now()
		wf.DoneAt = &now
		o.store.UpdateWorkflow(o.ctx, wf) //nolint:errcheck

		o.bus.Publish(events.Event{
			Type:   "workflow.completed",
			Source: "orchestrator",
			Payload: map[string]any{"workflow_id": wf.ID, "status": wf.Status},
		})
		return
	}

	ready := FindReadySteps(wf.Steps)
	for _, stepID := range ready {
		for i := range wf.Steps {
			if wf.Steps[i].ID == stepID {
				o.dispatchStep(wf, &wf.Steps[i])
				break
			}
		}
	}

	o.store.UpdateWorkflow(o.ctx, wf)  //nolint:errcheck
}

func (o *Orchestrator) dispatchStep(wf *protocol.Workflow, step *protocol.WorkflowStep) {
	_, span := tracing.StartSpan(o.ctx, "orchestrator.dispatchStep")
	defer span.End()
	span.SetAttr("workflow.id", wf.ID)
	span.SetAttr("step.id", step.ID)
	span.SetAttr("step.task_type", step.TaskType)

	// Check if step needs approval before dispatch
	if step.ApprovalRequired {
		step.Status = protocol.StepAwaitApproval
		o.bus.Publish(events.Event{
			Type:     "workflow.step_awaiting_approval",
			Source:   "orchestrator",
			Severity: "warn",
			Payload: map[string]any{
				"workflow_id": wf.ID,
				"step_id":     step.ID,
				"task_type":   step.TaskType,
			},
		})
		return
	}

	// Build input: merge dependency outputs into step input
	input := step.Input
	if len(step.DependsOn) > 0 {
		merged := make(map[string]any)
		// Start with step's own input
		if len(input) > 0 {
			json.Unmarshal(input, &merged) //nolint:errcheck
		}
		// Add outputs from dependencies
		depOutputs := make(map[string]json.RawMessage)
		for _, depID := range step.DependsOn {
			for _, s := range wf.Steps {
				if s.ID == depID && len(s.Output) > 0 {
					depOutputs[depID] = s.Output
				}
			}
		}
		if len(depOutputs) > 0 {
			merged["_deps"] = depOutputs
		}
		input, _ = json.Marshal(merged)
	}

	task := &protocol.Task{
		ID:       protocol.GenerateID("task"),
		Type:     step.TaskType,
		Priority: protocol.PriorityNormal,
		Status:   protocol.TaskPending,
		Input:    input,
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{step.TaskType},
		},
		WorkflowID: wf.ID,
		Context:    wf.Context,
		CreatedAt:  time.Now(),
	}

	worker, err := o.router.RouteTaskCtx(o.ctx, task)
	if err != nil {
		step.Status = protocol.StepFailed
		step.Error = &protocol.TaskError{Code: "no_worker", Message: err.Error()}
		return
	}

	o.store.AddTask(o.ctx, task) //nolint:errcheck
	step.Status = protocol.StepRunning
	step.TaskID = task.ID

	// Dispatch to worker asynchronously
	if o.dispatcher != nil {
		o.wg.Add(1)
		go func() {
			defer o.wg.Done()
			err := o.dispatcher.Dispatch(o.ctx, task, worker)
			if err != nil {
				o.FailStep(wf.ID, task.ID, protocol.TaskError{Code: "dispatch_error", Message: err.Error()}) //nolint:errcheck
			} else {
				// Task completed successfully, advance workflow
				got, _ := o.store.GetTask(o.ctx, task.ID)
				if got != nil && got.Status == protocol.TaskCompleted {
					o.CompleteStep(wf.ID, task.ID, got.Output) //nolint:errcheck
				}
			}
		}()
	}
}

// ApproveStep approves a step that is awaiting approval, allowing it to proceed.
func (o *Orchestrator) ApproveStep(workflowID, stepID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	wf, err := o.store.GetWorkflow(o.ctx, workflowID)
	if err != nil {
		return err
	}

	for i := range wf.Steps {
		if wf.Steps[i].ID == stepID && wf.Steps[i].Status == protocol.StepAwaitApproval {
			wf.Steps[i].ApprovalRequired = false
			wf.Steps[i].Status = protocol.StepPending
			if err := o.store.UpdateWorkflow(o.ctx, wf); err != nil {
				return err
			}
			o.bus.Publish(events.Event{
				Type:   "workflow.step_approved",
				Source: "orchestrator",
				Payload: map[string]any{"workflow_id": workflowID, "step_id": stepID},
			})
			o.advanceWorkflowLocked(wf)
			return nil
		}
	}
	return fmt.Errorf("step not found or not awaiting approval")
}

// CancelWorkflow aborts a running workflow, marking pending steps as failed.
func (o *Orchestrator) CancelWorkflow(workflowID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	wf, err := o.store.GetWorkflow(o.ctx, workflowID)
	if err != nil {
		return err
	}

	if wf.Status != protocol.WorkflowRunning {
		return fmt.Errorf("workflow is not running (status: %s)", wf.Status)
	}

	for i := range wf.Steps {
		s := &wf.Steps[i]
		switch s.Status {
		case protocol.StepPending, protocol.StepAwaitApproval, protocol.StepRunning:
			s.Status = protocol.StepFailed
			s.Error = &protocol.TaskError{Code: "cancelled", Message: "workflow cancelled"}
		}
	}

	wf.Status = protocol.WorkflowAborted
	now := time.Now()
	wf.DoneAt = &now
	o.store.UpdateWorkflow(o.ctx, wf)  //nolint:errcheck

	o.bus.Publish(events.Event{
		Type:     "workflow.cancelled",
		Source:   "orchestrator",
		Severity: "warn",
		Payload:  map[string]any{"workflow_id": workflowID},
	})

	return nil
}

func (o *Orchestrator) GetWorkflow(id string) (*protocol.Workflow, error) {
	return o.store.GetWorkflow(o.ctx, id)
}

func (o *Orchestrator) ListWorkflows() []*protocol.Workflow {
	return o.store.ListWorkflows(o.ctx)
}
