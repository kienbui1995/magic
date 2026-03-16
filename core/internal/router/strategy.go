package router

import (
	"github.com/kienbui1995/magic/core/internal/protocol"
)

type WorkerScore struct {
	Worker *protocol.Worker
	Score  float64
}

func filterByCapability(workers []*protocol.Worker, required []string) []*protocol.Worker {
	var result []*protocol.Worker
	for _, w := range workers {
		if w.Status != protocol.StatusActive {
			continue
		}
		if hasAllCapabilities(w, required) {
			result = append(result, w)
		}
	}
	return result
}

func hasAllCapabilities(w *protocol.Worker, required []string) bool {
	capSet := make(map[string]bool)
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

func scoreBestMatch(w *protocol.Worker) float64 {
	availability := 1.0
	if w.Limits.MaxConcurrentTasks > 0 {
		availability = 1.0 - float64(w.CurrentLoad)/float64(w.Limits.MaxConcurrentTasks)
	}
	if availability < 0 {
		availability = 0
	}
	return availability
}

func findCheapest(workers []*protocol.Worker, capName string) *protocol.Worker {
	var cheapest *protocol.Worker
	minCost := float64(999999)
	for _, w := range workers {
		for _, c := range w.Capabilities {
			if c.Name == capName && c.EstCostPerCall < minCost {
				minCost = c.EstCostPerCall
				cheapest = w
			}
		}
	}
	return cheapest
}
