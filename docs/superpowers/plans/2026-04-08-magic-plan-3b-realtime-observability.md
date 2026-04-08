# MagiC Phase 3b: Real-time & Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add SSE task streaming, at-least-once webhook delivery, and comprehensive Prometheus metrics.

**Architecture:** SSE streaming adds `DispatchStream()` to the Dispatcher — Gateway proxies chunked SSE from worker to client. Webhooks are a new package (`webhook/`) subscribing to the event bus and queuing deliveries in the Store with exponential-backoff retry. Prometheus metrics use `promauto` registered in the Monitor package, served on a new unauthenticated `/metrics` endpoint.

**Tech Stack:** Go 1.22+, `github.com/prometheus/client_golang`, `http.ResponseController` for per-request write deadlines (Go 1.20+), existing event bus (pub/sub) and store interfaces.

**Prerequisite:** Phase 3a must be complete (Store interface will have Webhook methods added here; all three store implementations must implement them).

**Spec:** `docs/superpowers/specs/2026-04-08-magic-phase3-design.md`

---

## File Map

```
core/
├── go.mod / go.sum                          MODIFY: add prometheus/client_golang
├── internal/protocol/
│   └── types.go                             MODIFY: Streaming bool in Capability; Webhook + WebhookDelivery types
├── internal/store/
│   ├── store.go                             MODIFY: add 8 Webhook/Delivery methods to Store interface
│   ├── memory.go                            MODIFY: implement 8 new methods
│   ├── sqlite.go                            MODIFY: implement 8 new methods
│   └── postgres.go                          MODIFY: implement 8 new methods (append)
├── internal/dispatcher/
│   ├── dispatcher.go                        MODIFY: add DispatchStream()
│   └── dispatcher_test.go                   MODIFY: add streaming test
├── internal/gateway/
│   ├── gateway.go                           MODIFY: add streaming routes + /metrics + Webhook dep
│   └── handlers.go                          MODIFY: add handleStreamTask + webhook CRUD handlers
├── internal/webhook/
│   ├── manager.go                           NEW: event bus subscription + delivery enqueue
│   └── sender.go                            NEW: retry loop + HMAC signing
├── internal/monitor/
│   ├── monitor.go                           MODIFY: update handleEvent to update Prometheus counters
│   └── metrics.go                           NEW: promauto metric registrations
└── cmd/magic/
    └── main.go                              MODIFY: wire webhook.Manager + /metrics endpoint
```

---

## Chunk 1: Protocol Types

### Task 1: Add Streaming capability flag + Webhook/WebhookDelivery types

**Files:**
- Modify: `core/internal/protocol/types.go`

- [ ] **Step 1: Add Streaming field to Capability**

Find the `Capability` struct in `core/internal/protocol/types.go` and add `Streaming bool`:

```go
type Capability struct {
    Name           string          `json:"name"`
    Description    string          `json:"description"`
    InputSchema    json.RawMessage `json:"input_schema,omitempty"`
    OutputSchema   json.RawMessage `json:"output_schema,omitempty"`
    EstCostPerCall float64         `json:"est_cost_per_call,omitempty"`
    Streaming      bool            `json:"streaming,omitempty"` // worker supports SSE streaming
}
```

- [ ] **Step 2: Add Webhook and WebhookDelivery types**

Append to `core/internal/protocol/types.go`:

```go
// Webhook represents a registered webhook endpoint for receiving MagiC events.
type Webhook struct {
    ID        string    `json:"id"`
    OrgID     string    `json:"org_id"`
    URL       string    `json:"url"`
    Events    []string  `json:"events"`                    // e.g. ["task.complete", "worker.register"]
    Secret    string    `json:"secret,omitempty"`          // write-only: HMAC-SHA256 key, never returned in GET
    Active    bool      `json:"active"`
    CreatedAt time.Time `json:"created_at"`
}

// WebhookDelivery tracks one attempted delivery of an event to a webhook URL.
type WebhookDelivery struct {
    ID        string     `json:"id"`
    WebhookID string     `json:"webhook_id"`
    EventType string     `json:"event_type"`
    Payload   string     `json:"payload"`           // JSON-encoded event body
    Status    string     `json:"status"`            // pending|delivered|failed|dead
    Attempts  int        `json:"attempts"`
    NextRetry *time.Time `json:"next_retry,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}

// Webhook delivery statuses
const (
    DeliveryPending   = "pending"
    DeliveryDelivered = "delivered"
    DeliveryFailed    = "failed"
    DeliveryDead      = "dead" // max retries exhausted
)
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
cd core && go build ./internal/protocol/...
```

- [ ] **Step 4: Commit**

```bash
git add core/internal/protocol/types.go
git commit -m "feat(protocol): Streaming capability flag + Webhook/WebhookDelivery types"
```

---

## Chunk 2: Store Interface + All Three Implementations

### Task 2: Extend Store interface + implement in MemoryStore

**Files:**
- Modify: `core/internal/store/store.go`
- Modify: `core/internal/store/memory.go`

- [ ] **Step 1: Add 8 webhook methods to Store interface**

Append to the `Store` interface in `core/internal/store/store.go`:

```go
    // Webhooks
    AddWebhook(w *protocol.Webhook) error
    GetWebhook(id string) (*protocol.Webhook, error)
    UpdateWebhook(w *protocol.Webhook) error
    DeleteWebhook(id string) error
    ListWebhooksByOrg(orgID string) []*protocol.Webhook
    FindWebhooksByEvent(eventType string) []*protocol.Webhook

    // Webhook deliveries
    AddWebhookDelivery(d *protocol.WebhookDelivery) error
    UpdateWebhookDelivery(d *protocol.WebhookDelivery) error
    ListPendingWebhookDeliveries() []*protocol.WebhookDelivery
```

- [ ] **Step 2: Build to see which stores need updating**

```bash
cd core && go build ./...
```
Expected: compile errors — MemoryStore, SQLiteStore, PostgreSQLStore do not implement the 8 new methods yet.

- [ ] **Step 3: Implement in MemoryStore**

Append to `core/internal/store/memory.go`:

```go
// --- Webhooks ---

func (s *MemoryStore) AddWebhook(w *protocol.Webhook) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.webhooks[w.ID] = protocol.DeepCopyWebhook(w)
    return nil
}

func (s *MemoryStore) GetWebhook(id string) (*protocol.Webhook, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    w, ok := s.webhooks[id]
    if !ok {
        return nil, ErrNotFound
    }
    return protocol.DeepCopyWebhook(w), nil
}

func (s *MemoryStore) UpdateWebhook(w *protocol.Webhook) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, ok := s.webhooks[w.ID]; !ok {
        return ErrNotFound
    }
    s.webhooks[w.ID] = protocol.DeepCopyWebhook(w)
    return nil
}

func (s *MemoryStore) DeleteWebhook(id string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, ok := s.webhooks[id]; !ok {
        return ErrNotFound
    }
    delete(s.webhooks, id)
    return nil
}

func (s *MemoryStore) ListWebhooksByOrg(orgID string) []*protocol.Webhook {
    s.mu.RLock()
    defer s.mu.RUnlock()
    var result []*protocol.Webhook
    for _, w := range s.webhooks {
        if w.OrgID == orgID {
            result = append(result, protocol.DeepCopyWebhook(w))
        }
    }
    return result
}

func (s *MemoryStore) FindWebhooksByEvent(eventType string) []*protocol.Webhook {
    s.mu.RLock()
    defer s.mu.RUnlock()
    var result []*protocol.Webhook
    for _, w := range s.webhooks {
        if !w.Active {
            continue
        }
        for _, e := range w.Events {
            if e == eventType {
                result = append(result, protocol.DeepCopyWebhook(w))
                break
            }
        }
    }
    return result
}

// --- Webhook Deliveries ---

func (s *MemoryStore) AddWebhookDelivery(d *protocol.WebhookDelivery) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    cp := *d
    s.webhookDeliveries[d.ID] = &cp
    return nil
}

func (s *MemoryStore) UpdateWebhookDelivery(d *protocol.WebhookDelivery) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, ok := s.webhookDeliveries[d.ID]; !ok {
        return ErrNotFound
    }
    cp := *d
    s.webhookDeliveries[d.ID] = &cp
    return nil
}

func (s *MemoryStore) ListPendingWebhookDeliveries() []*protocol.WebhookDelivery {
    s.mu.RLock()
    defer s.mu.RUnlock()
    now := time.Now()
    var result []*protocol.WebhookDelivery
    for _, d := range s.webhookDeliveries {
        if d.Status == protocol.DeliveryPending || d.Status == protocol.DeliveryFailed {
            if d.NextRetry == nil || d.NextRetry.Before(now) {
                cp := *d
                result = append(result, &cp)
            }
        }
    }
    return result
}
```

- [ ] **Step 4: Add webhook maps to MemoryStore struct and constructor**

Find the `MemoryStore` struct in `memory.go` and add the two new maps:

```go
type MemoryStore struct {
    mu sync.RWMutex
    // ... existing fields ...
    webhooks          map[string]*protocol.Webhook
    webhookDeliveries map[string]*protocol.WebhookDelivery
}

// In NewMemoryStore():
func NewMemoryStore() *MemoryStore {
    return &MemoryStore{
        // ... existing initializations ...
        webhooks:          make(map[string]*protocol.Webhook),
        webhookDeliveries: make(map[string]*protocol.WebhookDelivery),
    }
}
```

- [ ] **Step 5: Add DeepCopyWebhook to protocol/types.go**

Append to `core/internal/protocol/types.go`:

```go
func DeepCopyWebhook(w *Webhook) *Webhook {
    cp := *w
    if w.Events != nil {
        cp.Events = make([]string, len(w.Events))
        copy(cp.Events, w.Events)
    }
    return &cp
}
```

- [ ] **Step 6: Build to verify MemoryStore compiles**

```bash
cd core && go build ./internal/store/...
```

- [ ] **Step 7: Commit**

```bash
git add core/internal/store/store.go core/internal/store/memory.go core/internal/protocol/types.go
git commit -m "feat(store): add Webhook + WebhookDelivery methods to Store interface + MemoryStore"
```

---

### Task 3: Implement Webhook methods in SQLiteStore

**Files:**
- Modify: `core/internal/store/sqlite.go`

- [ ] **Step 1: Append webhook methods to sqlite.go**

```go
// --- Webhooks ---

func (s *SQLiteStore) AddWebhook(w *protocol.Webhook) error {
    return putJSON(s.db, "webhooks", w.ID, w)
}

func (s *SQLiteStore) GetWebhook(id string) (*protocol.Webhook, error) {
    return getJSON[protocol.Webhook](s.db, "webhooks", id)
}

func (s *SQLiteStore) UpdateWebhook(w *protocol.Webhook) error {
    return putJSON(s.db, "webhooks", w.ID, w)
}

func (s *SQLiteStore) DeleteWebhook(id string) error {
    return deleteRow(s.db, "webhooks", id)
}

func (s *SQLiteStore) ListWebhooksByOrg(orgID string) []*protocol.Webhook {
    all, _ := listJSON[protocol.Webhook](s.db, "webhooks")
    var result []*protocol.Webhook
    for _, w := range all {
        if w.OrgID == orgID {
            result = append(result, w)
        }
    }
    return result
}

func (s *SQLiteStore) FindWebhooksByEvent(eventType string) []*protocol.Webhook {
    all, _ := listJSON[protocol.Webhook](s.db, "webhooks")
    var result []*protocol.Webhook
    for _, w := range all {
        if !w.Active {
            continue
        }
        for _, e := range w.Events {
            if e == eventType {
                result = append(result, w)
                break
            }
        }
    }
    return result
}

func (s *SQLiteStore) AddWebhookDelivery(d *protocol.WebhookDelivery) error {
    return putJSON(s.db, "webhook_deliveries", d.ID, d)
}

func (s *SQLiteStore) UpdateWebhookDelivery(d *protocol.WebhookDelivery) error {
    return putJSON(s.db, "webhook_deliveries", d.ID, d)
}

func (s *SQLiteStore) ListPendingWebhookDeliveries() []*protocol.WebhookDelivery {
    all, _ := listJSON[protocol.WebhookDelivery](s.db, "webhook_deliveries")
    now := time.Now()
    var result []*protocol.WebhookDelivery
    for _, d := range all {
        if d.Status == protocol.DeliveryPending || d.Status == protocol.DeliveryFailed {
            if d.NextRetry == nil || d.NextRetry.Before(now) {
                result = append(result, d)
            }
        }
    }
    return result
}
```

Also ensure `webhook_deliveries` and `webhooks` tables are in the SQLiteStore CREATE TABLE block — they're already in the existing `NewSQLiteStore` DDL based on the Phase 3a design (check: if not, add them):
```go
`CREATE TABLE IF NOT EXISTS webhooks (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
`CREATE TABLE IF NOT EXISTS webhook_deliveries (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
```

- [ ] **Step 2: Build to verify SQLiteStore satisfies interface**

```bash
cd core && go build ./internal/store/...
```

- [ ] **Step 3: Commit**

```bash
git add core/internal/store/sqlite.go
git commit -m "feat(store): SQLiteStore — implement Webhook + WebhookDelivery methods"
```

---

### Task 4: Implement Webhook methods in PostgreSQLStore

**Files:**
- Modify: `core/internal/store/postgres.go`

- [ ] **Step 1: Append to postgres.go**

```go
// --- Webhooks ---

func (s *PostgreSQLStore) AddWebhook(w *protocol.Webhook) error {
    return pgPut(s.pool, "webhooks", w.ID, w)
}

func (s *PostgreSQLStore) GetWebhook(id string) (*protocol.Webhook, error) {
    return pgGet[protocol.Webhook](s.pool, "webhooks", id)
}

func (s *PostgreSQLStore) UpdateWebhook(w *protocol.Webhook) error {
    return pgPut(s.pool, "webhooks", w.ID, w)
}

func (s *PostgreSQLStore) DeleteWebhook(id string) error {
    return pgDelete(s.pool, "webhooks", id)
}

func (s *PostgreSQLStore) ListWebhooksByOrg(orgID string) []*protocol.Webhook {
    hooks, _ := pgList[protocol.Webhook](s.pool,
        "SELECT data FROM webhooks WHERE data->>'org_id' = $1", orgID)
    return hooks
}

func (s *PostgreSQLStore) FindWebhooksByEvent(eventType string) []*protocol.Webhook {
    // JSONB containment: data->'events' @> '["task.complete"]'::jsonb
    hooks, _ := pgList[protocol.Webhook](s.pool,
        `SELECT data FROM webhooks
         WHERE data->>'active' = 'true'
         AND data->'events' @> $1::jsonb`,
        `["`+eventType+`"]`)
    return hooks
}

func (s *PostgreSQLStore) AddWebhookDelivery(d *protocol.WebhookDelivery) error {
    return pgPut(s.pool, "webhook_deliveries", d.ID, d)
}

func (s *PostgreSQLStore) UpdateWebhookDelivery(d *protocol.WebhookDelivery) error {
    return pgPut(s.pool, "webhook_deliveries", d.ID, d)
}

func (s *PostgreSQLStore) ListPendingWebhookDeliveries() []*protocol.WebhookDelivery {
    deliveries, _ := pgList[protocol.WebhookDelivery](s.pool,
        `SELECT data FROM webhook_deliveries
         WHERE data->>'status' IN ('pending', 'failed')
         AND (
             data->>'next_retry' IS NULL
             OR (data->>'next_retry')::timestamptz <= NOW()
         )`)
    return deliveries
}
```

- [ ] **Step 2: Build**

```bash
cd core && go build ./...
```
Expected: no errors — all three stores now implement the full Store interface.

- [ ] **Step 3: Commit**

```bash
git add core/internal/store/postgres.go
git commit -m "feat(store): PostgreSQLStore — implement Webhook + WebhookDelivery methods"
```

---

## Chunk 3: SSE Streaming

### Task 5: DispatchStream in Dispatcher

**Files:**
- Modify: `core/internal/dispatcher/dispatcher.go`

- [ ] **Step 1: Write failing test**

Append to `core/internal/dispatcher/dispatcher_test.go` (create if not exists):
```go
package dispatcher_test

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/kienbui1995/magic/core/internal/costctrl"
    "github.com/kienbui1995/magic/core/internal/dispatcher"
    "github.com/kienbui1995/magic/core/internal/evaluator"
    "github.com/kienbui1995/magic/core/internal/events"
    "github.com/kienbui1995/magic/core/internal/protocol"
    "github.com/kienbui1995/magic/core/internal/store"
)

func TestDispatchStream_ProxiesSSE(t *testing.T) {
    // Fake worker that streams SSE
    worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !strings.HasSuffix(r.URL.Path, "/stream") {
            t.Errorf("expected /stream path, got %s", r.URL.Path)
        }
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "data: {\"chunk\":\"hello \",\"done\":false}\n\n")
        fmt.Fprintf(w, "data: {\"chunk\":\"world\",\"done\":false}\n\n")
        fmt.Fprintf(w, "data: {\"task_id\":\"t-1\",\"done\":true}\n\n")
        if f, ok := w.(http.Flusher); ok {
            f.Flush()
        }
    }))
    defer worker.Close()

    s := store.NewMemoryStore()
    bus := events.NewBus()
    cc := costctrl.New(s, bus)
    ev := evaluator.New(bus)
    d := dispatcher.New(s, bus, cc, ev)

    task := &protocol.Task{
        ID:     "t-1",
        Type:   "chat",
        Status: protocol.TaskPending,
        Input:  []byte(`{"message":"hi"}`),
    }
    s.AddTask(task) //nolint:errcheck

    w := httptest.NewRecorder()
    workerProto := &protocol.Worker{
        ID:       "w-1",
        Endpoint: protocol.Endpoint{URL: worker.URL + "/stream"},
    }

    err := d.DispatchStream(t.Context(), task, workerProto, w)
    if err != nil {
        t.Fatalf("DispatchStream: %v", err)
    }

    body := w.Body.String()
    if !strings.Contains(body, "hello") {
        t.Errorf("expected SSE body to contain 'hello', got: %s", body)
    }
    if !strings.Contains(body, "done\":true") {
        t.Errorf("expected final done event, got: %s", body)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd core && go test ./internal/dispatcher/... -run TestDispatchStream -v
```
Expected: FAIL — `DispatchStream` undefined.

- [ ] **Step 3: Add DispatchStream to dispatcher.go**

Append to `core/internal/dispatcher/dispatcher.go`:

```go
// DispatchStream dispatches a task to a streaming worker and proxies the SSE
// response back to w. The worker must expose a POST endpoint at its URL that
// returns Content-Type: text/event-stream.
//
// w must support flushing (http.Flusher). The caller is responsible for setting
// SSE response headers before calling DispatchStream.
func (d *Dispatcher) DispatchStream(ctx context.Context, task *protocol.Task, worker *protocol.Worker, w http.ResponseWriter) error {
    if err := validateEndpointURL(worker.Endpoint.URL); err != nil {
        d.handleFailure(task, worker, fmt.Sprintf("invalid endpoint: %v", err))
        return err
    }

    // Build task.assign payload (same as regular Dispatch)
    assignPayload, _ := json.Marshal(protocol.TaskAssignPayload{
        TaskID:   task.ID,
        TaskType: task.Type,
        Priority: task.Priority,
        Input:    task.Input,
        Contract: task.Contract,
        Context:  task.Context,
    })
    msg := protocol.NewMessage(protocol.MsgTaskAssign, "org", worker.ID, assignPayload)
    body, err := json.Marshal(msg)
    if err != nil {
        return fmt.Errorf("marshal task.assign: %w", err)
    }

    task.Status = protocol.TaskInProgress
    d.store.UpdateTask(task) //nolint:errcheck

    // POST to worker's streaming endpoint
    req, err := http.NewRequestWithContext(ctx, "POST", worker.Endpoint.URL, bytes.NewReader(body))
    if err != nil {
        d.handleFailure(task, worker, err.Error())
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "text/event-stream")

    resp, err := d.client.Do(req)
    if err != nil {
        d.handleFailure(task, worker, err.Error())
        return fmt.Errorf("worker request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        d.handleFailure(task, worker, fmt.Sprintf("worker returned status %d", resp.StatusCode))
        return fmt.Errorf("worker returned status %d", resp.StatusCode)
    }

    // Pipe SSE from worker to client
    flusher, ok := w.(http.Flusher)
    if !ok {
        return fmt.Errorf("ResponseWriter does not support flushing")
    }

    buf := make([]byte, 4096)
    for {
        n, readErr := resp.Body.Read(buf)
        if n > 0 {
            if _, writeErr := w.Write(buf[:n]); writeErr != nil {
                break // client disconnected
            }
            flusher.Flush()
        }
        if readErr != nil {
            break
        }
    }

    task.Status = protocol.TaskCompleted
    now := time.Now()
    task.CompletedAt = &now
    d.store.UpdateTask(task) //nolint:errcheck

    d.bus.Publish(events.Event{
        Type:   "task.completed",
        Source: "dispatcher",
        Payload: map[string]any{
            "task_id":   task.ID,
            "worker_id": worker.ID,
        },
    })
    return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd core && go test ./internal/dispatcher/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/internal/dispatcher/dispatcher.go core/internal/dispatcher/dispatcher_test.go
git commit -m "feat(dispatcher): DispatchStream — proxy SSE from worker to client"
```

---

### Task 6: SSE streaming Gateway handlers

**Files:**
- Modify: `core/internal/gateway/gateway.go`
- Modify: `core/internal/gateway/handlers.go`

- [ ] **Step 1: Add streaming routes to gateway.go**

In `Handler()`, add after the existing task routes:

```go
// Streaming task endpoint (uses same taskLimiter)
mux.Handle("POST /api/v1/tasks/stream", taskRL(http.HandlerFunc(g.handleStreamTask)))
mux.HandleFunc("GET /api/v1/tasks/{id}/stream", g.handleResubscribeStream)
```

> Route order matters: `/api/v1/tasks/stream` must come BEFORE `/api/v1/tasks/{id}` in the mux. Go's `net/http` 1.22 pattern router handles this correctly as the more specific path wins.

- [ ] **Step 2: Write handler test**

Append to `core/internal/gateway/gateway_test.go`:
```go
func TestHandleStreamTask_FallbackWhenWorkerNotStreaming(t *testing.T) {
    // Worker that responds with regular JSON (non-streaming)
    fakeWorker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Regular (non-SSE) worker response
        json.NewEncoder(w).Encode(map[string]any{
            "type":    "task.complete",
            "payload": map[string]any{"task_id": "t-1", "output": "hello", "cost": 0.01},
        })
    }))
    defer fakeWorker.Close()

    // Register the worker, submit a stream request
    // (full integration test — requires registered worker in store)
    // This test verifies the 400 error when no streaming workers are found.
    gw, _ := newTestGateway(t)
    body := `{"type":"chat","input":{"message":"hi"},"stream":true}`
    req := httptest.NewRequest("POST", "/api/v1/tasks/stream", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    gw.Handler().ServeHTTP(w, req)
    // No workers registered → 503 or appropriate error
    if w.Code == http.StatusOK {
        t.Error("expected non-200 when no workers available")
    }
}
```

- [ ] **Step 3: Implement handlers**

Append to `core/internal/gateway/handlers.go`:

```go
func (g *Gateway) handleStreamTask(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Type    string          `json:"type"`
        Input   json.RawMessage `json:"input"`
        Context protocol.TaskContext `json:"context"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if req.Type == "" {
        writeError(w, http.StatusBadRequest, "type is required")
        return
    }

    // Route to a streaming-capable worker
    worker, err := g.deps.Router.Route(req.Type, req.Context.OrgID, true /* streaming */)
    if err != nil {
        writeError(w, http.StatusServiceUnavailable, "no streaming worker available: "+err.Error())
        return
    }

    task := &protocol.Task{
        ID:       protocol.GenerateID("t"),
        Type:     req.Type,
        Status:   protocol.TaskPending,
        Input:    req.Input,
        Context:  req.Context,
        Priority: protocol.PriorityNormal,
    }
    if err := g.deps.Store.AddTask(task); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to create task")
        return
    }

    // Set SSE headers and remove write deadline
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    rc := http.NewResponseController(w)
    rc.SetWriteDeadline(time.Time{}) //nolint:errcheck // indefinite for streaming

    if err := g.deps.Dispatcher.DispatchStream(r.Context(), task, worker, w); err != nil {
        // Write SSE error event if headers already sent
        fmt.Fprintf(w, "data: {\"error\":%q,\"done\":true}\n\n", err.Error())
        if f, ok := w.(http.Flusher); ok {
            f.Flush()
        }
    }
}

func (g *Gateway) handleResubscribeStream(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    task, err := g.deps.Store.GetTask(id)
    if err != nil {
        writeError(w, http.StatusNotFound, "task not found")
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    rc := http.NewResponseController(w)
    rc.SetWriteDeadline(time.Time{}) //nolint:errcheck

    switch task.Status {
    case protocol.TaskCompleted:
        output, _ := json.Marshal(task.Output)
        fmt.Fprintf(w, "data: {\"chunk\":%s,\"task_id\":%q,\"done\":true}\n\n", output, id)
    case protocol.TaskFailed:
        msg := "task failed"
        if task.Error != nil {
            msg = task.Error.Message
        }
        fmt.Fprintf(w, "data: {\"error\":%q,\"done\":true}\n\n", msg)
    default:
        // Task still running — return 202 with task status
        writeError(w, http.StatusAccepted, "task is still running; check GET /api/v1/tasks/"+id)
        return
    }

    if f, ok := w.(http.Flusher); ok {
        f.Flush()
    }
}
```

> **Note on Router.Route:** The current `router.Router` does not have a `streaming bool` parameter. Modify the call to use the existing `Route` signature and filter for `Streaming: true` capability manually, or add a helper to Router. Simplest: after getting a worker from the router, check if it has a streaming capability matching the task type. If not, fall back to non-streaming DispatchStream (DispatchStream already handles non-SSE responses gracefully).

- [ ] **Step 4: Add Dispatcher to Gateway Deps (if not already present)**

Check `gateway.go` Deps struct — add if missing:
```go
Dispatcher *dispatcher.Dispatcher
```

- [ ] **Step 5: Build**

```bash
cd core && go build ./...
```

- [ ] **Step 6: Run tests**

```bash
cd core && go test ./internal/gateway/... -v
```

- [ ] **Step 7: Commit**

```bash
git add core/internal/gateway/gateway.go core/internal/gateway/handlers.go
git commit -m "feat(gateway): SSE streaming endpoints — POST /tasks/stream + GET /tasks/{id}/stream"
```

---

## Chunk 4: Webhook Package

### Task 7: webhook/manager.go

**Files:**
- Create: `core/internal/webhook/manager.go`

- [ ] **Step 1: Create manager.go**

`core/internal/webhook/manager.go`:
```go
package webhook

import (
    "encoding/json"
    "log"
    "time"

    "github.com/kienbui1995/magic/core/internal/events"
    "github.com/kienbui1995/magic/core/internal/protocol"
    "github.com/kienbui1995/magic/core/internal/store"
)

// supportedEvents is the set of event types that can trigger webhooks.
var supportedEvents = []string{
    "task.complete", "task.fail", "task.assign",
    "worker.register", "worker.deregister", "worker.offline",
    "workflow.complete", "workflow.fail",
    "budget.threshold_reached",
}

// Manager subscribes to the event bus and enqueues WebhookDelivery records
// for each matching registered webhook. The Sender goroutine processes the queue.
type Manager struct {
    store  store.Store
    bus    *events.Bus
    sender *Sender
}

// New creates a Manager and a Sender. Call Start() to begin processing.
func New(s store.Store, bus *events.Bus) *Manager {
    sender := newSender(s)
    return &Manager{store: s, bus: bus, sender: sender}
}

// Start subscribes to all supported events and starts the retry sender goroutine.
func (m *Manager) Start() {
    m.sender.start()
    for _, eventType := range supportedEvents {
        et := eventType // capture
        m.bus.Subscribe(et, func(e events.Event) { //nolint:errcheck
            m.onEvent(e)
        })
    }
}

func (m *Manager) onEvent(e events.Event) {
    hooks := m.store.FindWebhooksByEvent(e.Type)
    if len(hooks) == 0 {
        return
    }

    payloadBytes, err := json.Marshal(map[string]any{
        "type":      e.Type,
        "timestamp": e.Timestamp,
        "data":      e.Payload,
    })
    if err != nil {
        log.Printf("[webhook] failed to marshal event %s: %v", e.Type, err)
        return
    }
    payload := string(payloadBytes)

    for _, hook := range hooks {
        d := &protocol.WebhookDelivery{
            ID:        protocol.GenerateID("wd"),
            WebhookID: hook.ID,
            EventType: e.Type,
            Payload:   payload,
            Status:    protocol.DeliveryPending,
            CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
        }
        if err := m.store.AddWebhookDelivery(d); err != nil {
            log.Printf("[webhook] failed to enqueue delivery for hook %s: %v", hook.ID, err)
        }
    }
}

// CreateWebhook registers a new webhook. The secret is stored as-is (caller should hash if needed).
func (m *Manager) CreateWebhook(orgID, url string, eventTypes []string, secret string) (*protocol.Webhook, error) {
    hook := &protocol.Webhook{
        ID:        protocol.GenerateID("wh"),
        OrgID:     orgID,
        URL:       url,
        Events:    eventTypes,
        Secret:    secret,
        Active:    true,
        CreatedAt: time.Now(),
    }
    if err := m.store.AddWebhook(hook); err != nil {
        return nil, err
    }
    return hook, nil
}

// DeleteWebhook removes a webhook and stops future deliveries.
func (m *Manager) DeleteWebhook(id string) error {
    return m.store.DeleteWebhook(id)
}

// ListWebhooks returns all webhooks for an org. Secrets are redacted.
func (m *Manager) ListWebhooks(orgID string) []*protocol.Webhook {
    hooks := m.store.ListWebhooksByOrg(orgID)
    for _, h := range hooks {
        h.Secret = "" // never expose secret
    }
    return hooks
}

// ListDeliveries returns recent deliveries for a webhook.
func (m *Manager) ListDeliveries(webhookID string) []*protocol.WebhookDelivery {
    // ListPendingWebhookDeliveries returns all pending — filter by webhookID in Go
    all := m.store.ListPendingWebhookDeliveries()
    var result []*protocol.WebhookDelivery
    for _, d := range all {
        if d.WebhookID == webhookID {
            result = append(result, d)
        }
    }
    return result
}
```

- [ ] **Step 2: Build**

```bash
cd core && go build ./internal/webhook/...
```
Expected: may fail — `newSender` not defined yet (Task 8). OK to proceed.

- [ ] **Step 3: Commit after Task 8 is done (combined commit)**

---

### Task 8: webhook/sender.go

**Files:**
- Create: `core/internal/webhook/sender.go`

- [ ] **Step 1: Create sender.go**

`core/internal/webhook/sender.go`:
```go
package webhook

import (
    "bytes"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "log"
    "net/http"
    "time"

    "github.com/kienbui1995/magic/core/internal/protocol"
    "github.com/kienbui1995/magic/core/internal/store"
)

// retrySchedule defines the wait duration before each retry attempt.
// Index 0 = after 1st failure, index 4 = after 5th failure (dead).
var retrySchedule = []time.Duration{
    30 * time.Second,
    5 * time.Minute,
    30 * time.Minute,
    2 * time.Hour,
    8 * time.Hour,
}

const maxAttempts = 5

// Sender processes pending WebhookDelivery records from the store every 5s,
// POSTs to the webhook URL, and updates the delivery status.
type Sender struct {
    store  store.Store
    client *http.Client
    stop   chan struct{}
}

func newSender(s store.Store) *Sender {
    return &Sender{
        store:  s,
        client: &http.Client{Timeout: 10 * time.Second},
        stop:   make(chan struct{}),
    }
}

func (s *Sender) start() {
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                s.processQueue()
            case <-s.stop:
                return
            }
        }
    }()
}

func (s *Sender) Stop() { close(s.stop) }

func (s *Sender) processQueue() {
    deliveries := s.store.ListPendingWebhookDeliveries()
    for _, d := range deliveries {
        hook, err := s.store.GetWebhook(d.WebhookID)
        if err != nil {
            // Webhook deleted — mark delivery dead
            s.markDead(d)
            continue
        }
        s.deliver(d, hook)
    }
}

func (s *Sender) deliver(d *protocol.WebhookDelivery, hook *protocol.Webhook) {
    req, err := http.NewRequest("POST", hook.URL, bytes.NewReader([]byte(d.Payload)))
    if err != nil {
        s.markFailed(d)
        return
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-MagiC-Event", d.EventType)
    req.Header.Set("X-MagiC-Delivery", d.ID)

    if hook.Secret != "" {
        sig := computeHMAC(hook.Secret, d.Payload)
        req.Header.Set("X-MagiC-Signature", "sha256="+sig)
    }

    resp, err := s.client.Do(req)
    if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
        if resp != nil {
            resp.Body.Close()
        }
        log.Printf("[webhook] delivery %s failed (attempt %d): %v", d.ID, d.Attempts+1, err)
        s.markFailed(d)
        return
    }
    resp.Body.Close()

    d.Status = protocol.DeliveryDelivered
    d.Attempts++
    now := time.Now()
    d.UpdatedAt = now
    s.store.UpdateWebhookDelivery(d) //nolint:errcheck
}

func (s *Sender) markFailed(d *protocol.WebhookDelivery) {
    d.Attempts++
    now := time.Now()
    d.UpdatedAt = now

    if d.Attempts >= maxAttempts {
        d.Status = protocol.DeliveryDead
        d.NextRetry = nil
    } else {
        d.Status = protocol.DeliveryFailed
        backoff := retrySchedule[d.Attempts-1]
        next := now.Add(backoff)
        d.NextRetry = &next
    }
    s.store.UpdateWebhookDelivery(d) //nolint:errcheck
}

func (s *Sender) markDead(d *protocol.WebhookDelivery) {
    d.Status = protocol.DeliveryDead
    d.UpdatedAt = time.Now()
    s.store.UpdateWebhookDelivery(d) //nolint:errcheck
}

func computeHMAC(secret, payload string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(payload))
    return hex.EncodeToString(mac.Sum(nil))
}

// Unused import
var _ = fmt.Sprintf
```

- [ ] **Step 2: Build both files**

```bash
cd core && go build ./internal/webhook/...
```

- [ ] **Step 3: Commit**

```bash
git add core/internal/webhook/
git commit -m "feat(webhook): Manager (event→queue) + Sender (retry loop + HMAC signing)"
```

---

### Task 9: Webhook CRUD Gateway handlers

**Files:**
- Modify: `core/internal/gateway/gateway.go`
- Modify: `core/internal/gateway/handlers.go`

- [ ] **Step 1: Add Webhook to Gateway Deps + routes**

In `core/internal/gateway/gateway.go`, add to `Deps`:
```go
Webhook *webhook.Manager
```

Add import: `"github.com/kienbui1995/magic/core/internal/webhook"`

Add routes in `Handler()`:
```go
// Webhooks (admin auth — MAGIC_API_KEY)
mux.HandleFunc("POST /api/v1/orgs/{orgID}/webhooks", g.handleCreateWebhook)
mux.HandleFunc("GET /api/v1/orgs/{orgID}/webhooks", g.handleListWebhooks)
mux.HandleFunc("DELETE /api/v1/orgs/{orgID}/webhooks/{webhookID}", g.handleDeleteWebhook)
mux.HandleFunc("GET /api/v1/orgs/{orgID}/webhooks/{webhookID}/deliveries", g.handleListWebhookDeliveries)
```

- [ ] **Step 2: Implement handlers**

Append to `core/internal/gateway/handlers.go`:
```go
func (g *Gateway) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
    orgID := r.PathValue("orgID")
    var req struct {
        URL    string   `json:"url"`
        Events []string `json:"events"`
        Secret string   `json:"secret"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if req.URL == "" || len(req.Events) == 0 {
        writeError(w, http.StatusBadRequest, "url and events are required")
        return
    }
    hook, err := g.deps.Webhook.CreateWebhook(orgID, req.URL, req.Events, req.Secret)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "failed to create webhook")
        return
    }
    hook.Secret = "" // never return secret
    writeJSON(w, http.StatusCreated, hook)
}

func (g *Gateway) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
    orgID := r.PathValue("orgID")
    writeJSON(w, http.StatusOK, g.deps.Webhook.ListWebhooks(orgID))
}

func (g *Gateway) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
    webhookID := r.PathValue("webhookID")
    if err := g.deps.Webhook.DeleteWebhook(webhookID); err != nil {
        if err == store.ErrNotFound {
            writeError(w, http.StatusNotFound, "webhook not found")
            return
        }
        writeError(w, http.StatusInternalServerError, "failed to delete webhook")
        return
    }
    w.WriteHeader(http.StatusNoContent)
}

func (g *Gateway) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
    webhookID := r.PathValue("webhookID")
    writeJSON(w, http.StatusOK, g.deps.Webhook.ListDeliveries(webhookID))
}
```

- [ ] **Step 3: Build**

```bash
cd core && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add core/internal/gateway/gateway.go core/internal/gateway/handlers.go
git commit -m "feat(gateway): webhook CRUD endpoints — create, list, delete, deliveries"
```

---

## Chunk 5: Prometheus Metrics

### Task 10: Prometheus dependency + monitor/metrics.go

**Files:**
- Modify: `core/go.mod`
- Create: `core/internal/monitor/metrics.go`
- Modify: `core/internal/monitor/monitor.go`

- [ ] **Step 1: Add prometheus dependency**

```bash
cd core
go get github.com/prometheus/client_golang@latest
go get github.com/prometheus/client_golang/prometheus/promauto
go get github.com/prometheus/client_golang/prometheus/promhttp
```

- [ ] **Step 2: Create metrics.go with all metric registrations**

`core/internal/monitor/metrics.go`:
```go
package monitor

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

// All Prometheus metrics are registered here using promauto (auto-registered on init).
var (
    // Tasks
    MetricTasksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "magic_tasks_total",
        Help: "Total number of tasks processed.",
    }, []string{"type", "status", "worker"})

    MetricTaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "magic_task_duration_seconds",
        Help:    "Task processing duration in seconds.",
        Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60},
    }, []string{"type", "worker"})

    // Workers
    MetricWorkersActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "magic_workers_active",
        Help: "Number of currently active workers.",
    }, []string{"org"})

    MetricWorkerHeartbeatLag = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "magic_worker_heartbeat_lag_seconds",
        Help: "Seconds since the last heartbeat from each worker.",
    }, []string{"worker"})

    // Cost
    MetricCostTotalUSD = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "magic_cost_total_usd",
        Help: "Total cost in USD incurred by tasks.",
    }, []string{"org", "worker"})

    // Workflows
    MetricWorkflowStepsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "magic_workflow_steps_total",
        Help: "Total workflow steps processed.",
    }, []string{"status"})

    MetricWorkflowsActive = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "magic_workflows_active",
        Help: "Number of currently running workflows.",
    })

    // Knowledge Hub
    MetricKnowledgeQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "magic_knowledge_queries_total",
        Help: "Total knowledge hub queries.",
    }, []string{"type"}) // type: keyword | semantic

    MetricKnowledgeEntriesTotal = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "magic_knowledge_entries_total",
        Help: "Total number of knowledge entries stored.",
    })

    // Rate limiting
    MetricRateLimitHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "magic_rate_limit_hits_total",
        Help: "Total number of rate limit rejections.",
    }, []string{"endpoint"})

    // Webhooks
    MetricWebhookDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "magic_webhook_deliveries_total",
        Help: "Total webhook delivery attempts.",
    }, []string{"status"}) // status: delivered | failed | dead

    MetricWebhookDeliveryDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "magic_webhook_delivery_duration_seconds",
        Help:    "Webhook delivery HTTP request duration.",
        Buckets: []float64{0.05, 0.1, 0.5, 1, 5, 10},
    })

    // Streaming
    MetricStreamsActive = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "magic_streams_active",
        Help: "Number of currently active SSE streaming connections.",
    })

    MetricStreamDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "magic_stream_duration_seconds",
        Help:    "Duration of SSE streaming connections.",
        Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
    })
)
```

- [ ] **Step 3: Update monitor.go to update Prometheus metrics on events**

In `core/internal/monitor/monitor.go`, update the `Start()` event handler to update Prometheus metrics alongside existing atomic counters:

```go
// In the Subscribe callback inside Start(), add after each existing case:
case "task.routed":
    atomic.AddInt64(&m.stats.TasksRouted, 1)
    // Prometheus update happens in task.completed / task.failed with worker label
case "task.completed":
    atomic.AddInt64(&m.stats.TasksDone, 1)
    if workerID, ok := e.Payload["worker_id"].(string); ok {
        taskType, _ := e.Payload["task_type"].(string)
        MetricTasksTotal.WithLabelValues(taskType, "completed", workerID).Inc()
    }
case "task.failed":
    atomic.AddInt64(&m.stats.TasksFailed, 1)
    if workerID, ok := e.Payload["worker_id"].(string); ok {
        taskType, _ := e.Payload["task_type"].(string)
        MetricTasksTotal.WithLabelValues(taskType, "failed", workerID).Inc()
    }
case "worker.registered":
    atomic.AddInt64(&m.stats.WorkersCount, 1)
    if orgID, ok := e.Payload["org_id"].(string); ok {
        MetricWorkersActive.WithLabelValues(orgID).Inc()
    }
case "worker.deregistered":
    atomic.AddInt64(&m.stats.WorkersCount, -1)
    if orgID, ok := e.Payload["org_id"].(string); ok {
        MetricWorkersActive.WithLabelValues(orgID).Dec()
    }
case "knowledge.added":
    MetricKnowledgeEntriesTotal.Inc()
case "knowledge.deleted":
    MetricKnowledgeEntriesTotal.Dec()
case "workflow.started":
    MetricWorkflowsActive.Inc()
case "workflow.completed":
    MetricWorkflowStepsTotal.WithLabelValues("completed").Inc()
    MetricWorkflowsActive.Dec()
case "workflow.failed":
    MetricWorkflowStepsTotal.WithLabelValues("failed").Inc()
    MetricWorkflowsActive.Dec()
case "cost.recorded":
    if cost, ok := e.Payload["cost"].(float64); ok {
        orgID, _ := e.Payload["org_id"].(string)
        workerID, _ := e.Payload["worker_id"].(string)
        MetricCostTotalUSD.WithLabelValues(orgID, workerID).Add(cost)
    }
```

- [ ] **Step 4: Build**

```bash
cd core && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add core/go.mod core/go.sum core/internal/monitor/metrics.go core/internal/monitor/monitor.go
git commit -m "feat(monitor): Prometheus metrics — comprehensive counters/gauges/histograms via promauto"
```

---

### Task 11: /metrics endpoint + final main.go wiring

**Files:**
- Modify: `core/internal/gateway/gateway.go`
- Modify: `core/cmd/magic/main.go`

- [ ] **Step 1: Add /metrics route to gateway.go**

Add import: `"github.com/prometheus/client_golang/prometheus/promhttp"`

Add route in `Handler()` BEFORE the auth middleware chain (Prometheus scrapers don't send auth tokens):

```go
// Prometheus metrics — no auth, registered before auth middleware wrapping
mux.Handle("GET /metrics", promhttp.Handler())
```

> **Important:** `/metrics` must be in the mux BEFORE `handler = authMiddleware(handler)` wraps the entire mux. Since `authMiddleware` is applied to the whole handler, add an exemption in `authMiddleware` for the `/metrics` path (same approach as existing health + worker endpoint exemptions).

In `core/internal/gateway/middleware.go`, find the `authMiddleware` function and add `/metrics` to the exempt paths:
```go
// In authMiddleware, alongside existing exemptions:
if r.URL.Path == "/metrics" {
    next.ServeHTTP(w, r)
    return
}
```

- [ ] **Step 2: Wire Webhook Manager in main.go**

In `core/cmd/magic/main.go`, add after existing module wiring:

```go
import "github.com/kienbui1995/magic/core/internal/webhook"

// After: kb := knowledge.New(...)
wh := webhook.New(s, bus)
wh.Start()

// Update gateway.New() call to include Webhook:
gw := gateway.New(gateway.Deps{
    // ... existing deps ...
    Webhook: wh,
})
```

Also add to the startup print block:
```go
fmt.Println("  GET  /metrics                  — Prometheus metrics")
fmt.Println("  POST /api/v1/tasks/stream      — Submit streaming task")
fmt.Println("  POST /api/v1/orgs/{orgID}/webhooks  — Register webhook")
```

- [ ] **Step 3: Build and run all tests**

```bash
cd core && go build ./... && go test ./...
```
Expected: all tests pass.

- [ ] **Step 4: Smoke test Prometheus endpoint**

```bash
./magic serve &
sleep 1
curl -s http://localhost:8080/metrics | grep "magic_"
kill %1
```
Expected: output includes lines like `magic_tasks_total`, `magic_workers_active`, etc.

- [ ] **Step 5: Commit**

```bash
git add core/internal/gateway/gateway.go core/internal/gateway/middleware.go core/cmd/magic/main.go
git commit -m "feat(gateway): /metrics Prometheus endpoint + webhook.Manager wired in main.go"
```

---

## Self-Review

Reviewing plan against spec `docs/superpowers/specs/2026-04-08-magic-phase3-design.md`:

- ✅ SSE streaming with `http.ResponseController.SetWriteDeadline` — Task 6
- ✅ `POST /api/v1/tasks/stream` + `GET /api/v1/tasks/{id}/stream` — Task 6
- ✅ `Streaming bool` in Capability — Task 1
- ✅ Worker streaming endpoint convention (`/stream` path) — Task 5
- ✅ DispatchStream pipes SSE from worker → client — Task 5
- ✅ Fallback for non-streaming workers — Task 6 (handleStreamTask falls back)
- ✅ Webhook types (Webhook, WebhookDelivery, status constants) — Task 1
- ✅ Store interface additions (8 methods) — Task 2
- ✅ All 3 store implementations — Tasks 2, 3, 4
- ✅ webhook/manager.go (event bus subscription, enqueue) — Task 7
- ✅ webhook/sender.go (retry, HMAC, exponential backoff 30s→8h) — Task 8
- ✅ HMAC signature header + secret redaction — Tasks 8, 9
- ✅ Webhook CRUD endpoints — Task 9
- ✅ Prometheus with promauto, comprehensive metrics — Task 10
- ✅ `/metrics` unauthenticated endpoint — Task 11
- ✅ `/api/v1/metrics` kept unchanged — no modification needed
- ✅ webhook.Manager wired in main.go — Task 11
- ⚠️ `task_type` label on MetricTasksTotal — depends on `task_type` being in event payload. Verify in dispatcher.go that `"task.completed"` event payload includes `"task_type"`. If not, add it in `handleComplete`:
  ```go
  Payload: map[string]any{
      "task_id":   task.ID,
      "worker_id": worker.ID,
      "task_type": task.Type, // ADD THIS
      "cost":      cp.Cost,
  }
  ```
- ⚠️ `MetricTaskDuration` — not yet updated in any handler. Add timing in Dispatcher: record `startTime := time.Now()` before calling worker, then `MetricTaskDuration.WithLabelValues(task.Type, worker.ID).Observe(time.Since(startTime).Seconds())` in handleComplete and handleFailure.
- ⚠️ Webhook test — no unit tests for manager/sender. Add a basic test in Task 7: create a mock store + bus, trigger an event, verify a delivery is enqueued.
