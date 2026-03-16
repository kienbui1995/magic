package gateway

import (
	"net/http"

	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/monitor"
	"github.com/kienbm/magic-claw/core/internal/registry"
	"github.com/kienbm/magic-claw/core/internal/router"
	"github.com/kienbm/magic-claw/core/internal/store"
)

type Gateway struct {
	registry *registry.Registry
	router   *router.Router
	store    store.Store
	bus      *events.Bus
	monitor  *monitor.Monitor
}

func New(reg *registry.Registry, rt *router.Router, s store.Store, bus *events.Bus, mon *monitor.Monitor) *Gateway {
	return &Gateway{
		registry: reg,
		router:   rt,
		store:    s,
		bus:      bus,
		monitor:  mon,
	}
}

func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", g.handleHealth)
	mux.HandleFunc("POST /api/v1/workers/register", g.handleRegisterWorker)
	mux.HandleFunc("POST /api/v1/workers/heartbeat", g.handleHeartbeat)
	mux.HandleFunc("GET /api/v1/workers", g.handleListWorkers)
	mux.HandleFunc("POST /api/v1/tasks", g.handleSubmitTask)
	mux.HandleFunc("GET /api/v1/metrics", g.handleGetStats)

	var handler http.Handler = mux
	handler = requestIDMiddleware(handler)
	handler = corsMiddleware(handler)

	return handler
}
