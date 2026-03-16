package gateway

import (
	"net/http"

	"github.com/kienbm/magic-claw/core/internal/costctrl"
	"github.com/kienbm/magic-claw/core/internal/evaluator"
	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/monitor"
	"github.com/kienbm/magic-claw/core/internal/orchestrator"
	"github.com/kienbm/magic-claw/core/internal/orgmgr"
	"github.com/kienbm/magic-claw/core/internal/registry"
	"github.com/kienbm/magic-claw/core/internal/router"
	"github.com/kienbm/magic-claw/core/internal/store"
)

type Gateway struct {
	registry     *registry.Registry
	router       *router.Router
	store        store.Store
	bus          *events.Bus
	monitor      *monitor.Monitor
	costCtrl     *costctrl.Controller
	evaluator    *evaluator.Evaluator
	orchestrator *orchestrator.Orchestrator
	orgMgr       *orgmgr.Manager
}

func New(reg *registry.Registry, rt *router.Router, s store.Store, bus *events.Bus, mon *monitor.Monitor, cc *costctrl.Controller, ev *evaluator.Evaluator, orch *orchestrator.Orchestrator, mgr *orgmgr.Manager) *Gateway {
	return &Gateway{
		registry:     reg,
		router:       rt,
		store:        s,
		bus:          bus,
		monitor:      mon,
		costCtrl:     cc,
		evaluator:    ev,
		orchestrator: orch,
		orgMgr:       mgr,
	}
}

func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /health", g.handleHealth)

	// Workers
	mux.HandleFunc("POST /api/v1/workers/register", g.handleRegisterWorker)
	mux.HandleFunc("POST /api/v1/workers/heartbeat", g.handleHeartbeat)
	mux.HandleFunc("GET /api/v1/workers", g.handleListWorkers)

	// Tasks
	mux.HandleFunc("POST /api/v1/tasks", g.handleSubmitTask)

	// Workflows
	mux.HandleFunc("POST /api/v1/workflows", g.handleSubmitWorkflow)
	mux.HandleFunc("GET /api/v1/workflows", g.handleListWorkflows)

	// Teams
	mux.HandleFunc("POST /api/v1/teams", g.handleCreateTeam)
	mux.HandleFunc("GET /api/v1/teams", g.handleListTeams)

	// Costs
	mux.HandleFunc("GET /api/v1/costs", g.handleCostReport)

	// Metrics
	mux.HandleFunc("GET /api/v1/metrics", g.handleGetStats)

	var handler http.Handler = mux
	handler = requestIDMiddleware(handler)
	handler = corsMiddleware(handler)

	return handler
}
