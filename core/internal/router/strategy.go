package router

import (
	"github.com/kienbui1995/magic/core/internal/protocol"
)

// Strategy defines the interface for routing plugins.
// Implementations receive pre-filtered candidates (active, capable, not excluded)
// and must return the best worker or nil if none is suitable.
type Strategy interface {
	Name() string
	Select(candidates []*protocol.Worker, task *protocol.Task) *protocol.Worker
}

// --- Built-in strategies ---

// BestMatchStrategy picks the worker with the highest availability.
type BestMatchStrategy struct{}

func (BestMatchStrategy) Name() string { return "best_match" }

func (BestMatchStrategy) Select(candidates []*protocol.Worker, task *protocol.Task) *protocol.Worker {
	if len(candidates) == 0 {
		return nil
	}
	best := candidates[0]
	bestScore := availability(best)
	for _, w := range candidates[1:] {
		if s := availability(w); s > bestScore {
			best, bestScore = w, s
		}
	}
	return best
}

// CheapestStrategy picks the worker with the lowest cost for the first required capability.
type CheapestStrategy struct{}

func (CheapestStrategy) Name() string { return "cheapest" }

func (CheapestStrategy) Select(candidates []*protocol.Worker, task *protocol.Task) *protocol.Worker {
	capName := ""
	if len(task.Routing.RequiredCapabilities) > 0 {
		capName = task.Routing.RequiredCapabilities[0]
	}
	var cheapest *protocol.Worker
	minCost := float64(999999)
	for _, w := range candidates {
		for _, c := range w.Capabilities {
			if c.Name == capName && c.EstCostPerCall < minCost {
				minCost = c.EstCostPerCall
				cheapest = w
			}
		}
	}
	return cheapest
}

// SpecificStrategy picks a specific preferred worker by ID.
type SpecificStrategy struct{}

func (SpecificStrategy) Name() string { return "specific" }

func (SpecificStrategy) Select(candidates []*protocol.Worker, task *protocol.Task) *protocol.Worker {
	if len(task.Routing.PreferredWorkers) == 0 {
		return nil
	}
	targetID := task.Routing.PreferredWorkers[0]
	for _, w := range candidates {
		if w.ID == targetID {
			return w
		}
	}
	return nil
}

// --- Helpers ---

func availability(w *protocol.Worker) float64 {
	if w.Limits.MaxConcurrentTasks <= 0 {
		return 1.0
	}
	a := 1.0 - float64(w.CurrentLoad)/float64(w.Limits.MaxConcurrentTasks)
	if a < 0 {
		return 0
	}
	return a
}

func filterByCapability(workers []*protocol.Worker, required []string) []*protocol.Worker {
	var result []*protocol.Worker
	for _, w := range workers {
		if w.Status != protocol.StatusActive {
			continue
		}
		if w.Limits.MaxConcurrentTasks > 0 && w.CurrentLoad >= w.Limits.MaxConcurrentTasks {
			continue
		}
		if hasAllCapabilities(w, required) {
			result = append(result, w)
		}
	}
	return result
}

func hasAllCapabilities(w *protocol.Worker, required []string) bool {
	capSet := make(map[string]bool, len(w.Capabilities))
	for _, c := range w.Capabilities {
		capSet[c.Name] = true
	}
	for _, r := range required {
		if !capSet[r] {
			return false
		}
	}
	return true
}
