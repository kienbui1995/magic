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

type WorkflowRequest struct {
	Name    string                  `json:"name"`
	Steps   []protocol.WorkflowStep `json:"steps"`
	Context protocol.TaskContext    `json:"context"`
}

func (g *Gateway) handleSubmitWorkflow(w http.ResponseWriter, r *http.Request) {
	var req WorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	wf, err := g.orchestrator.Submit(req.Name, req.Steps, req.Context)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wf)
}

func (g *Gateway) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows := g.orchestrator.ListWorkflows()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workflows)
}

type CreateTeamRequest struct {
	Name        string  `json:"name"`
	OrgID       string  `json:"org_id"`
	DailyBudget float64 `json:"daily_budget"`
}

func (g *Gateway) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var req CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	team, err := g.orgMgr.CreateTeam(req.Name, req.OrgID, req.DailyBudget)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(team)
}

func (g *Gateway) handleListTeams(w http.ResponseWriter, r *http.Request) {
	teams := g.orgMgr.ListTeams()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(teams)
}

func (g *Gateway) handleCostReport(w http.ResponseWriter, r *http.Request) {
	report := g.costCtrl.OrgReport()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
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
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	entry, err := g.knowledge.Add(req.Title, req.Content, req.Tags, req.Scope, req.ScopeID, req.CreatedBy)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

func (g *Gateway) handleSearchKnowledge(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var entries []*protocol.KnowledgeEntry
	if query != "" {
		entries = g.knowledge.Search(query)
	} else {
		entries = g.knowledge.List()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
