package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kienbm/magic-claw/core/internal/protocol"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": "0.1.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func (g *Gateway) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	var payload protocol.RegisterPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	worker, err := g.registry.Register(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register worker")
		return
	}

	writeJSON(w, http.StatusCreated, worker)
}

func (g *Gateway) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, g.registry.ListWorkers())
}

func (g *Gateway) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var payload protocol.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := g.registry.Heartbeat(payload); err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) handleSubmitTask(w http.ResponseWriter, r *http.Request) {
	var task protocol.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusServiceUnavailable, "no worker available for task")
		return
	}

	g.store.AddTask(&task)

	// Copy for async dispatch to avoid race condition (H-04)
	taskCopy := task
	workerCopy := *worker
	go g.dispatcher.Dispatch(&taskCopy, &workerCopy)

	writeJSON(w, http.StatusCreated, task)
}

func (g *Gateway) handleGetStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, g.monitor.Stats())
}

type WorkflowRequest struct {
	Name    string                  `json:"name"`
	Steps   []protocol.WorkflowStep `json:"steps"`
	Context protocol.TaskContext    `json:"context"`
}

func (g *Gateway) handleSubmitWorkflow(w http.ResponseWriter, r *http.Request) {
	var req WorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wf, err := g.orchestrator.Submit(req.Name, req.Steps, req.Context)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow")
		return
	}

	writeJSON(w, http.StatusCreated, wf)
}

func (g *Gateway) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, g.orchestrator.ListWorkflows())
}

type CreateTeamRequest struct {
	Name        string  `json:"name"`
	OrgID       string  `json:"org_id"`
	DailyBudget float64 `json:"daily_budget"`
}

func (g *Gateway) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var req CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	team, err := g.orgMgr.CreateTeam(req.Name, req.OrgID, req.DailyBudget)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create team")
		return
	}

	writeJSON(w, http.StatusCreated, team)
}

func (g *Gateway) handleListTeams(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, g.orgMgr.ListTeams())
}

func (g *Gateway) handleCostReport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, g.costCtrl.OrgReport())
}

type AddKnowledgeRequest struct {
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Scope     string   `json:"scope"`
	ScopeID   string   `json:"scope_id"`
	CreatedBy string   `json:"created_by"`
}

func (g *Gateway) handleAddKnowledge(w http.ResponseWriter, r *http.Request) {
	var req AddKnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entry, err := g.knowledge.Add(req.Title, req.Content, req.Tags, req.Scope, req.ScopeID, req.CreatedBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add knowledge entry")
		return
	}

	writeJSON(w, http.StatusCreated, entry)
}

func (g *Gateway) handleSearchKnowledge(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var entries []*protocol.KnowledgeEntry
	if query != "" {
		entries = g.knowledge.Search(query)
	} else {
		entries = g.knowledge.List()
	}
	writeJSON(w, http.StatusOK, entries)
}
