package gateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func setupGateway() *gateway.Gateway {
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
	kb := knowledge.New(s, bus)
	return gateway.New(gateway.Deps{
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

func TestGateway_ListTasks(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Register a worker
	regPayload := protocol.RegisterPayload{
		Name:         "Bot",
		Capabilities: []protocol.Capability{{Name: "greeting"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 5},
	}
	body, _ := json.Marshal(regPayload)
	http.Post(srv.URL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))

	// Submit a task
	taskReq := map[string]any{
		"type": "greeting", "input": map[string]string{"name": "Test"},
		"routing": map[string]any{"strategy": "best_match", "required_capabilities": []string{"greeting"}},
		"contract": map[string]any{"timeout_ms": 30000},
	}
	body, _ = json.Marshal(taskReq)
	http.Post(srv.URL+"/api/v1/tasks", "application/json", bytes.NewReader(body))

	// List tasks
	resp, _ := http.Get(srv.URL + "/api/v1/tasks")
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}

	var tasks []*protocol.Task
	json.NewDecoder(resp.Body).Decode(&tasks)
	if len(tasks) != 1 {
		t.Errorf("tasks count: got %d, want 1", len(tasks))
	}
}

func TestGateway_SubmitWorkflow(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	regPayload := protocol.RegisterPayload{
		Name:         "MultiBot",
		Capabilities: []protocol.Capability{{Name: "market_research"}, {Name: "content_writing"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 10},
	}
	body, _ := json.Marshal(regPayload)
	http.Post(srv.URL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))

	wfReq := map[string]any{
		"name": "Test Workflow",
		"steps": []map[string]any{
			{"id": "step1", "task_type": "market_research", "input": map[string]string{"topic": "AI"}},
			{"id": "step2", "task_type": "content_writing", "depends_on": []string{"step1"}, "input": map[string]string{}},
		},
	}
	body, _ = json.Marshal(wfReq)

	resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}

	var wf protocol.Workflow
	json.NewDecoder(resp.Body).Decode(&wf)
	if wf.Status != protocol.WorkflowRunning {
		t.Errorf("status: got %q, want running", wf.Status)
	}
}

func TestGateway_CreateTeam(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"name":         "Marketing",
		"org_id":       "org_magic",
		"daily_budget": 10.0,
	})
	resp, err := http.Post(srv.URL+"/api/v1/teams", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}

	var team protocol.Team
	json.NewDecoder(resp.Body).Decode(&team)
	if team.Name != "Marketing" {
		t.Errorf("Name: got %q", team.Name)
	}
}

func TestGateway_ListTeams(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"name": "Sales", "org_id": "org", "daily_budget": 5.0})
	http.Post(srv.URL+"/api/v1/teams", "application/json", bytes.NewReader(body))

	resp, _ := http.Get(srv.URL + "/api/v1/teams")
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d", resp.StatusCode)
	}

	var teams []*protocol.Team
	json.NewDecoder(resp.Body).Decode(&teams)
	if len(teams) != 1 {
		t.Errorf("teams: got %d, want 1", len(teams))
	}
}

func TestGateway_CostReport(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/api/v1/costs")
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d", resp.StatusCode)
	}

	var report map[string]any
	json.NewDecoder(resp.Body).Decode(&report)
	if _, ok := report["total_cost"]; !ok {
		t.Error("should have total_cost field")
	}
}

func TestGateway_AddKnowledge(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"title":   "API Guide",
		"content": "Use REST",
		"tags":    []string{"api"},
		"scope":   "org",
		"scope_id": "org_magic",
	})
	resp, err := http.Post(srv.URL+"/api/v1/knowledge", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}

	var entry protocol.KnowledgeEntry
	json.NewDecoder(resp.Body).Decode(&entry)
	if entry.Title != "API Guide" {
		t.Errorf("Title: got %q", entry.Title)
	}
}

func TestGateway_SearchKnowledge(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Add an entry first
	body, _ := json.Marshal(map[string]any{
		"title": "REST Guide", "content": "Use REST", "tags": []string{"api"},
		"scope": "org", "scope_id": "org_magic",
	})
	http.Post(srv.URL+"/api/v1/knowledge", "application/json", bytes.NewReader(body))

	// Search
	resp, _ := http.Get(srv.URL + "/api/v1/knowledge?q=REST")
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d", resp.StatusCode)
	}

	var entries []*protocol.KnowledgeEntry
	json.NewDecoder(resp.Body).Decode(&entries)
	if len(entries) != 1 {
		t.Errorf("search results: got %d, want 1", len(entries))
	}
}

// --- Token management tests ---

func TestCreateToken_Success(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"name": "test-token"})
	resp, err := http.Post(srv.URL+"/api/v1/orgs/org1/tokens", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	rawToken, ok := result["token"].(string)
	if !ok || rawToken == "" {
		t.Error("response should contain a non-empty 'token' field")
	}
	if len(rawToken) < 4 || rawToken[:4] != "mct_" {
		t.Errorf("token should start with 'mct_', got %q", rawToken)
	}
	if result["id"] == "" {
		t.Error("response should contain a non-empty 'id' field")
	}
}

func TestCreateToken_ListTokens(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Create a token
	body, _ := json.Marshal(map[string]any{"name": "my-token"})
	createResp, _ := http.Post(srv.URL+"/api/v1/orgs/org1/tokens", "application/json", bytes.NewReader(body))
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)

	// List tokens
	resp, _ := http.Get(srv.URL + "/api/v1/orgs/org1/tokens")
	if resp.StatusCode != 200 {
		t.Errorf("list status: got %d, want 200", resp.StatusCode)
	}

	var tokens []map[string]any
	json.NewDecoder(resp.Body).Decode(&tokens)
	if len(tokens) != 1 {
		t.Fatalf("token count: got %d, want 1", len(tokens))
	}

	// Verify raw token and hash are not exposed
	tok := tokens[0]
	if _, hasRaw := tok["token"]; hasRaw {
		t.Error("list response should NOT contain raw token")
	}
	if _, hasHash := tok["token_hash"]; hasHash {
		t.Error("list response should NOT contain token_hash")
	}
	if tok["name"] != "my-token" {
		t.Errorf("name: got %q, want 'my-token'", tok["name"])
	}
}

func TestRevokeToken_Success(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Create a token
	body, _ := json.Marshal(map[string]any{"name": "revoke-me"})
	createResp, _ := http.Post(srv.URL+"/api/v1/orgs/org1/tokens", "application/json", bytes.NewReader(body))
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	tokenID := created["id"].(string)

	// Revoke it
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/orgs/org1/tokens/"+tokenID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("revoke status: got %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "revoked" {
		t.Errorf("status: got %q, want 'revoked'", result["status"])
	}
	if result["token_id"] != tokenID {
		t.Errorf("token_id: got %q, want %q", result["token_id"], tokenID)
	}
}

func TestRevokeToken_WrongOrg(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Create token for org1
	body, _ := json.Marshal(map[string]any{"name": "org1-token"})
	createResp, _ := http.Post(srv.URL+"/api/v1/orgs/org1/tokens", "application/json", bytes.NewReader(body))
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	tokenID := created["id"].(string)

	// Try to revoke from org2
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/orgs/org2/tokens/"+tokenID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 && resp.StatusCode != 404 {
		t.Errorf("wrong-org revoke status: got %d, want 403 or 404", resp.StatusCode)
	}
}

func TestQueryAudit_Empty(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/orgs/org1/audit")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	entries, ok := result["entries"]
	if !ok {
		t.Fatal("response should have 'entries' field")
	}
	list, ok := entries.([]any)
	if !ok {
		t.Fatalf("'entries' should be a list, got %T", entries)
	}
	if len(list) != 0 {
		t.Errorf("entries count: got %d, want 0", len(list))
	}
}

// --- Worker auth middleware tests ---

func TestWorkerAuth_DevMode(t *testing.T) {
	// Dev mode: no tokens stored, register without auth header should pass
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	payload := protocol.RegisterPayload{
		Name:         "DevBot",
		Capabilities: []protocol.Capability{{Name: "test"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 1},
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(srv.URL+"/api/v1/workers/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	// Dev mode: no tokens exist, should succeed
	if resp.StatusCode != 201 {
		t.Errorf("dev mode register: got %d, want 201", resp.StatusCode)
	}
}

func TestWorkerAuth_InvalidToken(t *testing.T) {
	gw := setupGateway()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	// Add a token so we leave dev mode
	body, _ := json.Marshal(map[string]any{"name": "gate-token"})
	http.Post(srv.URL+"/api/v1/orgs/org1/tokens", "application/json", bytes.NewReader(body))

	// Try to register with an invalid token
	payload := protocol.RegisterPayload{
		Name:     "EvilBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9002"},
	}
	reqBody, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/workers/register", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer mct_invalid_token_value")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("invalid token status: got %d, want 401", resp.StatusCode)
	}
}
