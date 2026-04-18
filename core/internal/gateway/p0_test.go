package gateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/costctrl"
	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/gateway"
	"github.com/kienbui1995/magic/core/internal/knowledge"
	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/orchestrator"
	"github.com/kienbui1995/magic/core/internal/orgmgr"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/store"
)

// setupGatewayWithStore mirrors setupGateway but also returns the backing
// store so tests can seed entities directly without going through HTTP.
func setupGatewayWithStore() (*gateway.Gateway, store.Store) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)
	mon := monitor.New(bus, os.Stderr)
	mon.Start()
	cc := costctrl.New(s, bus)
	ev := evaluator.New(bus)
	disp := dispatcher.New(s, bus, cc, ev)
	orch := orchestrator.New(s, rt, bus, disp)
	mgr := orgmgr.New(s, bus)
	kb := knowledge.New(s, bus, nil)
	gw := gateway.New(gateway.Deps{
		Registry:     reg,
		Router:       rt,
		Store:        s,
		Bus:          bus,
		Monitor:      mon,
		CostCtrl:     cc,
		Evaluator:    ev,
		Orchestrator: orch,
		OrgMgr:       mgr,
		Knowledge:    kb,
		Dispatcher:   disp,
	})
	return gw, s
}

// --- API versioning ---

func TestAPIVersion_ResponseHeader(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-API-Version"); got != protocol.ProtocolVersion {
		t.Errorf("X-API-Version: got %q, want %q", got, protocol.ProtocolVersion)
	}
}

func TestAPIVersion_AcceptsMatchingMajor(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Client sends 1.5 (minor ahead) — server is 1.0 — major matches → OK
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/health", nil)
	req.Header.Set("X-API-Version", "1.5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("same-major request: got %d, want 200", resp.StatusCode)
	}
}

func TestAPIVersion_RejectsDifferentMajor(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/health", nil)
	req.Header.Set("X-API-Version", "2.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("different-major request: got %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body) //nolint:errcheck
	if body["error"] != "incompatible api version" {
		t.Errorf("error code: got %q", body["error"])
	}
}

func TestHealth_ReportsProtocolVersion(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body) //nolint:errcheck
	if body["protocol_version"] != protocol.ProtocolVersion {
		t.Errorf("health protocol_version: got %v, want %q", body["protocol_version"], protocol.ProtocolVersion)
	}
}

// --- Task cancel ---

func TestCancelTask_NotFound(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/tasks/nonexistent/cancel", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("cancel nonexistent: got %d, want 404", resp.StatusCode)
	}
}

func TestCancelTask_Success(t *testing.T) {
	gw, s := setupGatewayWithStore()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Seed a pending task directly into the store — avoids the 503 from
	// handleSubmitTask when no workers are available.
	taskID := protocol.GenerateID("task")
	if err := s.AddTask(&protocol.Task{
		ID:        taskID,
		Type:      "nop",
		Priority:  protocol.PriorityNormal,
		Status:    protocol.TaskPending,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/tasks/"+taskID+"/cancel", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel: got %d, want 200", resp.StatusCode)
	}
	var task protocol.Task
	json.NewDecoder(resp.Body).Decode(&task) //nolint:errcheck
	if task.Status != protocol.TaskCancelled {
		t.Errorf("task status after cancel: got %q, want %q", task.Status, protocol.TaskCancelled)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set after cancel")
	}

	// Second cancel → 409 (already terminal)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("double cancel: got %d, want 409", resp2.StatusCode)
	}
}

// --- Worker pause/resume ---

func registerWorker(t *testing.T, srvURL, name string) string {
	t.Helper()
	p := protocol.RegisterPayload{
		Name:     name,
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9999"},
	}
	body, _ := json.Marshal(p)
	resp, err := http.Post(srvURL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: got %d", resp.StatusCode)
	}
	var out protocol.Worker
	json.NewDecoder(resp.Body).Decode(&out) //nolint:errcheck
	return out.ID
}

func TestPauseResumeWorker(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	id := registerWorker(t, srv.URL, "WorkerA")

	// Pause
	resp, err := http.Post(srv.URL+"/api/v1/workers/"+id+"/pause", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause: got %d, want 200", resp.StatusCode)
	}

	// Verify worker status is paused
	getResp, _ := http.Get(srv.URL + "/api/v1/workers/" + id)
	var worker protocol.Worker
	json.NewDecoder(getResp.Body).Decode(&worker) //nolint:errcheck
	if worker.Status != protocol.StatusPaused {
		t.Errorf("worker status after pause: got %q, want %q", worker.Status, protocol.StatusPaused)
	}

	// Resume
	resp2, err := http.Post(srv.URL+"/api/v1/workers/"+id+"/resume", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("resume: got %d, want 200", resp2.StatusCode)
	}

	// Idempotent resume
	resp3, _ := http.Post(srv.URL+"/api/v1/workers/"+id+"/resume", "application/json", nil)
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("idempotent resume: got %d", resp3.StatusCode)
	}
}

func TestPauseWorker_NotFound(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/workers/nonexistent/pause", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("pause nonexistent: got %d, want 404", resp.StatusCode)
	}
}

// --- Input validation ---

func TestValidation_RegisterWorker_MissingName(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body := []byte(`{"endpoint":{"url":"http://localhost:9000"}}`)
	resp, err := http.Post(srv.URL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing name: got %d, want 400", resp.StatusCode)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out) //nolint:errcheck
	if out["error"] != "validation_failed" {
		t.Errorf("error code: got %v", out["error"])
	}
	fields, _ := out["fields"].([]any)
	if len(fields) == 0 {
		t.Error("expected fields in validation error body")
	}
}

func TestValidation_SubmitTask_InvalidPriority(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body := []byte(`{"type":"greet","priority":"URGENT"}`)
	resp, err := http.Post(srv.URL+"/api/v1/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid priority: got %d, want 400", resp.StatusCode)
	}
}

func TestValidation_SubmitTask_MissingType(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body := []byte(`{"priority":"normal"}`)
	resp, err := http.Post(srv.URL+"/api/v1/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing type: got %d, want 400", resp.StatusCode)
	}
}

func TestValidation_CreateTeam_MissingOrgID(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body := []byte(`{"name":"T1"}`)
	resp, err := http.Post(srv.URL+"/api/v1/teams", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing org_id: got %d, want 400", resp.StatusCode)
	}
}
