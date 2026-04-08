# MagiC Phase 3 Design Spec: Storage, Streaming & Observability

**Date:** 2026-04-08  
**Status:** Approved  
**Scope:** Two implementation phases — 3a (Storage Layer) and 3b (Real-time & Observability)

---

## Context

This spec was triggered by a community feature request from a team building VibeCode Adaptive (AI-powered education platform) on top of MagiC. After evaluating all 10 requested features against MagiC's infrastructure philosophy ("thin infrastructure layer + extension points"), we selected 5 features to implement:

**In scope:**
1. PostgreSQL backend (alongside SQLite)
2. pgvector for semantic search in Knowledge Hub
3. SSE streaming for task output
4. Webhooks with at-least-once delivery
5. Prometheus metrics (comprehensive)

**Out of scope (application concerns, not framework):**
- OAuth/JWT user authentication
- File upload handling
- Granular rate limiting per tier
- WebSocket (SSE is sufficient for all streaming use cases)

**Already exists (feature request didn't know):**
- SQLite persistent storage (`store/sqlite.go`)
- Rate limiting (`gateway/ratelimit.go`)
- Go SDK (`sdk/go/`)

---

## Phase 3a: Storage Layer

### PostgreSQL Backend

**Approach:** JSONB storage — same pattern as SQLiteStore (`id TEXT PRIMARY KEY, data JSONB NOT NULL`).

**Why JSONB over normalized columns:**
- `protocol.*` types evolve frequently — normalized schema requires migrations on every field addition
- JSONB supports GIN indexes for fast JSON field queries
- Minimal code divergence from SQLiteStore implementation
- Proven pattern in production systems (Grafana, etc.)

**Driver:** `github.com/jackc/pgx/v5` + `pgxpool` for connection pooling.

**Migration system:** `github.com/golang-migrate/migrate/v4` — versioned up/down SQL migrations in `core/migrations/`.

**Schema (all tables):**
```sql
CREATE TABLE workers          (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE tasks            (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE workflows        (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE teams            (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE knowledge        (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE worker_tokens    (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE audit_log        (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE webhooks         (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE webhook_deliveries (id TEXT PRIMARY KEY, data JSONB NOT NULL);

-- Indexes for common queries
CREATE INDEX idx_workers_org  ON workers  ((data->>'org_id'));
CREATE INDEX idx_tasks_org    ON tasks    ((data->>'org_id'));
CREATE INDEX idx_tasks_status ON tasks    ((data->>'status'));
CREATE INDEX idx_audit_org    ON audit_log((data->>'org_id'));
CREATE INDEX idx_wh_del_status ON webhook_deliveries((data->>'status'));
```

**Configuration:**
```env
MAGIC_POSTGRES_URL=postgres://user:pass@host:5432/magic?sslmode=require
MAGIC_POSTGRES_POOL_MIN=2
MAGIC_POSTGRES_POOL_MAX=20
```

**Store selection logic in `main.go`** (auto-detect, no explicit flag needed):
```
if MAGIC_POSTGRES_URL set  → PostgreSQLStore
elif MAGIC_STORE set       → SQLiteStore (existing)
else                       → MemoryStore (existing)
```

**New files:**
```
core/internal/store/
├── postgres.go        # PostgreSQLStore implementing Store interface
├── postgres_test.go   # integration tests (requires running PostgreSQL)
core/migrations/
├── 001_initial.up.sql
├── 001_initial.down.sql
```

---

### pgvector — Semantic Search in Knowledge Hub

**Architecture decision:** `VectorStore` is a separate interface in the `knowledge` package, NOT added to the `Store` interface. This avoids forcing SQLiteStore to implement stub methods for an unsupported operation.

**`VectorStore` interface (`knowledge/vector.go`):**
```go
type VectorStore interface {
    Upsert(id string, vector []float32, meta map[string]any) error
    Search(queryVector []float32, topK int) ([]SearchResult, error)
    Delete(id string) error
}

type SearchResult struct {
    ID         string         `json:"id"`
    Score      float32        `json:"score"`      // 0.0–1.0 cosine similarity
    Metadata   map[string]any `json:"metadata"`
}
```

**Hub update (`knowledge/hub.go`):**
```go
type Hub struct {
    store   store.Store
    bus     *events.Bus
    vectors VectorStore // nil if not configured — semantic search unavailable
}

func New(s store.Store, bus *events.Bus, vs VectorStore) *Hub { ... }
```

**`PGVectorStore` implementation (`store/pgvector.go`):**
```sql
-- Migration adds:
CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE knowledge_embeddings (
    id      TEXT PRIMARY KEY,
    vector  vector(1536),         -- dimension set by MAGIC_PGVECTOR_DIM (default: 1536)
    meta    JSONB
);
CREATE INDEX ON knowledge_embeddings USING ivfflat (vector vector_cosine_ops) WITH (lists = 100);
```

**Config:** `MAGIC_PGVECTOR_DIM=1536` — must match the embedding model dimension (1536 = text-embedding-3-small, 3072 = text-embedding-3-large). Set at server startup; changing requires re-creating the table.

**Auto-injection in `main.go`:** When PostgreSQL backend is used, instantiate `PGVectorStore` and inject into Hub. When SQLite/Memory → `vs = nil`.

**New API endpoints:**
```
POST /api/v1/knowledge/{id}/embedding   # Worker pushes embedding for an entry
POST /api/v1/knowledge/search/semantic  # Semantic search
```

**Semantic search request:**
```json
{
  "query_vector": [0.1, 0.2, ...],   // pre-computed by worker via LiteLLM
  "top_k": 5,
  "scope": "org",
  "scope_id": "org-123"
}
```

**Why workers generate embeddings (not MagiC):** MagiC is infrastructure — it doesn't call LLM APIs directly. Workers use LiteLLM proxy to generate embeddings, then push vectors to MagiC's Knowledge Hub. This keeps LLM concerns in worker code, not framework code.

**When `vectors == nil`:** `POST /api/v1/knowledge/search/semantic` returns `501 Not Implemented` with message `"semantic search requires PostgreSQL backend with pgvector"`.

---

## Phase 3b: Real-time & Observability

### SSE Streaming

**Approach:** MagiC Gateway proxies SSE from worker to client. Workers stay private (never exposed directly). Auth, audit, and rate limiting apply uniformly at the Gateway.

**Two new endpoints:**
```
POST /api/v1/tasks/stream         # Submit task + stream result (single request)
GET  /api/v1/tasks/{id}/stream    # Re-subscribe to existing task stream (reconnection)
```

**WriteTimeout problem:** HTTP server has `WriteTimeout: 60s`. SSE connections must be indefinite.  
**Solution:** `http.ResponseController.SetWriteDeadline(time.Time{})` per streaming request (Go 1.20+). No global server config change needed.

**SSE response format:**
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

data: {"chunk": "Hello ", "done": false}

data: {"chunk": "world!", "done": false}

data: {"task_id": "t-abc123", "cost": 0.002, "done": true}

```

**Worker capability declaration:**
```go
// Add to protocol.Capability
type Capability struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"input_schema,omitempty"`
    OutputSchema json.RawMessage `json:"output_schema,omitempty"`
    EstCostPerCall float64      `json:"est_cost_per_call,omitempty"`
    Streaming   bool            `json:"streaming,omitempty"`  // NEW
}
```

**Worker streaming endpoint convention:** Workers that support streaming expose `POST {endpoint}/stream`. Same request body as regular `/execute`. Response: SSE format.

**Dispatcher changes:** New method `DispatchStream(ctx context.Context, task *protocol.Task, worker *protocol.Worker, w http.ResponseWriter) error`. Calls `POST {worker.endpoint}/stream`, pipes SSE response body to `w`.

**Fallback behavior:** If worker doesn't support streaming but `POST /api/v1/tasks/stream` is called:
- Dispatch task normally (existing flow)
- Wrap result as a single SSE event: `data: {"chunk": "<full output>", "done": true}`
- Client gets a response, just not streamed

**Reconnection (`GET /api/v1/tasks/{id}/stream`):**
- Task must be in `running` status and worker must be streaming
- If task already `complete`: return final result as single SSE event and close
- If task `failed`: return error SSE event and close

**Rate limiting:** Streaming endpoint uses same `taskLimiter`. One stream counts as one task submission.

---

### Webhooks

**Philosophy:** Event-driven, at-least-once delivery. MagiC's event bus is the source of truth. Webhook deliveries are queued in persistent storage and retried on failure.

**New package:** `core/internal/webhook/`
```
webhook/
├── manager.go    # subscribe to event bus, match webhooks, enqueue deliveries
└── sender.go     # retry loop with exponential backoff, HMAC signing
```

**New protocol types (`protocol/types.go`):**
```go
type Webhook struct {
    ID        string    `json:"id"`
    OrgID     string    `json:"org_id"`
    URL       string    `json:"url"`
    Events    []string  `json:"events"`           // ["task.complete", "worker.register", ...]
    Secret    string    `json:"secret,omitempty"` // HMAC-SHA256 key (write-only: set on create, never returned in GET)
    Active    bool      `json:"active"`
    CreatedAt time.Time `json:"created_at"`
}

type WebhookDelivery struct {
    ID        string     `json:"id"`
    WebhookID string     `json:"webhook_id"`
    EventType string     `json:"event_type"`
    Payload   string     `json:"payload"`          // JSON string
    Status    string     `json:"status"`           // pending|delivered|failed|dead
    Attempts  int        `json:"attempts"`
    NextRetry *time.Time `json:"next_retry,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}
```

**Store interface additions:**
```go
// Webhooks
AddWebhook(w *protocol.Webhook) error
GetWebhook(id string) (*protocol.Webhook, error)
UpdateWebhook(w *protocol.Webhook) error
DeleteWebhook(id string) error
ListWebhooksByOrg(orgID string) []*protocol.Webhook
FindWebhooksByEvent(eventType string) []*protocol.Webhook

// Deliveries
AddWebhookDelivery(d *protocol.WebhookDelivery) error
GetWebhookDelivery(id string) (*protocol.WebhookDelivery, error)
UpdateWebhookDelivery(d *protocol.WebhookDelivery) error
ListPendingWebhookDeliveries() []*protocol.WebhookDelivery
```

**Supported event types:**
```
task.complete    task.fail      task.assign
worker.register  worker.deregister  worker.offline
workflow.complete  workflow.fail
budget.threshold_reached
```

**Delivery flow:**
```
Event Bus → WebhookManager.onEvent()
  → FindWebhooksByEvent(eventType)
  → for each matching webhook:
      → create WebhookDelivery{status: pending}
      → enqueue to sender goroutine

Sender goroutine (runs every 5s):
  → ListPendingWebhookDeliveries()
  → POST to webhook URL with HMAC signature
  → on success: status = delivered
  → on fail: attempts++, NextRetry = now + backoff, status = failed
  → after 5 attempts: status = dead
```

**Retry schedule:** 30s → 5m → 30m → 2h → 8h (exponential, capped at 8h)

**HMAC signature (same pattern as GitHub webhooks):**
```
X-MagiC-Signature: sha256=<hmac-sha256(secret, payload)>   # omitted if no secret configured
X-MagiC-Event: task.complete
X-MagiC-Delivery: <delivery-id>
```

**API endpoints:**
```
POST   /api/v1/orgs/{orgID}/webhooks
GET    /api/v1/orgs/{orgID}/webhooks
DELETE /api/v1/orgs/{orgID}/webhooks/{webhookID}
GET    /api/v1/orgs/{orgID}/webhooks/{webhookID}/deliveries
POST   /api/v1/orgs/{orgID}/webhooks/{webhookID}/test   # send test ping
```

**Wiring in `main.go`:**
```go
wh := webhook.New(s, bus)
wh.Start() // starts event subscription + sender goroutine
```

---

### Prometheus Metrics

**Two endpoints, both kept:**
- `GET /api/v1/metrics` — existing custom JSON stats (unchanged)
- `GET /metrics` — **NEW** Prometheus text format, no auth required (standard Prometheus scraping convention)

**Implementation:** Add `github.com/prometheus/client_golang` dependency. Register metrics using `promauto` in `monitor` package. Monitor already subscribes to all events via event bus — natural place to update counters.

**Full metrics registry:**
```
# Tasks
magic_tasks_total{type, status, worker}              counter
magic_task_duration_seconds{type, worker}            histogram (buckets: .1,.5,1,5,10,30,60)

# Workers
magic_workers_active{org}                            gauge
magic_worker_heartbeat_lag_seconds{worker}           gauge

# Cost
magic_cost_total_usd{org, worker}                   counter

# Workflows
magic_workflow_steps_total{workflow_id, status}      counter
magic_workflows_active                               gauge

# Knowledge Hub
magic_knowledge_queries_total{type}                  counter (keyword|semantic)
magic_knowledge_entries_total                        gauge

# Rate limiting
magic_rate_limit_hits_total{endpoint}               counter

# Webhooks
magic_webhook_deliveries_total{status}              counter (delivered|failed|dead)
magic_webhook_delivery_duration_seconds             histogram

# Streaming
magic_streams_active                                gauge
magic_stream_duration_seconds                       histogram
```

**`/metrics` endpoint:** Added to gateway mux without auth middleware (Prometheus scraper doesn't send Bearer tokens). Served via `promhttp.Handler()`.

**Monitor package changes:** Add `metrics.go` file with all `promauto` registrations. `Monitor.handleEvent()` updates relevant counters/gauges on each event.

---

## Module Wiring Summary

`main.go` updated wiring (additions only):

```go
// Phase 3a
var vs knowledge.VectorStore
if pgStore, ok := s.(*store.PostgreSQLStore); ok {
    vs = store.NewPGVectorStore(pgStore.Pool())
}
kb := knowledge.New(s, bus, vs) // VectorStore injected

// Phase 3b
wh := webhook.New(s, bus)
wh.Start()

gw := gateway.New(gateway.Deps{
    // ... existing ...
    Webhook: wh, // NEW
})
```

---

## File Map

### Phase 3a
```
core/
├── internal/store/
│   ├── postgres.go           # NEW: PostgreSQLStore
│   └── postgres_test.go      # NEW: integration tests
│   ├── pgvector.go           # NEW: PGVectorStore
│   └── pgvector_test.go      # NEW
├── internal/knowledge/
│   ├── vector.go             # NEW: VectorStore interface + SearchResult type
│   └── hub.go                # MODIFY: inject VectorStore, add SearchSemantic()
├── internal/gateway/
│   └── handlers.go           # MODIFY: add /knowledge/{id}/embedding, /knowledge/search/semantic
├── migrations/
│   ├── 001_initial.up.sql    # NEW
│   └── 001_initial.down.sql  # NEW
└── cmd/magic/
    └── main.go               # MODIFY: store backend selection + VectorStore injection
```

### Phase 3b
```
core/
├── internal/webhook/
│   ├── manager.go            # NEW
│   └── sender.go             # NEW
├── internal/monitor/
│   └── metrics.go            # NEW: promauto registrations + counter updates
├── internal/store/
│   ├── store.go              # MODIFY: add Webhook + WebhookDelivery methods
│   ├── memory.go             # MODIFY: implement new methods
│   ├── sqlite.go             # MODIFY: implement new methods
│   └── postgres.go           # MODIFY: implement new methods
├── internal/dispatcher/
│   └── dispatcher.go         # MODIFY: add DispatchStream()
├── internal/protocol/
│   └── types.go              # MODIFY: add Streaming bool to Capability
├── internal/gateway/
│   ├── gateway.go            # MODIFY: add /metrics, /tasks/stream, webhook deps
│   └── handlers.go           # MODIFY: add streaming + webhook handlers
└── cmd/magic/
    └── main.go               # MODIFY: wire webhook.Manager
```

---

## Dependencies to Add

```go
// go.mod additions — pin to latest stable at implementation time
require (
    github.com/jackc/pgx/v5              // PostgreSQL driver + pgxpool
    github.com/golang-migrate/migrate/v4 // Migration system
    github.com/prometheus/client_golang  // Prometheus metrics + promhttp
)
```

pgvector Go client is not needed — raw SQL with `pgx` handles `vector` type as `[]float32`.

---

## Non-Goals (explicitly out of scope)

- WebSocket bidirectional protocol
- Built-in OAuth/JWT or user management
- File upload handling
- Per-tier rate limiting
- Embedding model calls from MagiC core
