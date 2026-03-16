package registry

import (
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
)

const HeartbeatTimeout = 60 * time.Second

func (r *Registry) StartHealthCheck(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			r.checkHealth()
		}
	}()
}

func (r *Registry) checkHealth() {
	workers := r.store.ListWorkers()
	now := time.Now()
	for _, w := range workers {
		if w.Status == protocol.StatusActive && now.Sub(w.LastHeartbeat) > HeartbeatTimeout {
			w.Status = protocol.StatusOffline
			r.store.UpdateWorker(w)
			r.bus.Publish(events.Event{
				Type:     "worker.offline",
				Source:   "registry",
				Severity: "warn",
				Payload:  map[string]any{"worker_id": w.ID, "worker_name": w.Name},
			})
		}
	}
}
