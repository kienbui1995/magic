package orchestrator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kienbm/magic-claw/core/internal/dispatcher"
	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/router"
	"github.com/kienbm/magic-claw/core/internal/store"
)

type Orchestrator struct {
	store      store.Store
	router     *router.Router
	bus        *events.Bus
	dispatcher *dispatcher.Dispatcher
}

func New(s store.Store, rt *router.Router, bus *events.Bus, disp *dispatcher.Dispatcher) *Orchestrator {
	return &Orchestrator{store: s, router: rt, bus: bus, dispatcher: disp}
}

func (o *Orchestrator) Submit(name string, steps []protocol.WorkflowStep, ctx protocol.TaskContext) (*protocol.Workflow, error) {
	if err := ValidateDAG(steps); err != nil {
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

	if err := o.store.AddWorkflow(wf); err != nil {
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
	wf, err := o.store.GetWorkflow(workflowID)
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

	if err := o.store.UpdateWorkflow(wf); err != nil {
		return err
	}

	o.bus.Publish(events.Event{
		Type:   "workflow.step_completed",
		Source: "orchestrator",
		Payload: map[string]any{"workflow_id": workflowID, "task_id": taskID},
	})

	go o.advanceWorkflow(wf)
	return nil
}

func (o *Orchestrator) FailStep(workflowID, taskID string, taskErr protocol.TaskError) error {
	wf, err := o.store.GetWorkflow(workflowID)
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
				o.store.UpdateWorkflow(wf)
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

	if err := o.store.UpdateWorkflow(wf); err != nil {
		return err
	}

	go o.advanceWorkflow(wf)
	return nil
}

func (o *Orchestrator) advanceWorkflow(wf *protocol.Workflow) {
	if IsWorkflowDone(wf.Steps) {
		if HasFailed(wf.Steps) {
			wf.Status = protocol.WorkflowFailed
		} else {
			wf.Status = protocol.WorkflowCompleted
		}
		now := time.Now()
		wf.DoneAt = &now
		o.store.UpdateWorkflow(wf)

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

	o.store.UpdateWorkflow(wf)
}

func (o *Orchestrator) dispatchStep(wf *protocol.Workflow, step *protocol.WorkflowStep) {
	task := &protocol.Task{
		ID:       protocol.GenerateID("task"),
		Type:     step.TaskType,
		Priority: protocol.PriorityNormal,
		Status:   protocol.TaskPending,
		Input:    step.Input,
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{step.TaskType},
		},
		WorkflowID: wf.ID,
		Context:    wf.Context,
		CreatedAt:  time.Now(),
	}

	worker, err := o.router.RouteTask(task)
	if err != nil {
		step.Status = protocol.StepFailed
		step.Error = &protocol.TaskError{Code: "no_worker", Message: err.Error()}
		return
	}

	o.store.AddTask(task)
	step.Status = protocol.StepRunning
	step.TaskID = task.ID

	// Dispatch to worker asynchronously
	if o.dispatcher != nil {
		go func() {
			err := o.dispatcher.Dispatch(task, worker)
			if err != nil {
				o.FailStep(wf.ID, task.ID, protocol.TaskError{Code: "dispatch_error", Message: err.Error()})
			} else {
				// Task completed successfully, advance workflow
				got, _ := o.store.GetTask(task.ID)
				if got != nil && got.Status == protocol.TaskCompleted {
					o.CompleteStep(wf.ID, task.ID, got.Output)
				}
			}
		}()
	}
}

func (o *Orchestrator) GetWorkflow(id string) (*protocol.Workflow, error) {
	return o.store.GetWorkflow(id)
}

func (o *Orchestrator) ListWorkflows() []*protocol.Workflow {
	return o.store.ListWorkflows()
}
