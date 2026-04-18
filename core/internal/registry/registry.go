package registry

import (
	"fmt"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// Registry manages worker registration, heartbeats, and discovery.
type Registry struct {
	store store.Store
	bus   *events.Bus
}

// New creates a new worker registry.
func New(s store.Store, bus *events.Bus) *Registry {
	return &Registry{store: s, bus: bus}
}

// Register adds a new worker to the system.
func (r *Registry) Register(p protocol.RegisterPayload) (*protocol.Worker, error) {
	if p.WorkerToken != "" || r.store.HasAnyWorkerTokens() {
		// Security mode: token required
		hash := protocol.HashToken(p.WorkerToken)
		token, err := r.store.GetWorkerTokenByHash(hash)
		if err != nil {
			return nil, fmt.Errorf("invalid worker token")
		}
		if !token.IsValid() {
			return nil, fmt.Errorf("token expired or revoked")
		}
		if token.WorkerID != "" {
			return nil, fmt.Errorf("token already in use")
		}
		orgID := token.OrgID

		w := &protocol.Worker{
			ID:            protocol.GenerateID("worker"),
			Name:          p.Name,
			OrgID:         orgID,
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

		// Bind token to worker; rollback on failure
		token.WorkerID = w.ID
		if err := r.store.UpdateWorkerToken(token); err != nil {
			r.store.RemoveWorker(w.ID) //nolint:errcheck
			return nil, fmt.Errorf("token already in use")
		}

		r.bus.Publish(events.Event{
			Type:   "worker.registered",
			Source: "registry",
			Payload: map[string]any{
				"worker_id":   w.ID,
				"worker_name": w.Name,
				"org_id":      w.OrgID,
			},
		})

		return w, nil
	}

	// Dev mode: no tokens exist, allow anonymous registration
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

// Deregister removes a worker from the system.
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

// Heartbeat updates a worker's health status. Does not override "paused" status.
func (r *Registry) Heartbeat(p protocol.HeartbeatPayload) error {
	if p.WorkerToken != "" || r.store.HasAnyWorkerTokens() {
		// Security mode: validate token
		hash := protocol.HashToken(p.WorkerToken)
		token, err := r.store.GetWorkerTokenByHash(hash)
		if err != nil {
			r.store.AppendAudit(&protocol.AuditEntry{ //nolint:errcheck
				ID:       protocol.GenerateID("audit"),
				WorkerID: p.WorkerID,
				Action:   "worker.heartbeat",
				Resource: "worker:" + p.WorkerID,
				Outcome:  "denied",
			})
			return fmt.Errorf("invalid worker token")
		}
		if !token.IsValid() {
			r.store.AppendAudit(&protocol.AuditEntry{ //nolint:errcheck
				ID:       protocol.GenerateID("audit"),
				WorkerID: p.WorkerID,
				Action:   "worker.heartbeat",
				Resource: "worker:" + p.WorkerID,
				Outcome:  "denied",
			})
			return fmt.Errorf("token expired or revoked")
		}
		if token.WorkerID != p.WorkerID {
			r.store.AppendAudit(&protocol.AuditEntry{ //nolint:errcheck
				ID:       protocol.GenerateID("audit"),
				WorkerID: p.WorkerID,
				Action:   "worker.heartbeat",
				Resource: "worker:" + p.WorkerID,
				Outcome:  "denied",
			})
			return fmt.Errorf("token not authorized for this worker")
		}
	}
	// Dev mode: no tokens exist, skip validation

	w, err := r.store.GetWorker(p.WorkerID)
	if err != nil {
		return err
	}
	w.LastHeartbeat = time.Now()
	w.CurrentLoad = p.CurrentLoad
	if p.Status != "" && w.Status != protocol.StatusPaused {
		w.Status = p.Status
	}
	if err := r.store.UpdateWorker(w); err != nil {
		return err
	}

	r.bus.Publish(events.Event{
		Type:   "worker.heartbeat",
		Source: "registry",
		Payload: map[string]any{
			"worker_id": p.WorkerID,
		},
	})

	return nil
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

// PauseWorker marks a worker as paused. The router will skip paused workers
// when selecting targets for new tasks. Heartbeats from the worker will not
// override the paused state.
func (r *Registry) PauseWorker(id string) error {
	return r.setWorkerStatus(id, protocol.StatusPaused, "worker.paused")
}

// ResumeWorker transitions a paused worker back to active.
func (r *Registry) ResumeWorker(id string) error {
	return r.setWorkerStatus(id, protocol.StatusActive, "worker.resumed")
}

func (r *Registry) setWorkerStatus(id, status, eventType string) error {
	w, err := r.store.GetWorker(id)
	if err != nil {
		return err
	}
	if w.Status == status {
		return nil // idempotent: already in the target state
	}
	w.Status = status
	if err := r.store.UpdateWorker(w); err != nil {
		return err
	}
	r.bus.Publish(events.Event{
		Type:   eventType,
		Source: "registry",
		Payload: map[string]any{
			"worker_id": id,
			"status":    status,
		},
	})
	return nil
}
