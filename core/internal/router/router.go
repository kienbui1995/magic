package router

import (
	"context"
	"errors"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/store"
	"github.com/kienbui1995/magic/core/internal/tracing"
)

// ErrNoWorkerAvailable is returned when no suitable worker is found for a task.
var ErrNoWorkerAvailable = errors.New("no worker available for task")

// Router selects the best worker for a task based on routing strategy.
type Router struct {
	registry   *registry.Registry
	store      store.Store
	bus        *events.Bus
	strategies map[string]Strategy
}

// New creates a new task router with built-in strategies.
// Use RegisterStrategy to add custom routing plugins.
func New(reg *registry.Registry, s store.Store, bus *events.Bus) *Router {
	r := &Router{
		registry:   reg,
		store:      s,
		bus:        bus,
		strategies: make(map[string]Strategy),
	}
	// Register built-in strategies
	r.RegisterStrategy(BestMatchStrategy{})
	r.RegisterStrategy(CheapestStrategy{})
	r.RegisterStrategy(SpecificStrategy{})
	return r
}

// RegisterStrategy adds a custom routing strategy plugin.
// If a strategy with the same name exists, it is replaced.
func (r *Router) RegisterStrategy(s Strategy) {
	r.strategies[s.Name()] = s
}

// RouteTask selects a worker for the task using the configured routing strategy.
// When task.Context.OrgID is set, only workers in the same org are considered
// (security mode). When empty, all workers are eligible (dev mode).
//
// Kept for backward compatibility with call sites that do not yet have a
// context available. Prefer RouteTaskCtx so the routing span is a child of
// the caller's trace.
func (r *Router) RouteTask(task *protocol.Task) (*protocol.Worker, error) {
	// TODO(ctx): propagate from caller once all call sites pass ctx.
	return r.RouteTaskCtx(context.TODO(), task)
}

// RouteTaskCtx is the context-aware variant of RouteTask. Spans created here
// attach to any OTel span carried by ctx so the routing step shows up as a
// child of the incoming HTTP / workflow trace.
func (r *Router) RouteTaskCtx(ctx context.Context, task *protocol.Task) (*protocol.Worker, error) {
	ctx, span := tracing.StartSpan(ctx, "router.RouteTask")
	defer span.End()
	span.SetAttr("task.id", task.ID)
	span.SetAttr("task.type", task.Type)
	span.SetAttr("routing.strategy", task.Routing.Strategy)
	if task.Context.OrgID != "" {
		span.SetAttr("org.id", task.Context.OrgID)
	}

	orgID := task.Context.OrgID

	var allWorkers []*protocol.Worker
	if orgID != "" {
		allWorkers = r.store.ListWorkersByOrg(ctx, orgID)
	} else {
		allWorkers = r.registry.ListWorkers()
	}

	capable := filterByCapability(allWorkers, task.Routing.RequiredCapabilities)
	if len(capable) == 0 {
		return nil, ErrNoWorkerAvailable
	}

	if len(task.Routing.ExcludedWorkers) > 0 {
		excluded := make(map[string]bool, len(task.Routing.ExcludedWorkers))
		for _, id := range task.Routing.ExcludedWorkers {
			excluded[id] = true
		}
		var filtered []*protocol.Worker
		for _, w := range capable {
			if !excluded[w.ID] {
				filtered = append(filtered, w)
			}
		}
		capable = filtered
		if len(capable) == 0 {
			return nil, ErrNoWorkerAvailable
		}
	}

	// Lookup strategy; fall back to best_match for unknown names
	strategy, ok := r.strategies[task.Routing.Strategy]
	if !ok {
		strategy = r.strategies["best_match"]
	}

	selected := strategy.Select(capable, task)
	if selected == nil {
		return nil, ErrNoWorkerAvailable
	}

	span.SetAttr("worker.id", selected.ID)
	span.SetAttr("worker.name", selected.Name)

	task.AssignedWorker = selected.ID
	task.Status = protocol.TaskAssigned

	selected.CurrentLoad++
	r.store.UpdateWorker(ctx, selected) //nolint:errcheck

	r.bus.Publish(events.Event{
		Type:   "task.routed",
		Source: "router",
		Payload: map[string]any{
			"task_id":     task.ID,
			"worker_id":   selected.ID,
			"worker_name": selected.Name,
			"strategy":    task.Routing.Strategy,
		},
	})

	return selected, nil
}
