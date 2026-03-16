package gateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/gateway"
	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/registry"
	"github.com/kienbm/magic-claw/core/internal/router"
	"github.com/kienbm/magic-claw/core/internal/store"
	"github.com/kienbm/magic-claw/core/internal/monitor"
)

func setupGateway() *gateway.Gateway {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)
	mon := monitor.New(bus, os.Stderr)
	mon.Start()
	return gateway.New(reg, rt, s, bus, mon)
}

func TestGateway_Health(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
}

func TestGateway_RegisterWorker(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	payload := protocol.RegisterPayload{
		Name:         "TestBot",
		Capabilities: []protocol.Capability{{Name: "greeting"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 5},
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(srv.URL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}

	var result protocol.Worker
	json.NewDecoder(resp.Body).Decode(&result)
	if result.ID == "" {
		t.Error("worker ID should not be empty")
	}
}

func TestGateway_ListWorkers(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	payload := protocol.RegisterPayload{
		Name:     "TestBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
	}
	body, _ := json.Marshal(payload)
	http.Post(srv.URL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))

	resp, _ := http.Get(srv.URL + "/api/v1/workers")
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d", resp.StatusCode)
	}

	var workers []*protocol.Worker
	json.NewDecoder(resp.Body).Decode(&workers)
	if len(workers) != 1 {
		t.Errorf("workers count: got %d, want 1", len(workers))
	}
}

func TestGateway_SubmitTask(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	regPayload := protocol.RegisterPayload{
		Name:         "GreetBot",
		Capabilities: []protocol.Capability{{Name: "greeting"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 5},
	}
	body, _ := json.Marshal(regPayload)
	http.Post(srv.URL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))

	taskReq := map[string]any{
		"type":  "greeting",
		"input": map[string]string{"name": "Kien"},
		"routing": map[string]any{
			"strategy":              "best_match",
			"required_capabilities": []string{"greeting"},
		},
		"contract": map[string]any{
			"timeout_ms": 30000,
			"max_cost":   1.0,
		},
	}
	body, _ = json.Marshal(taskReq)
	resp, err := http.Post(srv.URL+"/api/v1/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}

	var task protocol.Task
	json.NewDecoder(resp.Body).Decode(&task)
	if task.Status != protocol.TaskAssigned {
		t.Errorf("status: got %q, want assigned", task.Status)
	}
	if task.AssignedWorker == "" {
		t.Error("assigned_worker should not be empty")
	}
}
