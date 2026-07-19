package registry

import (
	"context"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/protocol"
)

// HeartbeatTimeout is the duration after which a worker is marked offline.
const HeartbeatTimeout = 60 * time.Second

// StartHealthCheck runs a background goroutine that marks workers offline
// if no heartbeat is received. Returns a stop function to cancel it.
func (r *Registry) StartHealthCheck(interval time.Duration) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.checkHealth()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

func (r *Registry) checkHealth() {
	// TODO(ctx): derive from StartHealthCheck stop signal once Registry API takes ctx.
	ctx := context.TODO()
	workers := r.store.ListWorkers(ctx)
	now := time.Now()
	// Reset gauge to avoid stale series for deregistered workers.
	monitor.MetricWorkerHeartbeatLag.Reset()
	for _, w := range workers {
		lag := now.Sub(w.LastHeartbeat).Seconds()
		if lag < 0 {
			lag = 0
		}
		monitor.MetricWorkerHeartbeatLag.WithLabelValues(w.ID).Set(lag)
		if w.Status == protocol.StatusActive && now.Sub(w.LastHeartbeat) > HeartbeatTimeout {
			// Don't mark offline if worker has in-flight tasks — it may just be busy
			if w.CurrentLoad > 0 {
				continue
			}
			w.Status = protocol.StatusOffline
			r.store.UpdateWorker(ctx, w) //nolint:errcheck
			r.bus.Publish(events.Event{
				Type:     "worker.offline",
				Source:   "registry",
				Severity: "warn",
				Payload:  map[string]any{"worker_id": w.ID, "worker_name": w.Name},
			})
		}
	}
}
