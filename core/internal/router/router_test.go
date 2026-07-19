package router_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/store"
)

func setupRouter(t *testing.T) (*router.Router, *registry.Registry) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)

	reg.Register(protocol.RegisterPayload{
		Name:         "ContentBot",
		Capabilities: []protocol.Capability{{Name: "content_writing", EstCostPerCall: 0.05}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 5},
	})
	reg.Register(protocol.RegisterPayload{
		Name:         "CheapBot",
		Capabilities: []protocol.Capability{{Name: "content_writing", EstCostPerCall: 0.01}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9002"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 5},
	})

	return rt, reg
}

func TestRouter_RouteTask_BestMatch(t *testing.T) {
	rt, _ := setupRouter(t)

	task := &protocol.Task{
		ID:    protocol.GenerateID("task"),
		Type:  "content_writing",
		Input: json.RawMessage(`{"topic": "test"}`),
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{"content_writing"},
		},
		Contract: protocol.Contract{TimeoutMs: 30000, MaxCost: 1.0},
	}

	worker, err := rt.RouteTask(task)
	if err != nil {
		t.Fatalf("RouteTask: %v", err)
	}
	if worker == nil {
		t.Fatal("worker should not be nil")
	}
}

func TestRouter_RouteTask_NoCapableWorker(t *testing.T) {
	rt, _ := setupRouter(t)

	task := &protocol.Task{
		ID:   protocol.GenerateID("task"),
		Type: "data_analysis",
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{"data_analysis"},
		},
	}

	_, err := rt.RouteTask(task)
	if err == nil {
		t.Error("should fail — no worker with data_analysis capability")
	}
}

func TestRouter_RouteTask_Cheapest(t *testing.T) {
	rt, _ := setupRouter(t)

	task := &protocol.Task{
		ID:   protocol.GenerateID("task"),
		Type: "content_writing",
		Routing: protocol.RoutingConfig{
			Strategy:             "cheapest",
			RequiredCapabilities: []string{"content_writing"},
		},
	}

	worker, err := rt.RouteTask(task)
	if err != nil {
		t.Fatalf("RouteTask: %v", err)
	}
	if worker.Name != "CheapBot" {
		t.Errorf("cheapest should pick CheapBot, got %q", worker.Name)
	}
}

// setupRouterWithStore creates a router, registry and store for org isolation tests.
func setupRouterWithStore(t *testing.T) (*router.Router, *registry.Registry, store.Store) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)
	return rt, reg, s
}

// makeWorker is a helper to create a worker with OrgID and capability set.
func makeWorker(name, orgID, capability string) *protocol.Worker {
	return &protocol.Worker{
		ID:    protocol.GenerateID("worker"),
		Name:  name,
		OrgID: orgID,
		Capabilities: []protocol.Capability{
			{Name: capability, EstCostPerCall: 0.05},
		},
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
		Limits:   protocol.WorkerLimits{MaxConcurrentTasks: 5},
		Status:   protocol.StatusActive,
	}
}

func TestRouteTask_OrgIsolation(t *testing.T) {
	rt, _, s := setupRouterWithStore(t)

	workerA := makeWorker("BotA", "org_a", "content_writing")
	workerB := makeWorker("BotB", "org_b", "content_writing")
	s.AddWorker(context.Background(), workerA)
	s.AddWorker(context.Background(), workerB)

	task := &protocol.Task{
		ID:   protocol.GenerateID("task"),
		Type: "content_writing",
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{"content_writing"},
		},
		Context: protocol.TaskContext{OrgID: "org_a"},
	}

	selected, err := rt.RouteTask(task)
	if err != nil {
		t.Fatalf("RouteTask: %v", err)
	}
	if selected.OrgID != "org_a" {
		t.Errorf("expected worker from org_a, got OrgID=%q (worker=%q)", selected.OrgID, selected.Name)
	}
	if selected.Name != "BotA" {
		t.Errorf("expected BotA, got %q", selected.Name)
	}
}

func TestRouteTask_OrgIsolation_NoWorkers(t *testing.T) {
	rt, _, s := setupRouterWithStore(t)

	workerB := makeWorker("BotB", "org_b", "content_writing")
	s.AddWorker(context.Background(), workerB)

	task := &protocol.Task{
		ID:   protocol.GenerateID("task"),
		Type: "content_writing",
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{"content_writing"},
		},
		Context: protocol.TaskContext{OrgID: "org_a"},
	}

	_, err := rt.RouteTask(task)
	if err == nil {
		t.Error("expected ErrNoWorkerAvailable, got nil")
	}
	if err != router.ErrNoWorkerAvailable {
		t.Errorf("expected ErrNoWorkerAvailable, got %v", err)
	}
}

func TestRouteTask_NoOrgID_RoutesAll(t *testing.T) {
	rt, _, s := setupRouterWithStore(t)

	workerA := makeWorker("BotA", "org_a", "content_writing")
	workerB := makeWorker("BotB", "org_b", "content_writing")
	s.AddWorker(context.Background(), workerA)
	s.AddWorker(context.Background(), workerB)

	task := &protocol.Task{
		ID:   protocol.GenerateID("task"),
		Type: "content_writing",
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{"content_writing"},
		},
		// Context.OrgID intentionally empty — dev mode
	}

	selected, err := rt.RouteTask(task)
	if err != nil {
		t.Fatalf("RouteTask: %v", err)
	}
	if selected == nil {
		t.Fatal("expected a worker to be selected")
	}
}

// roundRobinStrategy is a custom plugin that picks workers in round-robin order.
type roundRobinStrategy struct{ idx int }

func (s *roundRobinStrategy) Name() string { return "round_robin" }
func (s *roundRobinStrategy) Select(candidates []*protocol.Worker, _ *protocol.Task) *protocol.Worker {
	if len(candidates) == 0 {
		return nil
	}
	w := candidates[s.idx%len(candidates)]
	s.idx++
	return w
}

func TestRouter_CustomStrategyPlugin(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)

	// Register custom plugin
	rr := &roundRobinStrategy{}
	rt.RegisterStrategy(rr)

	reg.Register(protocol.RegisterPayload{
		Name:         "BotA",
		Capabilities: []protocol.Capability{{Name: "writing"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 10},
	})
	reg.Register(protocol.RegisterPayload{
		Name:         "BotB",
		Capabilities: []protocol.Capability{{Name: "writing"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9002"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 10},
	})

	names := make([]string, 2)
	for i := range names {
		task := &protocol.Task{
			ID:   protocol.GenerateID("task"),
			Type: "writing",
			Routing: protocol.RoutingConfig{
				Strategy:             "round_robin",
				RequiredCapabilities: []string{"writing"},
			},
		}
		w, err := rt.RouteTask(task)
		if err != nil {
			t.Fatalf("RouteTask[%d]: %v", i, err)
		}
		names[i] = w.Name
	}

	if names[0] == names[1] {
		t.Errorf("round_robin should alternate workers, got %q and %q", names[0], names[1])
	}
}

func TestRouteTask_OrgIsolation_MultipleWorkers(t *testing.T) {
	rt, _, s := setupRouterWithStore(t)

	// Two org_a workers with different loads; one org_b worker
	workerA1 := makeWorker("BotA1", "org_a", "content_writing")
	workerA1.CurrentLoad = 0
	workerA2 := makeWorker("BotA2", "org_a", "content_writing")
	workerA2.CurrentLoad = 3
	workerB := makeWorker("BotB", "org_b", "content_writing")
	s.AddWorker(context.Background(), workerA1)
	s.AddWorker(context.Background(), workerA2)
	s.AddWorker(context.Background(), workerB)

	task := &protocol.Task{
		ID:   protocol.GenerateID("task"),
		Type: "content_writing",
		Routing: protocol.RoutingConfig{
			Strategy:             "best_match",
			RequiredCapabilities: []string{"content_writing"},
		},
		Context: protocol.TaskContext{OrgID: "org_a"},
	}

	selected, err := rt.RouteTask(task)
	if err != nil {
		t.Fatalf("RouteTask: %v", err)
	}
	if selected.OrgID != "org_a" {
		t.Errorf("expected worker from org_a, got OrgID=%q", selected.OrgID)
	}
	// best_match picks the worker with most availability (lowest load)
	if selected.Name != "BotA1" {
		t.Errorf("expected BotA1 (lower load), got %q", selected.Name)
	}
}
