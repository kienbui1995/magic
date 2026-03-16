package costctrl

import (
	"fmt"
	"sync"

	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/store"
)

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
	store   store.Store
	bus     *events.Bus
	mu      sync.RWMutex
	records []CostRecord
}

func New(s store.Store, bus *events.Bus) *Controller {
	return &Controller{store: s, bus: bus}
}

func (c *Controller) RecordCost(workerID, taskID string, cost float64) {
	c.mu.Lock()
	c.records = append(c.records, CostRecord{WorkerID: workerID, TaskID: taskID, Cost: cost})
	c.mu.Unlock()

	w, err := c.store.GetWorker(workerID)
	if err == nil {
		w.TotalCostToday += cost
		c.store.UpdateWorker(w)
		c.checkBudget(w)
	}

	c.bus.Publish(events.Event{
		Type: "cost.recorded", Source: "costctrl",
		Payload: map[string]any{"worker_id": workerID, "task_id": taskID, "cost": cost},
	})
}

func (c *Controller) checkBudget(w *protocol.Worker) {
	if w.Limits.MaxCostPerDay <= 0 {
		return
	}
	ratio := w.TotalCostToday / w.Limits.MaxCostPerDay
	if ratio >= 1.0 {
		w.Status = protocol.StatusPaused
		c.store.UpdateWorker(w)
		c.bus.Publish(events.Event{Type: "budget.exceeded", Source: "costctrl", Severity: "error",
			Payload: map[string]any{"worker_id": w.ID, "spent": w.TotalCostToday, "budget": w.Limits.MaxCostPerDay}})
	} else if ratio >= 0.8 {
		c.bus.Publish(events.Event{Type: "budget.threshold", Source: "costctrl", Severity: "warn",
			Payload: map[string]any{"worker_id": w.ID, "percent": fmt.Sprintf("%.0f%%", ratio*100),
				"spent": w.TotalCostToday, "budget": w.Limits.MaxCostPerDay}})
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
