package costctrl

import (
	"fmt"
	"sync"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// Decision represents the outcome of a cost policy check.
type Decision int

const (
	Allow Decision = iota
	Warn
	Reject
)

// CostPolicy defines the interface for cost control plugins.
// Implementations inspect a worker's state after a cost is recorded
// and return a decision (Allow, Warn, Reject).
type CostPolicy interface {
	Name() string
	Check(worker *protocol.Worker, cost float64) Decision
}

type CostRecord struct {
	WorkerID string
	TaskID   string
	Cost     float64
}

type CostReport struct {
	TotalCost float64 `json:"total_cost"`
	TaskCount int     `json:"task_count"`
}

type Controller struct {
	store    store.Store
	bus      *events.Bus
	mu       sync.RWMutex
	records  []CostRecord
	policies []CostPolicy
}

func New(s store.Store, bus *events.Bus) *Controller {
	c := &Controller{store: s, bus: bus}
	c.RegisterPolicy(BudgetPolicy{})
	return c
}

// RegisterPolicy adds a custom cost policy plugin.
func (c *Controller) RegisterPolicy(p CostPolicy) {
	c.policies = append(c.policies, p)
}

func (c *Controller) RecordCost(workerID, taskID string, cost float64) {
	c.mu.Lock()
	c.records = append(c.records, CostRecord{WorkerID: workerID, TaskID: taskID, Cost: cost})
	w, err := c.store.GetWorker(workerID)
	if err == nil {
		w.TotalCostToday += cost
		c.store.UpdateWorker(w) //nolint:errcheck
	}
	c.mu.Unlock()

	if err == nil {
		c.applyPolicies(w, cost)
	}

	c.bus.Publish(events.Event{
		Type: "cost.recorded", Source: "costctrl",
		Payload: map[string]any{"worker_id": workerID, "task_id": taskID, "cost": cost},
	})
}

func (c *Controller) applyPolicies(w *protocol.Worker, cost float64) {
	for _, p := range c.policies {
		switch p.Check(w, cost) {
		case Reject:
			w.Status = protocol.StatusPaused
			c.store.UpdateWorker(w) //nolint:errcheck
			c.bus.Publish(events.Event{Type: "budget.exceeded", Source: "costctrl", Severity: "error",
				Payload: map[string]any{"worker_id": w.ID, "policy": p.Name(),
					"spent": w.TotalCostToday, "budget": w.Limits.MaxCostPerDay}})
			return // stop on first reject
		case Warn:
			c.bus.Publish(events.Event{Type: "budget.threshold", Source: "costctrl", Severity: "warn",
				Payload: map[string]any{"worker_id": w.ID, "policy": p.Name(),
					"percent": fmt.Sprintf("%.0f%%", w.TotalCostToday/w.Limits.MaxCostPerDay*100),
					"spent":   w.TotalCostToday, "budget": w.Limits.MaxCostPerDay}})
		}
	}
}

func (c *Controller) WorkerReport(workerID string) CostReport {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var r CostReport
	for _, rec := range c.records {
		if rec.WorkerID == workerID {
			r.TotalCost += rec.Cost
			r.TaskCount++
		}
	}
	return r
}

func (c *Controller) OrgReport() CostReport {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var r CostReport
	for _, rec := range c.records {
		r.TotalCost += rec.Cost
		r.TaskCount++
	}
	return r
}

// --- Built-in policy: BudgetPolicy ---

// BudgetPolicy warns at 80% and rejects at 100% of MaxCostPerDay.
type BudgetPolicy struct{}

func (BudgetPolicy) Name() string { return "budget" }

func (BudgetPolicy) Check(w *protocol.Worker, _ float64) Decision {
	if w.Limits.MaxCostPerDay <= 0 {
		return Allow
	}
	ratio := w.TotalCostToday / w.Limits.MaxCostPerDay
	if ratio >= 1.0 {
		return Reject
	}
	if ratio >= 0.8 {
		return Warn
	}
	return Allow
}
