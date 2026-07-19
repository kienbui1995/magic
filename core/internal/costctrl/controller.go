package costctrl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
	"github.com/kienbui1995/magic/core/internal/tracing"
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

// StartDailyReset runs a background goroutine that resets TotalCostToday
// for all workers at midnight UTC. Returns a stop function.
func (c *Controller) StartDailyReset() func() {
	stop := make(chan struct{})
	go func() {
		for {
			now := time.Now().UTC()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			timer := time.NewTimer(midnight.Sub(now))
			select {
			case <-timer.C:
				c.resetDailyCosts()
			case <-stop:
				timer.Stop()
				return
			}
		}
	}()
	return func() { close(stop) }
}

func (c *Controller) resetDailyCosts() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// TODO(ctx): propagate from caller once costctrl API takes ctx.
	ctx := context.TODO()
	for _, w := range c.store.ListWorkers(ctx) {
		if w.TotalCostToday > 0 {
			w.TotalCostToday = 0
			if w.Status == protocol.StatusPaused {
				w.Status = protocol.StatusActive
			}
			c.store.UpdateWorker(ctx, w) //nolint:errcheck
		}
	}
	c.bus.Publish(events.Event{
		Type: "cost.daily_reset", Source: "costctrl",
		Payload: map[string]any{"time": time.Now().UTC().Format(time.RFC3339)},
	})
}

// RegisterPolicy adds a custom cost policy plugin.
func (c *Controller) RegisterPolicy(p CostPolicy) {
	c.policies = append(c.policies, p)
}

const maxCostRecords = 50_000

func (c *Controller) RecordCost(workerID, taskID string, cost float64) {
	// TODO(ctx): propagate from caller once all call sites pass ctx.
	c.RecordCostCtx(context.TODO(), workerID, taskID, cost)
}

// RecordCostCtx is the context-aware variant of RecordCost. Accepts a ctx so
// the cost-tracking span attaches to the caller's trace (dispatch → record).
func (c *Controller) RecordCostCtx(ctx context.Context, workerID, taskID string, cost float64) {
	ctx, span := tracing.StartSpan(ctx, "costctrl.RecordCost")
	defer span.End()
	span.SetAttr("worker.id", workerID)
	span.SetAttr("task.id", taskID)
	span.SetAttr("cost.usd", cost)

	c.mu.Lock()
	c.records = append(c.records, CostRecord{WorkerID: workerID, TaskID: taskID, Cost: cost})
	if len(c.records) > maxCostRecords {
		c.records = c.records[len(c.records)-maxCostRecords:]
	}
	// Atomic read-modify-write under lock to prevent lost updates
	var orgID string
	w, err := c.store.GetWorker(ctx, workerID)
	if err == nil {
		orgID = w.OrgID
		w.TotalCostToday += cost
		c.store.UpdateWorker(ctx, w) //nolint:errcheck
	}
	// Apply policies while still holding lock to prevent concurrent budget checks
	if err == nil {
		c.applyPolicies(ctx, w, cost)
	}
	c.mu.Unlock()
	if orgID != "" {
		span.SetAttr("org.id", orgID)
	}

	c.bus.Publish(events.Event{
		Type: "cost.recorded", Source: "costctrl",
		Payload: map[string]any{
			"worker_id": workerID,
			"task_id":   taskID,
			"cost":      cost,
			"org_id":    orgID,
		},
	})
}

func (c *Controller) applyPolicies(ctx context.Context, w *protocol.Worker, cost float64) {
	_, span := tracing.StartSpan(ctx, "costctrl.applyPolicies")
	defer span.End()
	span.SetAttr("worker.id", w.ID)
	span.SetAttr("policy.count", len(c.policies))

	for _, p := range c.policies {
		switch p.Check(w, cost) {
		case Reject:
			span.SetAttr("policy.result", "reject")
			span.SetAttr("policy.name", p.Name())
			w.Status = protocol.StatusPaused
			c.store.UpdateWorker(ctx, w) //nolint:errcheck
			c.bus.Publish(events.Event{Type: "budget.exceeded", Source: "costctrl", Severity: "error",
				Payload: map[string]any{"worker_id": w.ID, "org_id": w.OrgID, "policy": p.Name(),
					"spent": w.TotalCostToday, "budget": w.Limits.MaxCostPerDay}})
			return // stop on first reject
		case Warn:
			span.SetAttr("policy.result", "warn")
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
