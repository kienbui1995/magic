//go:build e2e

// Package e2e provides end-to-end tests exercising the full MagiC stack
// (gateway + registry + router + dispatcher + store + events + webhook
// manager) with in-process components. Build tag `e2e` gates this package
// so unit test runs (plain `go test ./...`) remain unaffected.
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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
	"github.com/kienbui1995/magic/core/internal/webhook"
)

// fullStack holds every long-lived component wired together, mirroring the
// real `magic serve` startup path closely enough to catch regressions across
// module boundaries.
type fullStack struct {
	ServerURL string
	Store     store.Store
	Bus       *events.Bus
	Webhook   *webhook.Manager
	cleanup   func()
}

// setupFullStack builds an in-memory MagiC instance behind an httptest server.
// No external dependencies (no Postgres, no Redis).
func setupFullStack(t *testing.T) *fullStack {
	t.Helper()

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
	wh := webhook.New(s, bus, webhook.AllowAllURLs()) // allow loopback httptest servers in E2E
	wh.Start() // starts event subscribers + 5s retry sender

	var dispatchWG sync.WaitGroup

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
		Webhook:      wh,
		DispatchWG:   &dispatchWG,
	})

	srv := httptest.NewServer(gw.Handler())

	fs := &fullStack{
		ServerURL: srv.URL,
		Store:     s,
		Bus:       bus,
		Webhook:   wh,
	}
	fs.cleanup = func() {
		srv.Close()
		dispatchWG.Wait()
		wh.Stop()
		bus.Stop()
	}
	t.Cleanup(fs.cleanup)
	return fs
}

// startEchoWorker spins up an httptest worker that handles MagiC task.assign
// messages with the supplied handler. The handler must write a valid
// dispatcher.DispatchResponse JSON (type + payload) to w.
func startEchoWorker(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

// defaultEchoHandler replies with a task.complete for every task.assign,
// echoing input back as output with a fixed cost.
func defaultEchoHandler(cost float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var msg protocol.Message
		_ = json.NewDecoder(r.Body).Decode(&msg)
		var assign protocol.TaskAssignPayload
		_ = json.Unmarshal(msg.Payload, &assign)

		out, _ := json.Marshal(map[string]any{
			"echo":    json.RawMessage(assign.Input),
			"task_id": assign.TaskID,
		})
		payload, _ := json.Marshal(protocol.TaskCompletePayload{
			TaskID: assign.TaskID,
			Output: out,
			Cost:   cost,
		})
		resp := dispatcher.DispatchResponse{
			Type:    protocol.MsgTaskComplete,
			Payload: payload,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// registerWorker registers a worker via the gateway HTTP API and returns its ID.
func registerWorker(t *testing.T, serverURL, name, workerURL string, caps []string) string {
	t.Helper()
	capsSlice := make([]protocol.Capability, 0, len(caps))
	for _, c := range caps {
		capsSlice = append(capsSlice, protocol.Capability{Name: c})
	}
	body, _ := json.Marshal(protocol.RegisterPayload{
		Name:         name,
		Capabilities: capsSlice,
		Endpoint:     protocol.Endpoint{Type: "http", URL: workerURL},
		Limits:       protocol.WorkerLimits{MaxConcurrentTasks: 10},
	})
	resp, err := http.Post(serverURL+"/api/v1/workers/register",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("register worker status=%d body=%s", resp.StatusCode, raw)
	}
	var out protocol.Worker
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.ID
}

// submitTask submits a task via the gateway and returns (taskID, statusCode).
// Non-2xx returns ("", statusCode) and does not fatal.
func submitTask(t *testing.T, serverURL, taskType string, input any, caps []string) (string, int) {
	t.Helper()
	inputBytes, _ := json.Marshal(input)
	req := map[string]any{
		"type":  taskType,
		"input": json.RawMessage(inputBytes),
		"routing": map[string]any{
			"strategy":              "best_match",
			"required_capabilities": caps,
		},
		"contract": map[string]any{"timeout_ms": 10000, "max_cost": 10.0},
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(serverURL+"/api/v1/tasks",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", resp.StatusCode
	}
	var task protocol.Task
	_ = json.NewDecoder(resp.Body).Decode(&task)
	return task.ID, resp.StatusCode
}

// waitForTaskStatus polls GET /api/v1/tasks/{id} until task.Status == target
// or until timeout elapses. Returns the final task.
func waitForTaskStatus(t *testing.T, serverURL, taskID, target string, timeout time.Duration) *protocol.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		resp, err := http.Get(serverURL + "/api/v1/tasks/" + taskID)
		if err == nil {
			var task protocol.Task
			_ = json.NewDecoder(resp.Body).Decode(&task)
			resp.Body.Close()
			if task.Status == target {
				return &task
			}
			if time.Now().After(deadline) {
				t.Fatalf("task %s: waited %s for status=%q, last status=%q",
					taskID, timeout, target, task.Status)
			}
		} else if time.Now().After(deadline) {
			t.Fatalf("task %s: poll error: %v", taskID, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// webhookRecord captures an inbound webhook POST.
type webhookRecord struct {
	Headers http.Header
	Body    []byte
}

// webhookReceiver accumulates webhook POSTs for inspection.
type webhookReceiver struct {
	mu      sync.Mutex
	records []webhookRecord
	srv     *httptest.Server
}

// startWebhookReceiver runs an httptest server that records every POST.
func startWebhookReceiver(t *testing.T) *webhookReceiver {
	t.Helper()
	r := &webhookReceiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.records = append(r.records, webhookRecord{Headers: req.Header.Clone(), Body: body})
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *webhookReceiver) URL() string { return r.srv.URL }

func (r *webhookReceiver) Records() []webhookRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]webhookRecord, len(r.records))
	copy(out, r.records)
	return out
}

// waitForWebhooks polls until at least `n` records are seen or timeout.
func (r *webhookReceiver) waitForWebhooks(t *testing.T, n int, timeout time.Duration) []webhookRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		records := r.Records()
		if len(records) >= n {
			return records
		}
		if time.Now().After(deadline) {
			t.Fatalf("webhook: waited %s for %d records, got %d", timeout, n, len(records))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
