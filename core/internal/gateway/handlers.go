package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// validateWebhookURL blocks private/internal IPs to prevent SSRF.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	host := u.Hostname()
	if host == "metadata.google.internal" || host == "169.254.169.254" {
		return fmt.Errorf("URL must not point to cloud metadata service")
	}
	// Check literal IP
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("URL must not point to private/internal network")
		}
		return nil
	}
	// Resolve hostname to catch DNS rebinding
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil // DNS failure at creation time is OK
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("hostname resolves to private/internal IP")
		}
	}
	return nil
}

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
		"status":           "ok",
		"protocol_version": protocol.ProtocolVersion,
		"time":             time.Now().Format(time.RFC3339),
	})
}

func (g *Gateway) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	var payload protocol.RegisterPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if errs := validateRequest(
		required("name", payload.Name),
		maxLen("name", payload.Name, 255),
		required("endpoint.url", payload.Endpoint.URL),
	); len(errs) > 0 {
		writeValidationError(w, errs)
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

	if errs := validateRequest(
		required("type", task.Type),
		maxLen("type", task.Type, 255),
		oneOf("priority", task.Priority,
			protocol.PriorityLow, protocol.PriorityNormal,
			protocol.PriorityHigh, protocol.PriorityCritical),
	); len(errs) > 0 {
		writeValidationError(w, errs)
		return
	}

	task.ID = protocol.GenerateID("task")
	task.Status = protocol.TaskPending
	task.CreatedAt = time.Now()

	if task.TraceID == "" {
		task.TraceID = protocol.GenerateID("trace")
	}

	if task.Priority == "" {
		task.Priority = protocol.PriorityNormal
	}

	// Policy enforcement: check org policies before routing
	if g.deps.Policy != nil {
		result := g.deps.Policy.Enforce(&task)
		if !result.Allowed {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":      "policy violation",
				"violations": result.Violations,
			})
			return
		}
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
	if g.deps.DispatchWG != nil {
		g.deps.DispatchWG.Add(1)
	}
	go func() {
		if g.deps.DispatchWG != nil {
			defer g.deps.DispatchWG.Done()
		}
		ctx := context.Background()
		if g.deps.ShutdownCtx != nil {
			ctx = g.deps.ShutdownCtx
		}
		g.deps.Dispatcher.Dispatch(ctx, &taskCopy, &workerCopy) //nolint:errcheck
	}()

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

// handleCancelTask marks a task as cancelled if it is not already in a terminal state.
// Returns 404 if the task does not exist, 409 if already terminal.
//
// Note: if the task is currently being dispatched, the worker will not be
// actively stopped — the dispatcher will complete its HTTP roundtrip, but the
// final UpdateTask call from the dispatcher may overwrite the cancelled status
// if it races. Hard cancellation of in-flight work requires worker cooperation
// and is out of scope for this endpoint.
func (g *Gateway) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := g.deps.Store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if protocol.IsTaskTerminal(task.Status) {
		writeError(w, http.StatusConflict, "task already in terminal state: "+task.Status)
		return
	}
	task.Status = protocol.TaskCancelled
	now := time.Now()
	task.CompletedAt = &now
	if task.Error == nil {
		task.Error = &protocol.TaskError{Code: "cancelled", Message: "cancelled by user"}
	}
	if err := g.deps.Store.UpdateTask(task); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel task")
		return
	}
	g.deps.Bus.Publish(events.Event{
		Type:   "task.cancelled",
		Source: "gateway",
		Payload: map[string]any{
			"task_id":   task.ID,
			"worker_id": task.AssignedWorker,
		},
	})
	writeJSON(w, http.StatusOK, task)
}

// handlePauseWorker transitions a worker to the paused state. Paused workers
// are skipped by the router when selecting targets for new tasks.
func (g *Gateway) handlePauseWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if token := TokenFromContext(r.Context()); token != nil && token.WorkerID != id {
		writeError(w, http.StatusForbidden, "token not authorized for this worker")
		return
	}
	if err := g.deps.Registry.PauseWorker(id); err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": protocol.StatusPaused})
}

// handleResumeWorker transitions a paused worker back to active.
func (g *Gateway) handleResumeWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if token := TokenFromContext(r.Context()); token != nil && token.WorkerID != id {
		writeError(w, http.StatusForbidden, "token not authorized for this worker")
		return
	}
	if err := g.deps.Registry.ResumeWorker(id); err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": protocol.StatusActive})
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

	if wf.TraceID == "" {
		wf.TraceID = protocol.GenerateID("trace")
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

	if errs := validateRequest(
		required("name", req.Name),
		maxLen("name", req.Name, 255),
		required("org_id", req.OrgID),
	); len(errs) > 0 {
		writeValidationError(w, errs)
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
	limit, offset := getPagination(r)
	tokens := g.deps.Store.ListWorkerTokensByOrg(orgID)
	writeJSON(w, http.StatusOK, paginate(tokens, limit, offset))
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
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{req.Type},
		},
	}

	if task.TraceID == "" {
		task.TraceID = protocol.GenerateID("trace")
	}

	// Policy enforcement
	if g.deps.Policy != nil {
		result := g.deps.Policy.Enforce(task)
		if !result.Allowed {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":      "policy violation",
				"violations": result.Violations,
			})
			return
		}
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

	monitor.MetricStreamsActive.Inc()
	streamStart := time.Now()
	defer func() {
		monitor.MetricStreamsActive.Dec()
		monitor.MetricStreamDuration.Observe(time.Since(streamStart).Seconds())
	}()

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
	if errs := validateRequest(
		required("url", req.URL),
		nonEmptySlice("events", req.Events),
	); len(errs) > 0 {
		writeValidationError(w, errs)
		return
	}
	if err := validateWebhookURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid webhook URL: %v", err))
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
	limit, offset := getPagination(r)
	writeJSON(w, http.StatusOK, paginate(g.deps.Webhook.ListWebhooks(orgID), limit, offset))
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
	limit, offset := getPagination(r)
	writeJSON(w, http.StatusOK, paginate(g.deps.Webhook.ListDeliveries(webhookID), limit, offset))
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

// --- RBAC: Role Bindings ---

// POST /api/v1/orgs/{orgID}/roles
func (g *Gateway) handleCreateRoleBinding(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	var req struct {
		Subject string `json:"subject"`
		Role    string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Subject == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "subject and role are required")
		return
	}
	if req.Role != protocol.RoleOwner && req.Role != protocol.RoleAdmin && req.Role != protocol.RoleViewer {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, or viewer")
		return
	}
	// Check if binding already exists
	if existing, err := g.deps.Store.FindRoleBinding(orgID, req.Subject); err == nil {
		writeJSON(w, http.StatusConflict, existing)
		return
	}
	rb := &protocol.RoleBinding{
		ID:        protocol.GenerateID("rb"),
		OrgID:     orgID,
		Subject:   req.Subject,
		Role:      req.Role,
		CreatedAt: time.Now(),
	}
	if err := g.deps.Store.AddRoleBinding(rb); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create role binding")
		return
	}
	writeJSON(w, http.StatusCreated, rb)
}

// GET /api/v1/orgs/{orgID}/roles
func (g *Gateway) handleListRoleBindings(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	limit, offset := getPagination(r)
	writeJSON(w, http.StatusOK, paginate(g.deps.Store.ListRoleBindingsByOrg(orgID), limit, offset))
}

// DELETE /api/v1/orgs/{orgID}/roles/{roleID}
func (g *Gateway) handleDeleteRoleBinding(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	roleID := r.PathValue("roleID")
	rb, err := g.deps.Store.GetRoleBinding(roleID)
	if err != nil || rb.OrgID != orgID {
		writeError(w, http.StatusNotFound, "role binding not found")
		return
	}
	if err := g.deps.Store.RemoveRoleBinding(roleID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete role binding")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Policy CRUD ---

// POST /api/v1/orgs/{orgID}/policies
func (g *Gateway) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	var req struct {
		Name    string               `json:"name"`
		Rules   []protocol.PolicyRule `json:"rules"`
		Enabled bool                 `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Rules) == 0 {
		writeError(w, http.StatusBadRequest, "name and rules are required")
		return
	}
	p := &protocol.Policy{
		ID:        protocol.GenerateID("pol"),
		OrgID:     orgID,
		Name:      req.Name,
		Rules:     req.Rules,
		Enabled:   req.Enabled,
		CreatedAt: time.Now(),
	}
	if err := g.deps.Store.AddPolicy(p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create policy")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// GET /api/v1/orgs/{orgID}/policies
func (g *Gateway) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	limit, offset := getPagination(r)
	writeJSON(w, http.StatusOK, paginate(g.deps.Store.ListPoliciesByOrg(orgID), limit, offset))
}

// GET /api/v1/orgs/{orgID}/policies/{policyID}
func (g *Gateway) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	policyID := r.PathValue("policyID")
	p, err := g.deps.Store.GetPolicy(policyID)
	if err != nil || p.OrgID != orgID {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// PUT /api/v1/orgs/{orgID}/policies/{policyID}
func (g *Gateway) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	policyID := r.PathValue("policyID")
	existing, err := g.deps.Store.GetPolicy(policyID)
	if err != nil || existing.OrgID != orgID {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}
	var req struct {
		Name    *string               `json:"name"`
		Rules   []protocol.PolicyRule  `json:"rules"`
		Enabled *bool                 `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Rules != nil {
		existing.Rules = req.Rules
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if err := g.deps.Store.UpdatePolicy(existing); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update policy")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// DELETE /api/v1/orgs/{orgID}/policies/{policyID}
func (g *Gateway) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	policyID := r.PathValue("policyID")
	p, err := g.deps.Store.GetPolicy(policyID)
	if err != nil || p.OrgID != orgID {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}
	if err := g.deps.Store.RemovePolicy(policyID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete policy")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (g *Gateway) handleListDLQ(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)
	all := g.deps.Store.ListDLQ()
	writeJSON(w, http.StatusOK, paginate(all, limit, offset))
}
