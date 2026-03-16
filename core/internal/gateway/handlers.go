package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kienbm/magic-claw/core/internal/protocol"
)

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"version": "0.1.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func (g *Gateway) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	var payload protocol.RegisterPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	worker, err := g.registry.Register(payload)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(worker)
}

func (g *Gateway) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	workers := g.registry.ListWorkers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workers)
}

func (g *Gateway) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var payload protocol.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}
	if err := g.registry.Heartbeat(payload); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (g *Gateway) handleSubmitTask(w http.ResponseWriter, r *http.Request) {
	var task protocol.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	task.ID = protocol.GenerateID("task")
	task.Status = protocol.TaskPending
	task.CreatedAt = time.Now()

	if task.Priority == "" {
		task.Priority = protocol.PriorityNormal
	}

	worker, err := g.router.RouteTask(&task)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusServiceUnavailable)
		return
	}

	_ = worker

	g.store.AddTask(&task)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func (g *Gateway) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := g.monitor.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
