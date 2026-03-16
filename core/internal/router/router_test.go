package router_test

import (
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
