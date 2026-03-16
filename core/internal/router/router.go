package router

import (
	"fmt"
	"sort"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/store"
)

var ErrNoWorkerAvailable = fmt.Errorf("no worker available for task")

type Router struct {
	registry *registry.Registry
	store    store.Store
	bus      *events.Bus
}

func New(reg *registry.Registry, s store.Store, bus *events.Bus) *Router {
	return &Router{registry: reg, store: s, bus: bus}
}

func (r *Router) RouteTask(task *protocol.Task) (*protocol.Worker, error) {
	allWorkers := r.registry.ListWorkers()
	capable := filterByCapability(allWorkers, task.Routing.RequiredCapabilities)

	if len(capable) == 0 {
		return nil, ErrNoWorkerAvailable
	}

	if len(task.Routing.ExcludedWorkers) > 0 {
		excluded := make(map[string]bool)
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

	var selected *protocol.Worker

	switch task.Routing.Strategy {
	case "cheapest":
		capName := ""
		if len(task.Routing.RequiredCapabilities) > 0 {
			capName = task.Routing.RequiredCapabilities[0]
		}
		selected = findCheapest(capable, capName)

	case "specific":
		if len(task.Routing.PreferredWorkers) > 0 {
			targetID := task.Routing.PreferredWorkers[0]
			for _, w := range capable {
				if w.ID == targetID {
					selected = w
					break
				}
			}
		}

	default:
		scores := make([]WorkerScore, len(capable))
		for i, w := range capable {
			scores[i] = WorkerScore{Worker: w, Score: scoreBestMatch(w)}
		}
		sort.Slice(scores, func(i, j int) bool {
			return scores[i].Score > scores[j].Score
		})
		selected = scores[0].Worker
	}

	if selected == nil {
		return nil, ErrNoWorkerAvailable
	}

	task.AssignedWorker = selected.ID
	task.Status = protocol.TaskAssigned

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
