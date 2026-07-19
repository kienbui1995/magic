package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/llm"
	"github.com/kienbui1995/magic/core/internal/memory"
	"github.com/kienbui1995/magic/core/internal/prompt"
	"github.com/kienbui1995/magic/core/internal/protocol"
)

// --- LLM Gateway ---

// aiContext extracts org ID and trace ID from request headers.
func aiContext(r *http.Request) (orgID, traceID string) {
	orgID = r.Header.Get("X-Org-ID")
	traceID = r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = r.Header.Get("Traceparent")
	}
	return
}

func (g *Gateway) handleLLMChat(w http.ResponseWriter, r *http.Request) {
	if g.deps.LLM == nil {
		writeError(w, http.StatusNotFound, "LLM gateway not configured")
		return
	}
	var req llm.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages required")
		return
	}
	resp, err := g.deps.LLM.Chat(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "LLM request failed")
		return
	}
	orgID, traceID := aiContext(r)
	g.deps.Bus.Publish(events.Event{
		Type: "llm.chat", Source: "llm",
		Payload: map[string]any{"model": resp.Model, "provider": resp.Provider, "cost": resp.Cost, "tokens": resp.Usage.TotalTokens, "org_id": orgID, "trace_id": traceID},
	})
	writeJSON(w, http.StatusOK, resp)
}

func (g *Gateway) handleLLMModels(w http.ResponseWriter, r *http.Request) {
	if g.deps.LLM == nil {
		writeError(w, http.StatusNotFound, "LLM gateway not configured")
		return
	}
	writeJSON(w, http.StatusOK, g.deps.LLM.ListModels())
}

// --- Prompts ---

func (g *Gateway) handleAddPrompt(w http.ResponseWriter, r *http.Request) {
	if g.deps.Prompts == nil {
		writeError(w, http.StatusNotFound, "prompt registry not configured")
		return
	}
	var req struct {
		Name     string            `json:"name"`
		Content  string            `json:"content"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "name and content required")
		return
	}
	tmpl := g.deps.Prompts.Add(req.Name, req.Content, req.Metadata)
	// Persist to store
	g.deps.Store.AddPrompt(r.Context(), &protocol.PromptTemplate{
		ID: tmpl.ID, Name: tmpl.Name, Version: tmpl.Version,
		Content: tmpl.Content, Metadata: tmpl.Metadata, CreatedAt: tmpl.CreatedAt,
	}) //nolint:errcheck
	g.deps.Bus.Publish(events.Event{
		Type: "prompt.created", Source: "prompt",
		Payload: map[string]any{"id": tmpl.ID, "name": tmpl.Name, "version": tmpl.Version},
	})
	writeJSON(w, http.StatusCreated, tmpl)
}

func (g *Gateway) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	if g.deps.Prompts == nil {
		writeError(w, http.StatusNotFound, "prompt registry not configured")
		return
	}
	limit, offset := getPagination(r)
	writeJSON(w, http.StatusOK, paginate(g.deps.Prompts.List(), limit, offset))
}

func (g *Gateway) handleRenderPrompt(w http.ResponseWriter, r *http.Request) {
	if g.deps.Prompts == nil {
		writeError(w, http.StatusNotFound, "prompt registry not configured")
		return
	}
	var req struct {
		Name string            `json:"name"`
		Vars map[string]string `json:"vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tmpl, err := g.deps.Prompts.Resolve(req.Name)
	if err != nil {
		writeError(w, http.StatusNotFound, "prompt template not found")
		return
	}
	rendered := prompt.Render(tmpl.Content, req.Vars)
	writeJSON(w, http.StatusOK, map[string]any{
		"template": tmpl,
		"rendered": rendered,
	})
}

// --- Agent Memory ---

func (g *Gateway) handleAddTurn(w http.ResponseWriter, r *http.Request) {
	if g.deps.Memory == nil {
		writeError(w, http.StatusNotFound, "memory not configured")
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		AgentID   string `json:"agent_id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionID == "" || req.Role == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "session_id, role, content required")
		return
	}
	g.deps.Memory.GetOrCreateSession(req.SessionID, req.AgentID, 50)
	g.deps.Memory.AddTurn(req.SessionID, memory.Turn{Role: req.Role, Content: req.Content})
	// Persist to store
	g.deps.Store.AddMemoryTurn(r.Context(), req.SessionID, &protocol.MemoryTurn{
		SessionID: req.SessionID, Role: req.Role, Content: req.Content, Timestamp: time.Now().UTC(),
	}) //nolint:errcheck
	g.deps.Bus.Publish(events.Event{
		Type: "memory.turn_added", Source: "memory",
		Payload: map[string]any{"session_id": req.SessionID, "role": req.Role},
	})
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (g *Gateway) handleGetTurns(w http.ResponseWriter, r *http.Request) {
	if g.deps.Memory == nil {
		writeError(w, http.StatusNotFound, "memory not configured")
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id query param required")
		return
	}
	// Verify caller owns this session via org header
	orgID, _ := aiContext(r)
	if orgID != "" {
		// Session IDs should be prefixed with org ID for isolation
		if len(sessionID) < len(orgID) || sessionID[:len(orgID)] != orgID {
			writeError(w, http.StatusForbidden, "session not accessible")
			return
		}
	}
	turns := g.deps.Memory.GetTurns(sessionID, 0)
	writeJSON(w, http.StatusOK, turns)
}

func (g *Gateway) handleAddMemoryEntry(w http.ResponseWriter, r *http.Request) {
	if g.deps.Memory == nil {
		writeError(w, http.StatusNotFound, "memory not configured")
		return
	}
	var req memory.VectorEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" || req.AgentID == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "id, agent_id, content required")
		return
	}
	g.deps.Memory.AddEntry(&req)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}
