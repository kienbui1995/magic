package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/kienbm/magic-claw/core/internal/costctrl"
	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/store"
)

type Dispatcher struct {
	store    store.Store
	bus      *events.Bus
	costCtrl *costctrl.Controller
	client   *http.Client
}

func New(s store.Store, bus *events.Bus, cc *costctrl.Controller) *Dispatcher {
	return &Dispatcher{
		store:    s,
		bus:      bus,
		costCtrl: cc,
		client:   &http.Client{Timeout: 60 * time.Second},
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
func (d *Dispatcher) Dispatch(task *protocol.Task, worker *protocol.Worker) error {
	if err := validateEndpointURL(worker.Endpoint.URL); err != nil {
		d.handleFailure(task, worker, fmt.Sprintf("invalid endpoint: %v", err))
		return err
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

	// Update task status
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

	// POST to worker endpoint
	resp, err := d.client.Post(worker.Endpoint.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		d.handleFailure(task, worker, fmt.Sprintf("connection failed: %v", err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.handleFailure(task, worker, fmt.Sprintf("worker returned status %d", resp.StatusCode))
		return fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	// Parse response
	var dispResp DispatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&dispResp); err != nil {
		d.handleFailure(task, worker, fmt.Sprintf("invalid response: %v", err))
		return err
	}

	switch dispResp.Type {
	case protocol.MsgTaskComplete:
		return d.handleComplete(task, worker, dispResp.Payload)
	case protocol.MsgTaskFail:
		var fp failPayload
		json.Unmarshal(dispResp.Payload, &fp)
		d.handleFailure(task, worker, fp.Error.Message)
		return nil
	default:
		d.handleFailure(task, worker, fmt.Sprintf("unexpected response type: %s", dispResp.Type))
		return fmt.Errorf("unexpected response type: %s", dispResp.Type)
	}
}

func (d *Dispatcher) handleComplete(task *protocol.Task, worker *protocol.Worker, payload json.RawMessage) error {
	var cp completePayload
	json.Unmarshal(payload, &cp)

	task.Status = protocol.TaskCompleted
	task.Output = cp.Output
	task.Cost = cp.Cost
	now := time.Now()
	task.CompletedAt = &now
	task.Progress = 100
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
