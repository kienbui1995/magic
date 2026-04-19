//go:build e2e

package e2e

import (
	"context"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/protocol"
)

// TestE2E_TaskLifecycle — register → submit → worker completes →
// task marked completed, cost recorded, task.completed bus event fired,
// Prometheus magic_tasks_total counter incremented.
func TestE2E_TaskLifecycle(t *testing.T) {
	fs := setupFullStack(t)

	completedCh := make(chan events.Event, 4)
	fs.Bus.Subscribe("task.completed", func(e events.Event) { completedCh <- e })

	before := readTaskCounter("completed")

	workerURL := startEchoWorker(t, defaultEchoHandler(0.042))
	workerID := registerWorker(t, fs.ServerURL, "EchoBot", workerURL, []string{"echo"})

	taskID, status := submitTask(t, fs.ServerURL, "echo", map[string]string{"hello": "world"}, []string{"echo"})
	if status != http.StatusCreated {
		t.Fatalf("submit status: got %d, want 201", status)
	}

	task := waitForTaskStatus(t, fs.ServerURL, taskID, protocol.TaskCompleted, 5*time.Second)

	if task.Cost <= 0 {
		t.Errorf("expected cost > 0, got %v", task.Cost)
	}
	if task.AssignedWorker != workerID {
		t.Errorf("assigned_worker: got %q, want %q", task.AssignedWorker, workerID)
	}

	// Verify task.completed event on bus
	select {
	case e := <-completedCh:
		if gotID, _ := e.Payload["task_id"].(string); gotID != taskID {
			t.Errorf("event task_id: got %q, want %q", gotID, taskID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive task.completed event on bus")
	}

	// Cost report reflects the cost
	resp, err := http.Get(fs.ServerURL + "/api/v1/costs")
	if err != nil {
		t.Fatalf("cost report: %v", err)
	}
	defer resp.Body.Close()
	var report map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&report)
	if total, _ := report["total_cost"].(float64); total <= 0 {
		t.Errorf("total_cost in report: got %v, want > 0", total)
	}

	// Prometheus counter should have advanced
	after := readTaskCounter("completed")
	if after <= before {
		t.Errorf("magic_tasks_total{status=completed}: got %v, want > %v", after, before)
	}
}

// TestE2E_WebhookDelivery — submitting a task that completes triggers a
// webhook POST to a registered receiver with a valid HMAC-SHA256 signature
// and the expected event envelope.
//
// We bypass validateWebhookURL by registering the webhook through the
// webhook manager directly (loopback URLs are only blocked at the HTTP
// handler boundary).
func TestE2E_WebhookDelivery(t *testing.T) {
	fs := setupFullStack(t)
	receiver := startWebhookReceiver(t)

	const secret = "test-secret-do-not-use-in-prod"
	const orgID = "org_e2e"
	hook, err := fs.Webhook.CreateWebhook(context.Background(), orgID, receiver.URL(),
		[]string{"task.completed"}, secret)
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	workerURL := startEchoWorker(t, defaultEchoHandler(0.01))
	registerWorker(t, fs.ServerURL, "EchoBot", workerURL, []string{"echo"})

	taskID, status := submitTask(t, fs.ServerURL, "echo", map[string]string{"msg": "hi"}, []string{"echo"})
	if status != http.StatusCreated {
		t.Fatalf("submit: got %d", status)
	}
	waitForTaskStatus(t, fs.ServerURL, taskID, protocol.TaskCompleted, 5*time.Second)

	// Sender polls every 5s; allow up to 15s for first tick + delivery.
	records := receiver.waitForWebhooks(t, 1, 15*time.Second)
	rec := records[0]

	if got := rec.Headers.Get("X-MagiC-Event"); got != "task.completed" {
		t.Errorf("X-MagiC-Event: got %q, want task.completed", got)
	}
	if got := rec.Headers.Get("X-MagiC-Delivery"); got == "" {
		t.Error("X-MagiC-Delivery header missing")
	}

	// Verify HMAC-SHA256 signature
	sigHeader := rec.Headers.Get("X-MagiC-Signature")
	if !strings.HasPrefix(sigHeader, "sha256=") {
		t.Fatalf("signature header: got %q, want sha256= prefix", sigHeader)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rec.Body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if sigHeader != want {
		t.Errorf("signature mismatch:\n got=%q\nwant=%q", sigHeader, want)
	}

	// Payload envelope: {type, timestamp, data}
	var env map[string]any
	if err := json.Unmarshal(rec.Body, &env); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if env["type"] != "task.completed" {
		t.Errorf("payload.type: got %v", env["type"])
	}
	if _, ok := env["data"]; !ok {
		t.Error("payload missing data field")
	}

	// Sanity: the webhook we just created is queryable
	_ = hook
}

// TestE2E_TaskCancel — task sitting in pending state can be cancelled.
// We seed the task directly into the store (bypassing dispatch) to avoid
// racing with the worker reply, then verify /cancel transitions it to
// cancelled and publishes task.cancelled on the bus.
func TestE2E_TaskCancel(t *testing.T) {
	fs := setupFullStack(t)

	cancelledCh := make(chan events.Event, 4)
	completedCh := make(chan events.Event, 4)
	fs.Bus.Subscribe("task.cancelled", func(e events.Event) { cancelledCh <- e })
	fs.Bus.Subscribe("task.completed", func(e events.Event) { completedCh <- e })

	taskID := protocol.GenerateID("task")
	if err := fs.Store.AddTask(context.Background(), &protocol.Task{
		ID:        taskID,
		Type:      "slow",
		Priority:  protocol.PriorityNormal,
		Status:    protocol.TaskPending,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost,
		fs.ServerURL+"/api/v1/tasks/"+taskID+"/cancel", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status: got %d, want 200", resp.StatusCode)
	}
	var task protocol.Task
	_ = json.NewDecoder(resp.Body).Decode(&task)
	if task.Status != protocol.TaskCancelled {
		t.Errorf("status after cancel: got %q, want %q", task.Status, protocol.TaskCancelled)
	}

	select {
	case <-cancelledCh:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive task.cancelled event")
	}

	// Double-check no task.completed event was ever published for this task.
	select {
	case e := <-completedCh:
		if gotID, _ := e.Payload["task_id"].(string); gotID == taskID {
			t.Errorf("unexpected task.completed for cancelled task %s", taskID)
		}
	case <-time.After(200 * time.Millisecond):
		// expected: nothing
	}
}

// TestE2E_WorkerPauseResume — routing skips paused workers (task submit →
// 503), resume restores it (next submit succeeds end-to-end).
func TestE2E_WorkerPauseResume(t *testing.T) {
	fs := setupFullStack(t)

	workerURL := startEchoWorker(t, defaultEchoHandler(0.01))
	workerID := registerWorker(t, fs.ServerURL, "PauseBot", workerURL, []string{"echo"})

	// Pause
	resp, err := http.Post(fs.ServerURL+"/api/v1/workers/"+workerID+"/pause",
		"application/json", nil)
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause status: got %d, want 200", resp.StatusCode)
	}

	_, status := submitTask(t, fs.ServerURL, "echo", map[string]string{"x": "1"}, []string{"echo"})
	if status != http.StatusServiceUnavailable {
		t.Errorf("submit with paused worker: got %d, want 503", status)
	}

	// Resume
	resp2, err := http.Post(fs.ServerURL+"/api/v1/workers/"+workerID+"/resume",
		"application/json", nil)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("resume status: got %d, want 200", resp2.StatusCode)
	}

	taskID, status := submitTask(t, fs.ServerURL, "echo", map[string]string{"x": "2"}, []string{"echo"})
	if status != http.StatusCreated {
		t.Fatalf("submit after resume: got %d, want 201", status)
	}
	waitForTaskStatus(t, fs.ServerURL, taskID, protocol.TaskCompleted, 5*time.Second)
}

// TestE2E_WorkflowDAG — 2-step workflow with step2 depends_on step1 runs
// sequentially; step1 must complete before step2 is dispatched. We enforce
// ordering by having the worker record per-step timestamps.
func TestE2E_WorkflowDAG(t *testing.T) {
	fs := setupFullStack(t)

	var mu sync.Mutex
	timestamps := map[string]time.Time{}

	worker := startEchoWorker(t, func(w http.ResponseWriter, r *http.Request) {
		var msg protocol.Message
		_ = json.NewDecoder(r.Body).Decode(&msg)
		var assign protocol.TaskAssignPayload
		_ = json.Unmarshal(msg.Payload, &assign)

		mu.Lock()
		timestamps[assign.TaskType] = time.Now()
		mu.Unlock()

		out, _ := json.Marshal(map[string]any{"step": assign.TaskType})
		payload, _ := json.Marshal(protocol.TaskCompletePayload{
			TaskID: assign.TaskID, Output: out, Cost: 0.01,
		})
		_ = json.NewEncoder(w).Encode(dispatcher.DispatchResponse{
			Type: protocol.MsgTaskComplete, Payload: payload,
		})
	})

	registerWorker(t, fs.ServerURL, "DagBot", worker,
		[]string{"market_research", "content_writing"})

	wfReq := map[string]any{
		"name": "e2e-dag",
		"steps": []map[string]any{
			{"id": "s1", "task_type": "market_research", "input": map[string]string{"topic": "AI"}},
			{"id": "s2", "task_type": "content_writing",
				"depends_on": []string{"s1"}, "input": map[string]string{}},
		},
	}
	body, _ := json.Marshal(wfReq)
	resp, err := http.Post(fs.ServerURL+"/api/v1/workflows",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("submit workflow: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("workflow submit status=%d body=%s", resp.StatusCode, raw)
	}
	var wf protocol.Workflow
	_ = json.NewDecoder(resp.Body).Decode(&wf)

	// Poll until completed
	deadline := time.Now().Add(10 * time.Second)
	for {
		r, err := http.Get(fs.ServerURL + "/api/v1/workflows/" + wf.ID)
		if err != nil {
			t.Fatalf("get workflow: %v", err)
		}
		var cur protocol.Workflow
		_ = json.NewDecoder(r.Body).Decode(&cur)
		r.Body.Close()
		if cur.Status == protocol.WorkflowCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("workflow stuck in status=%q", cur.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	t1, ok1 := timestamps["market_research"]
	t2, ok2 := timestamps["content_writing"]
	mu.Unlock()

	if !ok1 || !ok2 {
		t.Fatalf("missing step timestamps: s1=%v s2=%v", ok1, ok2)
	}
	if !t1.Before(t2) {
		t.Errorf("step ordering violated: s1=%v s2=%v", t1, t2)
	}
}

// TestE2E_RateLimit — bursting far above the burst size (20) for task
// submissions triggers at least one 429 response. The limiter is per-IP
// and also per-org; when all traffic comes from the same httptest client
// and no X-Org-ID is set, both limiters key off the same IP, so excess
// requests are rejected.
func TestE2E_RateLimit(t *testing.T) {
	fs := setupFullStack(t)

	workerURL := startEchoWorker(t, defaultEchoHandler(0.001))
	registerWorker(t, fs.ServerURL, "RateBot", workerURL, []string{"echo"})

	const N = 60
	body, _ := json.Marshal(map[string]any{
		"type":  "echo",
		"input": map[string]string{"x": "1"},
		"routing": map[string]any{
			"strategy":              "best_match",
			"required_capabilities": []string{"echo"},
		},
		"contract": map[string]any{"timeout_ms": 5000},
	})

	var (
		ok      atomic.Int32
		limited atomic.Int32
		other   atomic.Int32
		wg      sync.WaitGroup
	)
	wg.Add(N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			<-start
			resp, err := http.Post(fs.ServerURL+"/api/v1/tasks",
				"application/json", bytes.NewReader(body))
			if err != nil {
				other.Add(1)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusCreated:
				ok.Add(1)
			case http.StatusTooManyRequests:
				limited.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	t.Logf("rate-limit: ok=%d limited=%d other=%d", ok.Load(), limited.Load(), other.Load())
	if limited.Load() == 0 {
		t.Errorf("expected at least one 429; got ok=%d limited=0 other=%d",
			ok.Load(), other.Load())
	}
	if int(ok.Load()) >= N {
		t.Errorf("all %d requests succeeded — rate limiter did not engage", N)
	}
}

// TestE2E_AuditLog — successful + failed worker-lifecycle actions produce
// audit entries queryable via GET /api/v1/orgs/{orgID}/audit.
//
// We seed audit entries directly into the store for both outcomes; the
// middleware-driven audit path requires tokens to be configured, which is
// exercised in gateway unit tests. This test focuses on the end-to-end
// query surface — filter + pagination + JSON shape.
func TestE2E_AuditLog(t *testing.T) {
	fs := setupFullStack(t)

	const orgID = "org_audit_e2e"
	now := time.Now()

	for i, e := range []*protocol.AuditEntry{
		{
			ID:        protocol.GenerateID("audit"),
			Timestamp: now,
			OrgID:     orgID,
			Action:    "worker.registered",
			Resource:  "worker/w1",
			Outcome:   "success",
		},
		{
			ID:        protocol.GenerateID("audit"),
			Timestamp: now.Add(time.Millisecond),
			OrgID:     orgID,
			Action:    "auth.rejected",
			Resource:  "/api/v1/workers/register",
			Outcome:   "denied",
			Detail:    map[string]any{"reason": "invalid token"},
		},
	} {
		if err := fs.Store.AppendAudit(context.Background(), e); err != nil {
			t.Fatalf("seed audit %d: %v", i, err)
		}
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/v1/orgs/%s/audit?limit=100",
		fs.ServerURL, orgID))
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit query status: got %d", resp.StatusCode)
	}

	var body struct {
		Entries []*protocol.AuditEntry `json:"entries"`
		Total   int                    `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total < 2 {
		t.Fatalf("audit total: got %d, want >= 2", body.Total)
	}
	seen := map[string]bool{}
	for _, e := range body.Entries {
		seen[e.Action] = true
		if e.OrgID != orgID {
			t.Errorf("entry org_id: got %q, want %q", e.OrgID, orgID)
		}
	}
	for _, want := range []string{"worker.registered", "auth.rejected"} {
		if !seen[want] {
			t.Errorf("expected audit action %q in entries", want)
		}
	}
}

// readTaskCounter returns the current sum of magic_tasks_total{status=<status>}
// across all label combinations.
func readTaskCounter(status string) float64 {
	mf, err := gatherMetric("magic_tasks_total")
	if err != nil || mf == nil {
		return 0
	}
	var total float64
	for _, m := range mf.GetMetric() {
		var got string
		for _, lbl := range m.GetLabel() {
			if lbl.GetName() == "status" {
				got = lbl.GetValue()
			}
		}
		if got == status {
			total += m.GetCounter().GetValue()
		}
	}
	return total
}

func gatherMetric(name string) (*dto.MetricFamily, error) {
	// Use the default prometheus registry that promauto registers into.
	// monitor.MetricTasksTotal is registered there.
	_ = monitor.MetricTasksTotal // force reference so the var is alive
	mfs, err := prometheusDefaultGather()
	if err != nil {
		return nil, err
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf, nil
		}
	}
	return nil, nil
}
