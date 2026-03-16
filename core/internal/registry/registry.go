package registry

import (
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

type Registry struct {
	store store.Store
	bus   *events.Bus
}

func New(s store.Store, bus *events.Bus) *Registry {
	return &Registry{store: s, bus: bus}
}

func (r *Registry) Register(p protocol.RegisterPayload) (*protocol.Worker, error) {
	w := &protocol.Worker{
		ID:            protocol.GenerateID("worker"),
		Name:          p.Name,
		Capabilities:  p.Capabilities,
		Endpoint:      p.Endpoint,
		Limits:        p.Limits,
		Status:        protocol.StatusActive,
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
		Metadata:      p.Metadata,
	}

	if err := r.store.AddWorker(w); err != nil {
		return nil, err
	}

	r.bus.Publish(events.Event{
		Type:   "worker.registered",
		Source: "registry",
		Payload: map[string]any{
			"worker_id":   w.ID,
			"worker_name": w.Name,
		},
	})

	return w, nil
}

func (r *Registry) Deregister(workerID string) error {
	if err := r.store.RemoveWorker(workerID); err != nil {
		return err
	}

	r.bus.Publish(events.Event{
		Type:   "worker.deregistered",
		Source: "registry",
		Payload: map[string]any{"worker_id": workerID},
	})

	return nil
}

func (r *Registry) Heartbeat(p protocol.HeartbeatPayload) error {
	w, err := r.store.GetWorker(p.WorkerID)
	if err != nil {
		return err
	}
	w.LastHeartbeat = time.Now()
	w.CurrentLoad = p.CurrentLoad
	if p.Status != "" {
		w.Status = p.Status
	}
	return r.store.UpdateWorker(w)
}

func (r *Registry) GetWorker(id string) (*protocol.Worker, error) {
	return r.store.GetWorker(id)
}

func (r *Registry) ListWorkers() []*protocol.Worker {
	return r.store.ListWorkers()
}

func (r *Registry) FindByCapability(capability string) []*protocol.Worker {
	return r.store.FindWorkersByCapability(capability)
}
