package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

func getPagination(r *http.Request) (limit, offset int) {
	limit = 100
	offset = 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	return
}

func paginate[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
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

	worker, err := g.deps.Registry.Register(payload)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "token already in use"):
			writeError(w, http.StatusConflict, "token already in use")
		case strings.Contains(msg, "token"):
			writeError(w, http.StatusUnauthorized, "invalid worker token")
		default:
			writeError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}

	writeJSON(w, http.StatusCreated, worker)
}

func (g *Gateway) handleGetWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	worker, err := g.deps.Registry.GetWorker(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

func (g *Gateway) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)
	workers := g.deps.Registry.ListWorkers()
	writeJSON(w, http.StatusOK, paginate(workers, limit, offset))
}

func (g *Gateway) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var payload protocol.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := g.deps.Registry.Heartbeat(payload); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not authorized"):
			writeError(w, http.StatusForbidden, "token not authorized for this worker")
		case strings.Contains(msg, "token"):
			writeError(w, http.StatusUnauthorized, "invalid worker token")
		default:
			writeError(w, http.StatusNotFound, "worker not found")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) handleDeregisterWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// In security mode, verify the caller's token is bound to this worker.
	if token := TokenFromContext(r.Context()); token != nil && token.WorkerID != id {
		writeError(w, http.StatusForbidden, "token not authorized for this worker")
		return
	}
	if err := g.deps.Registry.Deregister(id); err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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

	worker, err := g.deps.Router.RouteTask(&task)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "no worker available for task")
		return
	}

	g.deps.Store.AddTask(&task) //nolint:errcheck

	// Copy for async dispatch to avoid race condition (H-04)
	taskCopy := task
	workerCopy := *worker
	go g.deps.Dispatcher.Dispatch(context.Background(), &taskCopy, &workerCopy) //nolint:errcheck

	writeJSON(w, http.StatusCreated, task)
}

func (g *Gateway) handleListTasks(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)
	tasks := g.deps.Store.ListTasks()
	writeJSON(w, http.StatusOK, paginate(tasks, limit, offset))
}

func (g *Gateway) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := g.deps.Store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (g *Gateway) handleGetStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, g.deps.Monitor.Stats())
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

	wf, err := g.deps.Orchestrator.Submit(req.Name, req.Steps, req.Context)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow")
		return
	}

	writeJSON(w, http.StatusCreated, wf)
}

func (g *Gateway) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf, err := g.deps.Orchestrator.GetWorkflow(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (g *Gateway) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)
	workflows := g.deps.Orchestrator.ListWorkflows()
	writeJSON(w, http.StatusOK, paginate(workflows, limit, offset))
}

func (g *Gateway) handleApproveStep(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	stepID := r.PathValue("stepId")
	if err := g.deps.Orchestrator.ApproveStep(workflowID, stepID); err != nil {
		writeError(w, http.StatusBadRequest, "approval failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (g *Gateway) handleCancelWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := g.deps.Orchestrator.CancelWorkflow(id); err != nil {
		writeError(w, http.StatusBadRequest, "cancel failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
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

	team, err := g.deps.OrgMgr.CreateTeam(req.Name, req.OrgID, req.DailyBudget)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create team")
		return
	}

	writeJSON(w, http.StatusCreated, team)
}

func (g *Gateway) handleListTeams(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)
	teams := g.deps.OrgMgr.ListTeams()
	writeJSON(w, http.StatusOK, paginate(teams, limit, offset))
}

func (g *Gateway) handleCostReport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, g.deps.CostCtrl.OrgReport())
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

	entry, err := g.deps.Knowledge.Add(req.Title, req.Content, req.Tags, req.Scope, req.ScopeID, req.CreatedBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add knowledge entry")
		return
	}

	writeJSON(w, http.StatusCreated, entry)
}

func (g *Gateway) handleSearchKnowledge(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)
	query := r.URL.Query().Get("q")
	var entries []*protocol.KnowledgeEntry
	if query != "" {
		entries = g.deps.Knowledge.Search(query)
	} else {
		entries = g.deps.Knowledge.List()
	}
	writeJSON(w, http.StatusOK, paginate(entries, limit, offset))
}

func (g *Gateway) handleAddEmbedding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Vector   []float32      `json:"vector"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Vector) == 0 {
		writeError(w, http.StatusBadRequest, "vector is required")
		return
	}
	if err := g.deps.Knowledge.AddEmbedding(id, req.Vector, req.Metadata); err != nil {
		if strings.Contains(err.Error(), "pgvector") {
			writeError(w, http.StatusNotImplemented, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to store embedding")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueryVector []float32 `json:"query_vector"`
		TopK        int       `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.QueryVector) == 0 {
		writeError(w, http.StatusBadRequest, "query_vector is required")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 10
	}
	results, err := g.deps.Knowledge.SemanticSearch(req.QueryVector, req.TopK)
	if err != nil {
		if strings.Contains(err.Error(), "pgvector") {
			writeError(w, http.StatusNotImplemented, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "semantic search failed")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// createTokenRequest is the body for POST /api/v1/orgs/{orgID}/tokens.
type createTokenRequest struct {
	Name           string `json:"name"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

// handleCreateToken creates a new worker token for an org.
// POST /api/v1/orgs/{orgID}/tokens
func (g *Gateway) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Name) == 0 || len(req.Name) > 255 {
		writeError(w, http.StatusBadRequest, "token name must be 1-255 characters")
		return
	}
	if req.ExpiresInHours < 0 {
		writeError(w, http.StatusBadRequest, "expires_in_hours must be non-negative")
		return
	}

	raw, hash := protocol.GenerateToken()

	now := time.Now()
	token := &protocol.WorkerToken{
		ID:        protocol.GenerateID("token"),
		OrgID:     orgID,
		TokenHash: hash,
		Name:      req.Name,
		CreatedAt: now,
	}
	if req.ExpiresInHours > 0 {
		exp := now.Add(time.Duration(req.ExpiresInHours) * time.Hour)
		token.ExpiresAt = &exp
	}

	if err := g.deps.Store.AddWorkerToken(token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	reqID := w.Header().Get("X-Request-ID")
	_ = g.deps.Store.AppendAudit(&protocol.AuditEntry{
		ID:        protocol.GenerateID("audit"),
		Timestamp: time.Now(),
		OrgID:     orgID,
		Action:    "token.create",
		Resource:  "token/" + token.ID,
		RequestID: reqID,
		Outcome:   "success",
		Detail:    map[string]any{"token_id": token.ID, "name": token.Name},
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      raw,
		"id":         token.ID,
		"org_id":     token.OrgID,
		"name":       token.Name,
		"expires_at": token.ExpiresAt,
		"created_at": token.CreatedAt,
	})
}

// handleListTokens lists tokens for an org (without raw values or hashes).
// GET /api/v1/orgs/{orgID}/tokens
func (g *Gateway) handleListTokens(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	tokens := g.deps.Store.ListWorkerTokensByOrg(orgID)
	writeJSON(w, http.StatusOK, tokens)
}

// handleRevokeToken revokes a token by ID.
// DELETE /api/v1/orgs/{orgID}/tokens/{tokenID}
func (g *Gateway) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	tokenID := r.PathValue("tokenID")

	token, err := g.deps.Store.GetWorkerToken(tokenID)
	if err != nil {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}

	// Security: ensure the token belongs to this org
	if token.OrgID != orgID {
		writeError(w, http.StatusForbidden, "token does not belong to this org")
		return
	}

	now := time.Now()
	token.RevokedAt = &now
	if err := g.deps.Store.UpdateWorkerToken(token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}

	reqID := w.Header().Get("X-Request-ID")
	_ = g.deps.Store.AppendAudit(&protocol.AuditEntry{
		ID:        protocol.GenerateID("audit"),
		Timestamp: time.Now(),
		OrgID:     orgID,
		Action:    "token.revoke",
		Resource:  "token/" + tokenID,
		RequestID: reqID,
		Outcome:   "success",
		Detail:    map[string]any{"token_id": tokenID},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "revoked",
		"token_id":   tokenID,
		"revoked_at": now,
	})
}

// handleQueryAudit returns audit log entries for an org.
// GET /api/v1/orgs/{orgID}/audit
func (g *Gateway) handleQueryAudit(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	q := r.URL.Query()

	limit := 100
	offset := 0
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	filter := store.AuditFilter{
		OrgID:    orgID,
		WorkerID: q.Get("worker_id"),
		Action:   q.Get("action"),
		Limit:    limit,
		Offset:   offset,
	}

	if s := q.Get("start"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.StartTime = &t
		}
	}
	if e := q.Get("end"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			filter.EndTime = &t
		}
	}

	// Get total count (no pagination)
	countFilter := filter
	countFilter.Limit = 0
	countFilter.Offset = 0
	allEntries := g.deps.Store.QueryAudit(countFilter)
	total := len(allEntries)

	// Get paginated page
	entries := g.deps.Store.QueryAudit(filter)
	if entries == nil {
		entries = []*protocol.AuditEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// handleStreamTask submits a task and streams the result back via SSE.
// POST /api/v1/tasks/stream
func (g *Gateway) handleStreamTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string               `json:"type"`
		Input   json.RawMessage      `json:"input"`
		Context protocol.TaskContext `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}

	task := &protocol.Task{
		ID:       protocol.GenerateID("t"),
		Type:     req.Type,
		Status:   protocol.TaskPending,
		Input:    req.Input,
		Context:  req.Context,
		Priority: protocol.PriorityNormal,
	}

	// Route to a worker (populates task.AssignedWorker and sets status to TaskAssigned)
	worker, err := g.deps.Router.RouteTask(task)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "no worker available: "+err.Error())
		return
	}

	if err := g.deps.Store.AddTask(task); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	// Set SSE headers and remove write deadline for streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		// Not all ResponseWriters support this — log but continue
		_ = err
	}

	if err := g.deps.Dispatcher.DispatchStream(r.Context(), task, worker, w); err != nil {
		// Write SSE error event — headers already sent, so can't change status code
		fmt.Fprintf(w, "data: {\"error\":%q,\"done\":true}\n\n", err.Error())
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// handleCreateWebhook registers a new webhook for an org.
// POST /api/v1/orgs/{orgID}/webhooks
func (g *Gateway) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Secret string   `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" || len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "url and events are required")
		return
	}
	hook, err := g.deps.Webhook.CreateWebhook(orgID, req.URL, req.Events, req.Secret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}
	hook.Secret = "" // never return secret
	writeJSON(w, http.StatusCreated, hook)
}

// handleListWebhooks returns all webhooks for an org.
// GET /api/v1/orgs/{orgID}/webhooks
func (g *Gateway) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	writeJSON(w, http.StatusOK, g.deps.Webhook.ListWebhooks(orgID))
}

// handleDeleteWebhook removes a webhook by ID.
// DELETE /api/v1/orgs/{orgID}/webhooks/{webhookID}
func (g *Gateway) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	webhookID := r.PathValue("webhookID")

	// Verify org ownership before deleting
	hook, err := g.deps.Store.GetWebhook(webhookID)
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if hook.OrgID != orgID {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}

	if err := g.deps.Webhook.DeleteWebhook(webhookID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListWebhookDeliveries returns deliveries for a webhook.
// GET /api/v1/orgs/{orgID}/webhooks/{webhookID}/deliveries
func (g *Gateway) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	webhookID := r.PathValue("webhookID")
	writeJSON(w, http.StatusOK, g.deps.Webhook.ListDeliveries(webhookID))
}

// handleResubscribeStream returns the result of a completed/failed task as a single SSE event.
// GET /api/v1/tasks/{id}/stream
func (g *Gateway) handleResubscribeStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := g.deps.Store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Time{}) //nolint:errcheck

	switch task.Status {
	case protocol.TaskCompleted:
		output, _ := json.Marshal(task.Output)
		fmt.Fprintf(w, "data: {\"chunk\":%s,\"task_id\":%q,\"done\":true}\n\n", output, id)
	case protocol.TaskFailed:
		msg := "task failed"
		if task.Error != nil {
			msg = task.Error.Message
		}
		fmt.Fprintf(w, "data: {\"error\":%q,\"done\":true}\n\n", msg)
	default:
		writeError(w, http.StatusAccepted, "task is still running; poll GET /api/v1/tasks/"+id)
		return
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
