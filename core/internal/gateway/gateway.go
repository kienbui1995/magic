package gateway

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"

	"github.com/kienbui1995/magic/core/internal/costctrl"
	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/knowledge"
	"github.com/kienbui1995/magic/core/internal/llm"
	"github.com/kienbui1995/magic/core/internal/memory"
	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/orchestrator"
	"github.com/kienbui1995/magic/core/internal/orgmgr"
	"github.com/kienbui1995/magic/core/internal/policy"
	"github.com/kienbui1995/magic/core/internal/prompt"
	"github.com/kienbui1995/magic/core/internal/rbac"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/store"
	"github.com/kienbui1995/magic/core/internal/webhook"
)

// Deps holds all dependencies for the Gateway.
type Deps struct {
	Registry     *registry.Registry
	Router       *router.Router
	Store        store.Store
	Bus          *events.Bus
	Monitor      *monitor.Monitor
	CostCtrl     *costctrl.Controller
	Evaluator    *evaluator.Evaluator
	Orchestrator *orchestrator.Orchestrator
	OrgMgr       *orgmgr.Manager
	Knowledge    *knowledge.Hub
	Dispatcher   *dispatcher.Dispatcher
	Webhook      *webhook.Manager
	RBAC         *rbac.Enforcer     // nil = no RBAC
	Policy       *policy.Engine     // nil = no policy enforcement
	ShutdownCtx  context.Context    // cancelled on server shutdown
	DispatchWG   *sync.WaitGroup    // tracks in-flight dispatches
	LLM          *llm.Gateway       // nil = LLM features disabled
	Prompts      *prompt.Registry   // nil = prompt features disabled
	Memory       *memory.Store      // nil = memory features disabled
}

// Gateway is the HTTP entry point for the MagiC server.
type Gateway struct {
	deps Deps
}

// New creates a new Gateway with the given dependencies.
func New(deps Deps) *Gateway {
	return &Gateway{deps: deps}
}

func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()

	// Rate limiters (token-bucket, per endpoint group)
	// Register: 10 req/IP/min → ~1 token per 6s, burst 5
	registerLimiter := newLimiterStore(rate.Every(6*time.Second), 5)
	// Heartbeat: 4 req/IP/min → ~1 token per 15s, burst 4
	heartbeatLimiter := newLimiterStore(rate.Every(15*time.Second), 4)
	// Token management: 20 req/org/min → ~1 token per 3s, burst 10
	tokenLimiter := newLimiterStore(rate.Every(3*time.Second), 10)
	// Task submit: 200 req/IP/min → ~1 token per 300ms, burst 20
	taskLimiter := newLimiterStore(rate.Every(300*time.Millisecond), 20)
	// Task submit per org: 200 req/org/min via X-Org-ID header
	orgTaskLimiter := newLimiterStore(rate.Every(300*time.Millisecond), 20)
	// LLM chat: 30 req/IP/min → ~1 token per 2s, burst 5 (costs real money)
	llmLimiter := newLimiterStore(rate.Every(2*time.Second), 5)

	registerRL := rateLimitMiddleware(registerLimiter, clientIP)
	heartbeatRL := rateLimitMiddleware(heartbeatLimiter, clientIP)
	tokenRL := rateLimitMiddleware(tokenLimiter, func(r *http.Request) string {
		return r.PathValue("orgID")
	})
	taskRL := rateLimitMiddleware(taskLimiter, clientIP)
	orgTaskRL := rateLimitMiddleware(orgTaskLimiter, func(r *http.Request) string {
		if orgID := r.Header.Get("X-Org-ID"); orgID != "" {
			return orgID
		}
		return clientIP(r)
	})
	llmRL := rateLimitMiddleware(llmLimiter, clientIP)

	// Prometheus metrics — no auth (Prometheus scrapers don't send Bearer tokens)
	mux.Handle("GET /metrics", promhttp.Handler())

	// Health
	mux.HandleFunc("GET /health", g.handleHealth)

	// Dashboard
	mux.HandleFunc("GET /dashboard", dashboardHandler)

	// Workers (protected by workerAuthMiddleware + per-endpoint rate limiting)
	workerAuth := workerAuthMiddleware(g.deps.Store)
	mux.Handle("POST /api/v1/workers/register",
		registerRL(workerAuth(http.HandlerFunc(g.handleRegisterWorker))))
	mux.Handle("POST /api/v1/workers/heartbeat",
		heartbeatRL(workerAuth(http.HandlerFunc(g.handleHeartbeat))))
	mux.HandleFunc("GET /api/v1/workers", g.handleListWorkers)
	mux.HandleFunc("GET /api/v1/workers/{id}", g.handleGetWorker)
	mux.Handle("DELETE /api/v1/workers/{id}", workerAuth(http.HandlerFunc(g.handleDeregisterWorker)))
	mux.Handle("POST /api/v1/workers/{id}/pause", workerAuth(http.HandlerFunc(g.handlePauseWorker)))
	mux.Handle("POST /api/v1/workers/{id}/resume", workerAuth(http.HandlerFunc(g.handleResumeWorker)))

	// Tasks
	mux.Handle("POST /api/v1/tasks", orgTaskRL(taskRL(http.HandlerFunc(g.handleSubmitTask))))
	mux.HandleFunc("GET /api/v1/tasks", g.handleListTasks)
	// Streaming tasks (must be before /tasks/{id} to avoid ambiguity)
	mux.Handle("POST /api/v1/tasks/stream", orgTaskRL(taskRL(http.HandlerFunc(g.handleStreamTask))))
	mux.HandleFunc("GET /api/v1/tasks/{id}/stream", g.handleResubscribeStream)
	mux.HandleFunc("POST /api/v1/tasks/{id}/cancel", g.handleCancelTask)
	mux.HandleFunc("GET /api/v1/tasks/{id}", g.handleGetTask)

	// Workflows
	mux.HandleFunc("POST /api/v1/workflows", g.handleSubmitWorkflow)
	mux.HandleFunc("GET /api/v1/workflows", g.handleListWorkflows)
	mux.HandleFunc("GET /api/v1/workflows/{id}", g.handleGetWorkflow)
	mux.HandleFunc("POST /api/v1/workflows/{id}/approve/{stepId}", g.handleApproveStep)
	mux.HandleFunc("POST /api/v1/workflows/{id}/cancel", g.handleCancelWorkflow)

	// Teams
	mux.HandleFunc("POST /api/v1/teams", g.handleCreateTeam)
	mux.HandleFunc("GET /api/v1/teams", g.handleListTeams)

	// Costs
	mux.HandleFunc("GET /api/v1/costs", g.handleCostReport)

	// Metrics
	mux.HandleFunc("GET /api/v1/metrics", g.handleGetStats)

	// Knowledge
	mux.HandleFunc("POST /api/v1/knowledge", g.handleAddKnowledge)
	mux.HandleFunc("GET /api/v1/knowledge", g.handleSearchKnowledge)
	mux.HandleFunc("POST /api/v1/knowledge/{id}/embedding", g.handleAddEmbedding)
	mux.HandleFunc("POST /api/v1/knowledge/search/semantic", g.handleSemanticSearch)

	// Token management (admin auth — MAGIC_API_KEY) + per-org rate limiting
	mux.Handle("POST /api/v1/orgs/{orgID}/tokens",
		tokenRL(http.HandlerFunc(g.handleCreateToken)))
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/tokens", g.handleListTokens)
	mux.Handle("DELETE /api/v1/orgs/{orgID}/tokens/{tokenID}",
		tokenRL(http.HandlerFunc(g.handleRevokeToken)))

	// Audit log (admin auth — MAGIC_API_KEY)
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/audit", g.handleQueryAudit)

	// Webhooks
	mux.HandleFunc("POST /api/v1/orgs/{orgID}/webhooks", g.handleCreateWebhook)
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/webhooks", g.handleListWebhooks)
	mux.HandleFunc("DELETE /api/v1/orgs/{orgID}/webhooks/{webhookID}", g.handleDeleteWebhook)
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/webhooks/{webhookID}/deliveries", g.handleListWebhookDeliveries)

	// RBAC: Role bindings
	mux.HandleFunc("POST /api/v1/orgs/{orgID}/roles", g.handleCreateRoleBinding)
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/roles", g.handleListRoleBindings)
	mux.HandleFunc("DELETE /api/v1/orgs/{orgID}/roles/{roleID}", g.handleDeleteRoleBinding)

	// Policies
	mux.HandleFunc("POST /api/v1/orgs/{orgID}/policies", g.handleCreatePolicy)
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/policies", g.handleListPolicies)
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/policies/{policyID}", g.handleGetPolicy)
	mux.HandleFunc("PUT /api/v1/orgs/{orgID}/policies/{policyID}", g.handleUpdatePolicy)
	mux.HandleFunc("DELETE /api/v1/orgs/{orgID}/policies/{policyID}", g.handleDeletePolicy)

	// Dead Letter Queue
	mux.HandleFunc("GET /api/v1/dlq", g.handleListDLQ)

	// LLM Gateway
	mux.Handle("POST /api/v1/llm/chat", llmRL(http.HandlerFunc(g.handleLLMChat)))
	mux.HandleFunc("GET /api/v1/llm/models", g.handleLLMModels)

	// Prompts
	mux.Handle("POST /api/v1/prompts", llmRL(http.HandlerFunc(g.handleAddPrompt)))
	mux.HandleFunc("GET /api/v1/prompts", g.handleListPrompts)
	mux.Handle("POST /api/v1/prompts/render", llmRL(http.HandlerFunc(g.handleRenderPrompt)))

	// Agent Memory
	mux.Handle("POST /api/v1/memory/turns", llmRL(http.HandlerFunc(g.handleAddTurn)))
	mux.HandleFunc("GET /api/v1/memory/turns", g.handleGetTurns)
	mux.Handle("POST /api/v1/memory/entries", llmRL(http.HandlerFunc(g.handleAddMemoryEntry)))

	var handler http.Handler = mux
	handler = rbacMiddleware(g.deps.RBAC)(handler)
	handler = requestIDMiddleware(handler)
	handler = bodySizeMiddleware(handler)
	handler = authMiddleware(handler)
	handler = apiVersionMiddleware(handler)
	handler = securityHeadersMiddleware(handler)
	handler = corsMiddleware(handler)

	return handler
}
