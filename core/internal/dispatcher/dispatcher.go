package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/kienbui1995/magic/core/internal/costctrl"
	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

const (
	maxRetries           = 2
	circuitOpenDuration  = 30 * time.Second
	circuitFailThreshold = 3
)

type circuitState struct {
	failures  int
	openUntil time.Time
}

type Dispatcher struct {
	store     store.Store
	bus       *events.Bus
	costCtrl  *costctrl.Controller
	evaluator *evaluator.Evaluator
	client    *http.Client
	circuits  map[string]*circuitState
	circuitMu sync.Mutex
}

func New(s store.Store, bus *events.Bus, cc *costctrl.Controller, ev *evaluator.Evaluator) *Dispatcher {
	return &Dispatcher{
		store:     s,
		bus:       bus,
		costCtrl:  cc,
		evaluator: ev,
		client:    &http.Client{Timeout: 60 * time.Second},
		circuits:  make(map[string]*circuitState),
	}
}

// DispatchResponse is what the worker returns
type DispatchResponse struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type completePayload struct {
	TaskID string          `json:"task_id"`
	Output json.RawMessage `json:"output"`
	Cost   float64         `json:"cost"`
}

type failPayload struct {
	TaskID string `json:"task_id"`
	Error  struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func validateEndpointURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			// Allow loopback for dev, block other private ranges
			if !ip.IsLoopback() {
				return fmt.Errorf("endpoint URL points to private network")
			}
		}
	}
	return nil
}

// Dispatch sends a task.assign to the worker's endpoint and processes the response.
// It runs synchronously — caller should use a goroutine if async is needed.
func (d *Dispatcher) Dispatch(ctx context.Context, task *protocol.Task, worker *protocol.Worker) error {
	// Check circuit breaker
	if d.isCircuitOpen(worker.ID) {
		d.handleFailure(task, worker, "circuit breaker open: worker has too many recent failures")
		return fmt.Errorf("circuit breaker open for worker %s", worker.ID)
	}

	if err := validateEndpointURL(worker.Endpoint.URL); err != nil {
		d.handleFailure(task, worker, fmt.Sprintf("invalid endpoint: %v", err))
		return err
	}

	// Apply contract timeout if specified
	if task.Contract.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(task.Contract.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	// Build task.assign message
	assignPayload, _ := json.Marshal(protocol.TaskAssignPayload{
		TaskID:   task.ID,
		TaskType: task.Type,
		Priority: task.Priority,
		Input:    task.Input,
		Contract: task.Contract,
		Context:  task.Context,
	})

	msg := protocol.NewMessage(protocol.MsgTaskAssign, "org", worker.ID, assignPayload)
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal task.assign: %w", err)
	}

	task.Status = protocol.TaskInProgress
	d.store.UpdateTask(task)

	d.bus.Publish(events.Event{
		Type:   "task.dispatched",
		Source: "dispatcher",
		Payload: map[string]any{
			"task_id":   task.ID,
			"worker_id": worker.ID,
			"endpoint":  worker.Endpoint.URL,
		},
	})

	// Retry loop
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				d.handleFailure(task, worker, fmt.Sprintf("context cancelled: %v", ctx.Err()))
				d.recordFailure(worker.ID)
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		lastErr = d.tryDispatch(ctx, body, task, worker)
		if lastErr == nil {
			d.recordSuccess(worker.ID)
			return nil
		}
	}

	// All retries failed
	d.handleFailure(task, worker, fmt.Sprintf("failed after %d retries: %v", maxRetries+1, lastErr))
	d.recordFailure(worker.ID)
	return lastErr
}

func (d *Dispatcher) tryDispatch(ctx context.Context, body []byte, task *protocol.Task, worker *protocol.Worker) error {
	req, err := http.NewRequestWithContext(ctx, "POST", worker.Endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	var dispResp DispatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&dispResp); err != nil {
		return err
	}

	switch dispResp.Type {
	case protocol.MsgTaskComplete:
		return d.handleComplete(task, worker, dispResp.Payload)
	case protocol.MsgTaskFail:
		var fp failPayload
		json.Unmarshal(dispResp.Payload, &fp)
		d.handleFailure(task, worker, fp.Error.Message)
		return nil // worker explicitly failed, don't retry
	default:
		return fmt.Errorf("unexpected response type: %s", dispResp.Type)
	}
}

func (d *Dispatcher) handleComplete(task *protocol.Task, worker *protocol.Worker, payload json.RawMessage) error {
	var cp completePayload
	json.Unmarshal(payload, &cp)

	task.Output = cp.Output
	task.Cost = cp.Cost
	task.Progress = 100

	// Evaluate output quality if schema specified
	if d.evaluator != nil && len(task.Contract.OutputSchema) > 0 {
		result := d.evaluator.Evaluate(cp.Output, task.Contract)
		if !result.Pass {
			task.Status = protocol.TaskFailed
			task.Error = &protocol.TaskError{Code: "evaluation_failed", Message: fmt.Sprintf("output validation failed: %v", result.Errors)}
			now := time.Now()
			task.CompletedAt = &now
			d.store.UpdateTask(task)
			return fmt.Errorf("evaluation failed")
		}
	}

	task.Status = protocol.TaskCompleted
	now := time.Now()
	task.CompletedAt = &now
	d.store.UpdateTask(task)

	// Track cost
	if d.costCtrl != nil && cp.Cost > 0 {
		d.costCtrl.RecordCost(worker.ID, task.ID, cp.Cost)
	}

	// Update worker load
	worker.CurrentLoad--
	if worker.CurrentLoad < 0 {
		worker.CurrentLoad = 0
	}
	d.store.UpdateWorker(worker)

	d.bus.Publish(events.Event{
		Type:   "task.completed",
		Source: "dispatcher",
		Payload: map[string]any{
			"task_id":   task.ID,
			"worker_id": worker.ID,
			"cost":      cp.Cost,
		},
	})

	return nil
}

func (d *Dispatcher) handleFailure(task *protocol.Task, worker *protocol.Worker, reason string) {
	task.Status = protocol.TaskFailed
	task.Error = &protocol.TaskError{Code: "dispatch_error", Message: reason}
	now := time.Now()
	task.CompletedAt = &now
	d.store.UpdateTask(task)

	worker.CurrentLoad--
	if worker.CurrentLoad < 0 {
		worker.CurrentLoad = 0
	}
	d.store.UpdateWorker(worker)

	d.bus.Publish(events.Event{
		Type:     "task.failed",
		Source:   "dispatcher",
		Severity: "error",
		Payload: map[string]any{
			"task_id":   task.ID,
			"worker_id": worker.ID,
			"reason":    reason,
		},
	})
}

func (d *Dispatcher) isCircuitOpen(workerID string) bool {
	d.circuitMu.Lock()
	defer d.circuitMu.Unlock()
	cs, ok := d.circuits[workerID]
	if !ok {
		return false
	}
	if cs.failures >= circuitFailThreshold && time.Now().Before(cs.openUntil) {
		return true
	}
	if time.Now().After(cs.openUntil) {
		cs.failures = 0 // reset after cooldown
	}
	return false
}

func (d *Dispatcher) recordSuccess(workerID string) {
	d.circuitMu.Lock()
	defer d.circuitMu.Unlock()
	delete(d.circuits, workerID)
}

func (d *Dispatcher) recordFailure(workerID string) {
	d.circuitMu.Lock()
	defer d.circuitMu.Unlock()
	cs, ok := d.circuits[workerID]
	if !ok {
		cs = &circuitState{}
		d.circuits[workerID] = cs
	}
	cs.failures++
	if cs.failures >= circuitFailThreshold {
		cs.openUntil = time.Now().Add(circuitOpenDuration)
	}
}
