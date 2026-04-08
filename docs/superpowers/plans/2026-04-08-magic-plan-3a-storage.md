# MagiC Phase 3a: PostgreSQL + pgvector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PostgreSQL as a production storage backend and semantic vector search to the Knowledge Hub.

**Architecture:** `PostgreSQLStore` implements the existing `Store` interface using JSONB columns (same pattern as SQLiteStore). A separate `VectorStore` interface in the `knowledge` package is injected into `Hub` — keeping vector concerns out of the main Store interface. `PGVectorStore` implements vector search using pgvector extension. Backend is auto-selected from env vars at startup.

**Tech Stack:** Go 1.22+, `github.com/jackc/pgx/v5` + pgxpool, `github.com/golang-migrate/migrate/v4`, PostgreSQL 15+ with pgvector extension, embed.FS for bundled migrations.

**Spec:** `docs/superpowers/specs/2026-04-08-magic-phase3-design.md`

---

## File Map

```
core/
├── go.mod / go.sum                          MODIFY: add pgx, golang-migrate
├── internal/store/
│   ├── migrations/
│   │   ├── 001_initial.up.sql               NEW: all table DDL
│   │   ├── 001_initial.down.sql             NEW: drop all tables
│   │   ├── 002_pgvector.up.sql              NEW: pgvector extension + embeddings table
│   │   └── 002_pgvector.down.sql            NEW: drop embeddings table
│   ├── migrate.go                           NEW: embed FS + RunMigrations()
│   ├── postgres.go                          NEW: PostgreSQLStore (all Store methods)
│   ├── postgres_test.go                     NEW: integration tests (skipped if no MAGIC_POSTGRES_URL)
│   ├── pgvector.go                          NEW: PGVectorStore implementing VectorStore
│   └── pgvector_test.go                     NEW: integration tests
├── internal/knowledge/
│   ├── vector.go                            NEW: VectorStore interface + SearchResult type
│   ├── hub.go                               MODIFY: accept VectorStore, add SemanticSearch()
│   └── hub_test.go                          MODIFY: update New() signature in existing tests
├── internal/gateway/
│   └── handlers.go                          MODIFY: add handleAddEmbedding, handleSemanticSearch
└── cmd/magic/
    └── main.go                              MODIFY: PostgreSQL store selection + VectorStore wiring
```

---

## Chunk 1: Dependencies + Migrations

### Task 1: Add Go dependencies

**Files:**
- Modify: `core/go.mod`

- [ ] **Step 1: Add dependencies**

```bash
cd core
go get github.com/jackc/pgx/v5@latest
go get github.com/jackc/pgx/v5/pgxpool
go get github.com/golang-migrate/migrate/v4@latest
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/iofs
```

- [ ] **Step 2: Verify build succeeds**

```bash
cd core && go build ./...
```
Expected: no errors (new packages downloaded but not yet used).

- [ ] **Step 3: Commit**

```bash
git add core/go.mod core/go.sum
git commit -m "chore(deps): add pgx/v5, golang-migrate for PostgreSQL support"
```

---

### Task 2: Write SQL migrations

**Files:**
- Create: `core/internal/store/migrations/001_initial.up.sql`
- Create: `core/internal/store/migrations/001_initial.down.sql`
- Create: `core/internal/store/migrations/002_pgvector.up.sql`
- Create: `core/internal/store/migrations/002_pgvector.down.sql`
- Create: `core/internal/store/migrate.go`

- [ ] **Step 1: Create migration 001 up**

`core/internal/store/migrations/001_initial.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS workers (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS workflows (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS teams (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS knowledge (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

-- worker_tokens has an extra token_hash column for efficient lookup
-- (TokenHash has json:"-" so it is not in the JSONB blob)
CREATE TABLE IF NOT EXISTS worker_tokens (
    id         TEXT PRIMARY KEY,
    data       JSONB NOT NULL,
    token_hash TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_workers_org      ON workers ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_tasks_org        ON tasks   ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_tasks_status     ON tasks   ((data->>'status'));
CREATE INDEX IF NOT EXISTS idx_audit_org        ON audit_log((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_wh_del_status    ON webhook_deliveries((data->>'status'));
CREATE INDEX IF NOT EXISTS idx_wh_del_next      ON webhook_deliveries((data->>'next_retry'));
CREATE INDEX IF NOT EXISTS idx_worker_tokens_hash ON worker_tokens(token_hash);
```

- [ ] **Step 2: Create migration 001 down**

`core/internal/store/migrations/001_initial.down.sql`:
```sql
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS worker_tokens;
DROP TABLE IF EXISTS knowledge;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS workflows;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS workers;
```

- [ ] **Step 3: Create migration 002 up (pgvector)**

`core/internal/store/migrations/002_pgvector.up.sql`:
```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS knowledge_embeddings (
    id     TEXT PRIMARY KEY,
    vector vector(1536) NOT NULL,
    meta   JSONB
);

CREATE INDEX IF NOT EXISTS idx_knowledge_embeddings_vec
    ON knowledge_embeddings
    USING ivfflat (vector vector_cosine_ops)
    WITH (lists = 100);
```

- [ ] **Step 4: Create migration 002 down**

`core/internal/store/migrations/002_pgvector.down.sql`:
```sql
DROP TABLE IF EXISTS knowledge_embeddings;
DROP EXTENSION IF EXISTS vector;
```

- [ ] **Step 5: Create migrate.go with embedded FS**

`core/internal/store/migrate.go`:
```go
package store

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending up migrations to the given PostgreSQL URL.
// It is idempotent — safe to call on every startup.
func RunMigrations(postgresURL string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("iofs.New: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, postgresURL)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate.Up: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Verify build**

```bash
cd core && go build ./internal/store/...
```
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add core/internal/store/migrations/ core/internal/store/migrate.go
git commit -m "feat(store): add SQL migrations for PostgreSQL (001_initial, 002_pgvector)"
```

---

## Chunk 2: PostgreSQLStore

### Task 3: PostgreSQLStore — core helpers + Worker/Task/Workflow/Team methods

**Files:**
- Create: `core/internal/store/postgres.go`
- Create: `core/internal/store/postgres_test.go`

- [ ] **Step 1: Write failing tests for Worker CRUD**

`core/internal/store/postgres_test.go`:
```go
package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

func newTestPostgresStore(t *testing.T) *store.PostgreSQLStore {
	t.Helper()
	url := os.Getenv("MAGIC_POSTGRES_URL")
	if url == "" {
		t.Skip("MAGIC_POSTGRES_URL not set — skipping PostgreSQL integration tests")
	}
	if err := store.RunMigrations(url); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	s, err := store.NewPostgreSQLStore(context.Background(), url)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPostgreSQLStore_WorkerCRUD(t *testing.T) {
	s := newTestPostgresStore(t)

	w := &protocol.Worker{
		ID:    "w-test-" + time.Now().Format("150405"),
		Name:  "TestWorker",
		OrgID: "org-1",
		Capabilities: []protocol.Capability{
			{Name: "summarize", Description: "Summarize text"},
		},
		Status:        protocol.StatusActive,
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
	}

	if err := s.AddWorker(w); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}

	got, err := s.GetWorker(w.ID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Name != w.Name {
		t.Errorf("Name: got %q, want %q", got.Name, w.Name)
	}

	w.Name = "UpdatedWorker"
	if err := s.UpdateWorker(w); err != nil {
		t.Fatalf("UpdateWorker: %v", err)
	}

	got2, _ := s.GetWorker(w.ID)
	if got2.Name != "UpdatedWorker" {
		t.Errorf("after update: got %q, want UpdatedWorker", got2.Name)
	}

	found := s.FindWorkersByCapability("summarize")
	if len(found) == 0 {
		t.Error("FindWorkersByCapability: expected at least one result")
	}

	byOrg := s.ListWorkersByOrg("org-1")
	if len(byOrg) == 0 {
		t.Error("ListWorkersByOrg: expected at least one result")
	}

	if err := s.RemoveWorker(w.ID); err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}

	if _, err := s.GetWorker(w.ID); err != store.ErrNotFound {
		t.Errorf("after remove: expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd core && MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" go test ./internal/store/... -run TestPostgreSQLStore_WorkerCRUD -v
```
Expected: FAIL — `store.PostgreSQLStore undefined`.

- [ ] **Step 3: Implement PostgreSQLStore core + Worker/Task/Workflow/Team methods**

`core/internal/store/postgres.go`:
```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// PostgreSQLStore is a PostgreSQL-backed implementation of the Store interface.
// All entities are stored as JSONB blobs, matching the SQLiteStore pattern.
type PostgreSQLStore struct {
	pool *pgxpool.Pool
}

// NewPostgreSQLStore creates a new connection pool and pings the database.
// Call RunMigrations before this to ensure schema exists.
func NewPostgreSQLStore(ctx context.Context, connStr string) (*PostgreSQLStore, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &PostgreSQLStore{pool: pool}, nil
}

// Pool returns the underlying connection pool (used by PGVectorStore).
func (s *PostgreSQLStore) Pool() *pgxpool.Pool { return s.pool }

// Close closes the connection pool.
func (s *PostgreSQLStore) Close() { s.pool.Close() }

// --- Generic JSONB helpers ---

func pgPut(pool *pgxpool.Pool, table, id string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = pool.Exec(context.Background(),
		"INSERT INTO "+table+" (id, data) VALUES ($1, $2::jsonb)"+
			" ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data",
		id, data)
	return err
}

func pgGet[T any](pool *pgxpool.Pool, table, id string) (*T, error) {
	var data []byte
	err := pool.QueryRow(context.Background(),
		"SELECT data FROM "+table+" WHERE id = $1", id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func pgDelete(pool *pgxpool.Pool, table, id string) error {
	result, err := pool.Exec(context.Background(),
		"DELETE FROM "+table+" WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func pgList[T any](pool *pgxpool.Pool, query string, args ...any) ([]*T, error) {
	rows, err := pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*T
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		results = append(results, &v)
	}
	return results, nil
}

// --- Workers ---

func (s *PostgreSQLStore) AddWorker(w *protocol.Worker) error {
	return pgPut(s.pool, "workers", w.ID, w)
}

func (s *PostgreSQLStore) GetWorker(id string) (*protocol.Worker, error) {
	return pgGet[protocol.Worker](s.pool, "workers", id)
}

func (s *PostgreSQLStore) UpdateWorker(w *protocol.Worker) error {
	return pgPut(s.pool, "workers", w.ID, w)
}

func (s *PostgreSQLStore) RemoveWorker(id string) error {
	return pgDelete(s.pool, "workers", id)
}

func (s *PostgreSQLStore) ListWorkers() []*protocol.Worker {
	workers, _ := pgList[protocol.Worker](s.pool, "SELECT data FROM workers")
	return workers
}

func (s *PostgreSQLStore) FindWorkersByCapability(capability string) []*protocol.Worker {
	workers, _ := pgList[protocol.Worker](s.pool,
		`SELECT data FROM workers
		 WHERE EXISTS (
		     SELECT 1 FROM jsonb_array_elements(data->'capabilities') AS cap
		     WHERE cap->>'name' = $1
		 )`, capability)
	return workers
}

func (s *PostgreSQLStore) ListWorkersByOrg(orgID string) []*protocol.Worker {
	if orgID == "" {
		return s.ListWorkers()
	}
	workers, _ := pgList[protocol.Worker](s.pool,
		"SELECT data FROM workers WHERE data->>'org_id' = $1", orgID)
	return workers
}

func (s *PostgreSQLStore) FindWorkersByCapabilityAndOrg(capability, orgID string) []*protocol.Worker {
	if orgID == "" {
		return s.FindWorkersByCapability(capability)
	}
	workers, _ := pgList[protocol.Worker](s.pool,
		`SELECT data FROM workers
		 WHERE data->>'org_id' = $1
		 AND EXISTS (
		     SELECT 1 FROM jsonb_array_elements(data->'capabilities') AS cap
		     WHERE cap->>'name' = $2
		 )`, orgID, capability)
	return workers
}

// --- Tasks ---

func (s *PostgreSQLStore) AddTask(t *protocol.Task) error {
	return pgPut(s.pool, "tasks", t.ID, t)
}

func (s *PostgreSQLStore) GetTask(id string) (*protocol.Task, error) {
	return pgGet[protocol.Task](s.pool, "tasks", id)
}

func (s *PostgreSQLStore) UpdateTask(t *protocol.Task) error {
	return pgPut(s.pool, "tasks", t.ID, t)
}

func (s *PostgreSQLStore) ListTasks() []*protocol.Task {
	tasks, _ := pgList[protocol.Task](s.pool, "SELECT data FROM tasks")
	return tasks
}

func (s *PostgreSQLStore) ListTasksByOrg(orgID string) []*protocol.Task {
	if orgID == "" {
		return s.ListTasks()
	}
	tasks, _ := pgList[protocol.Task](s.pool,
		"SELECT data FROM tasks WHERE data->'context'->>'org_id' = $1", orgID)
	return tasks
}

// --- Workflows ---

func (s *PostgreSQLStore) AddWorkflow(w *protocol.Workflow) error {
	return pgPut(s.pool, "workflows", w.ID, w)
}

func (s *PostgreSQLStore) GetWorkflow(id string) (*protocol.Workflow, error) {
	return pgGet[protocol.Workflow](s.pool, "workflows", id)
}

func (s *PostgreSQLStore) UpdateWorkflow(w *protocol.Workflow) error {
	return pgPut(s.pool, "workflows", w.ID, w)
}

func (s *PostgreSQLStore) ListWorkflows() []*protocol.Workflow {
	wfs, _ := pgList[protocol.Workflow](s.pool, "SELECT data FROM workflows")
	return wfs
}

// --- Teams ---

func (s *PostgreSQLStore) AddTeam(t *protocol.Team) error {
	return pgPut(s.pool, "teams", t.ID, t)
}

func (s *PostgreSQLStore) GetTeam(id string) (*protocol.Team, error) {
	return pgGet[protocol.Team](s.pool, "teams", id)
}

func (s *PostgreSQLStore) UpdateTeam(t *protocol.Team) error {
	return pgPut(s.pool, "teams", t.ID, t)
}

func (s *PostgreSQLStore) RemoveTeam(id string) error {
	return pgDelete(s.pool, "teams", id)
}

func (s *PostgreSQLStore) ListTeams() []*protocol.Team {
	teams, _ := pgList[protocol.Team](s.pool, "SELECT data FROM teams")
	return teams
}

// --- Knowledge ---

func (s *PostgreSQLStore) AddKnowledge(k *protocol.KnowledgeEntry) error {
	return pgPut(s.pool, "knowledge", k.ID, k)
}

func (s *PostgreSQLStore) GetKnowledge(id string) (*protocol.KnowledgeEntry, error) {
	return pgGet[protocol.KnowledgeEntry](s.pool, "knowledge", id)
}

func (s *PostgreSQLStore) UpdateKnowledge(k *protocol.KnowledgeEntry) error {
	return pgPut(s.pool, "knowledge", k.ID, k)
}

func (s *PostgreSQLStore) DeleteKnowledge(id string) error {
	return pgDelete(s.pool, "knowledge", id)
}

func (s *PostgreSQLStore) ListKnowledge() []*protocol.KnowledgeEntry {
	entries, _ := pgList[protocol.KnowledgeEntry](s.pool, "SELECT data FROM knowledge")
	return entries
}

func (s *PostgreSQLStore) SearchKnowledge(query string) []*protocol.KnowledgeEntry {
	if query == "" {
		return s.ListKnowledge()
	}
	entries, _ := pgList[protocol.KnowledgeEntry](s.pool,
		"SELECT data FROM knowledge WHERE data->>'title' ILIKE $1 OR data->>'content' ILIKE $1",
		"%"+query+"%")
	return entries
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd core && MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" go test ./internal/store/... -run TestPostgreSQLStore_WorkerCRUD -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/internal/store/postgres.go core/internal/store/postgres_test.go
git commit -m "feat(store): PostgreSQLStore — core helpers + Worker/Task/Workflow/Team/Knowledge"
```

---

### Task 4: PostgreSQLStore — Worker Tokens + Audit + Webhook stubs

**Files:**
- Modify: `core/internal/store/postgres.go` (append)
- Modify: `core/internal/store/postgres_test.go` (append test)

> **Key design note:** `WorkerToken.TokenHash` has `json:"-"` so it is NOT stored in the JSONB blob. The `worker_tokens` table has an extra `token_hash TEXT` column (see migration 001) that stores the hash separately for efficient lookup. The `pgPut` helper does NOT handle this — use a custom insert for tokens.

- [ ] **Step 1: Write failing test for token operations**

Append to `core/internal/store/postgres_test.go`:
```go
func TestPostgreSQLStore_WorkerTokens(t *testing.T) {
	s := newTestPostgresStore(t)

	tok := &protocol.WorkerToken{
		ID:        "tok-" + time.Now().Format("150405.000"),
		OrgID:     "org-1",
		Name:      "test-token",
		CreatedAt: time.Now(),
	}
	tok.TokenHash = "abc123hash"

	if err := s.AddWorkerToken(tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	got, err := s.GetWorkerTokenByHash("abc123hash")
	if err != nil {
		t.Fatalf("GetWorkerTokenByHash: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("ID: got %q, want %q", got.ID, tok.ID)
	}
	if got.TokenHash != "abc123hash" {
		t.Errorf("TokenHash not restored: got %q", got.TokenHash)
	}

	has := s.HasAnyWorkerTokens()
	if !has {
		t.Error("HasAnyWorkerTokens: expected true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd core && MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" go test ./internal/store/... -run TestPostgreSQLStore_WorkerTokens -v
```
Expected: FAIL — methods not yet implemented (or missing from interface).

- [ ] **Step 3: Append token + audit + webhook stub methods to postgres.go**

Append to `core/internal/store/postgres.go`:
```go
// --- Worker Tokens ---
// Note: token_hash is stored in a dedicated column (not in JSONB) because
// WorkerToken.TokenHash has json:"-". Custom insert/update required.

func (s *PostgreSQLStore) AddWorkerToken(t *protocol.WorkerToken) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO worker_tokens (id, data, token_hash)
		 VALUES ($1, $2::jsonb, $3)
		 ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, token_hash = EXCLUDED.token_hash`,
		t.ID, data, t.TokenHash)
	return err
}

func (s *PostgreSQLStore) GetWorkerToken(id string) (*protocol.WorkerToken, error) {
	return pgGetToken(s.pool, "id = $1", id)
}

func (s *PostgreSQLStore) GetWorkerTokenByHash(hash string) (*protocol.WorkerToken, error) {
	return pgGetToken(s.pool, "token_hash = $1", hash)
}

// pgGetToken retrieves a WorkerToken and restores its TokenHash from the dedicated column.
func pgGetToken(pool *pgxpool.Pool, where string, arg any) (*protocol.WorkerToken, error) {
	var data []byte
	var hash string
	err := pool.QueryRow(context.Background(),
		"SELECT data, token_hash FROM worker_tokens WHERE "+where, arg).Scan(&data, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t protocol.WorkerToken
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	t.TokenHash = hash // restore from dedicated column
	return &t, nil
}

func (s *PostgreSQLStore) UpdateWorkerToken(t *protocol.WorkerToken) error {
	// CAS: reject if token is already bound to a different worker.
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var existingData []byte
	err = tx.QueryRow(ctx,
		"SELECT data FROM worker_tokens WHERE id = $1", t.ID).Scan(&existingData)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var existing protocol.WorkerToken
	if err := json.Unmarshal(existingData, &existing); err != nil {
		return err
	}
	if existing.WorkerID != "" && t.WorkerID != existing.WorkerID {
		return ErrTokenAlreadyBound
	}

	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		"UPDATE worker_tokens SET data = $2::jsonb, token_hash = $3 WHERE id = $1",
		t.ID, data, t.TokenHash)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgreSQLStore) ListWorkerTokensByOrg(orgID string) []*protocol.WorkerToken {
	rows, err := s.pool.Query(context.Background(),
		"SELECT data, token_hash FROM worker_tokens WHERE data->>'org_id' = $1", orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanTokenRows(rows)
}

func (s *PostgreSQLStore) ListWorkerTokensByWorker(workerID string) []*protocol.WorkerToken {
	rows, err := s.pool.Query(context.Background(),
		"SELECT data, token_hash FROM worker_tokens WHERE data->>'worker_id' = $1", workerID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanTokenRows(rows)
}

func scanTokenRows(rows pgx.Rows) []*protocol.WorkerToken {
	var result []*protocol.WorkerToken
	for rows.Next() {
		var data []byte
		var hash string
		if err := rows.Scan(&data, &hash); err != nil {
			continue
		}
		var t protocol.WorkerToken
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		t.TokenHash = hash
		result = append(result, &t)
	}
	return result
}

func (s *PostgreSQLStore) HasAnyWorkerTokens() bool {
	var count int
	s.pool.QueryRow(context.Background(), //nolint:errcheck
		"SELECT COUNT(*) FROM worker_tokens LIMIT 1").Scan(&count)
	return count > 0
}

// --- Audit Log ---

func (s *PostgreSQLStore) AppendAudit(e *protocol.AuditEntry) error {
	return pgPut(s.pool, "audit_log", e.ID, e)
}

func (s *PostgreSQLStore) QueryAudit(filter AuditFilter) []*protocol.AuditEntry {
	// Filter in Go (same approach as SQLiteStore) — audit log is append-only,
	// queries are infrequent and result sets are small (capped at filter.Limit).
	all, _ := pgList[protocol.AuditEntry](s.pool, "SELECT data FROM audit_log ORDER BY id DESC")
	var result []*protocol.AuditEntry
	for _, e := range all {
		if filter.OrgID != "" && e.OrgID != filter.OrgID {
			continue
		}
		if filter.WorkerID != "" && e.WorkerID != filter.WorkerID {
			continue
		}
		if filter.Action != "" && e.Action != filter.Action {
			continue
		}
		if filter.StartTime != nil && e.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && e.Timestamp.After(*filter.EndTime) {
			continue
		}
		result = append(result, e)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset >= len(result) {
		return nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// --- Webhooks (stubs used by Phase 3b — implemented fully there) ---
// These must exist now so PostgreSQLStore satisfies the Store interface
// once Phase 3b adds Webhook methods to it.
// Phase 3b will append the full implementations to this file.
```

- [ ] **Step 4: Run tests**

```bash
cd core && MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" go test ./internal/store/... -run TestPostgreSQLStore -v
```
Expected: PASS for all PostgreSQL tests.

- [ ] **Step 5: Verify full build (the Store interface will be satisfied once Phase 3b adds Webhook methods — skip compile check until then)**

```bash
cd core && go build ./internal/store/...
```

- [ ] **Step 6: Commit**

```bash
git add core/internal/store/postgres.go core/internal/store/postgres_test.go
git commit -m "feat(store): PostgreSQLStore — tokens, audit, org-scoped queries"
```

---

### Task 5: Update main.go for PostgreSQL backend selection

**Files:**
- Modify: `core/cmd/magic/main.go`

- [ ] **Step 1: Replace store selection block in runServer()**

Find and replace the store selection block in `main.go` (currently lines that check `MAGIC_STORE` env var):

```go
// Store — auto-detect backend from env vars
var s store.Store
switch {
case os.Getenv("MAGIC_POSTGRES_URL") != "":
    pgURL := os.Getenv("MAGIC_POSTGRES_URL")
    if err := store.RunMigrations(pgURL); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to run migrations: %v\n", err)
        os.Exit(1)
    }
    pgStore, err := store.NewPostgreSQLStore(context.Background(), pgURL)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to connect to PostgreSQL: %v\n", err)
        os.Exit(1)
    }
    s = pgStore
    fmt.Println("  Storage: PostgreSQL")
case os.Getenv("MAGIC_STORE") != "":
    sqliteStore, err := store.NewSQLiteStore(os.Getenv("MAGIC_STORE"))
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to open store: %v\n", err)
        os.Exit(1)
    }
    s = sqliteStore
    fmt.Printf("  Storage: SQLite (%s)\n", os.Getenv("MAGIC_STORE"))
default:
    s = store.NewMemoryStore()
    fmt.Println("  Storage: in-memory (set MAGIC_STORE=path.db or MAGIC_POSTGRES_URL for persistence)")
}
```

Also add `"context"` to imports if not already present.

- [ ] **Step 2: Build and verify**

```bash
cd core && go build ./cmd/magic/
```
Expected: binary compiles.

- [ ] **Step 3: Smoke test with PostgreSQL**

```bash
MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" ./magic serve &
sleep 1 && curl -s http://localhost:8080/health | grep '"status":"ok"'
kill %1
```
Expected: `{"status":"ok",...}` printed.

- [ ] **Step 4: Commit**

```bash
git add core/cmd/magic/main.go
git commit -m "feat(main): auto-select PostgreSQL/SQLite/Memory backend from env vars"
```

---

## Chunk 3: pgvector + Semantic Search

### Task 6: VectorStore interface + Hub update

**Files:**
- Create: `core/internal/knowledge/vector.go`
- Modify: `core/internal/knowledge/hub.go`
- Modify: `core/internal/knowledge/hub_test.go`

- [ ] **Step 1: Write failing test for Hub with VectorStore**

Add to `core/internal/knowledge/hub_test.go`:
```go
// mockVectorStore is a test double for VectorStore.
type mockVectorStore struct {
    upserted map[string][]float32
    results  []SearchResult
}

func (m *mockVectorStore) Upsert(id string, vector []float32, meta map[string]any) error {
    if m.upserted == nil {
        m.upserted = make(map[string][]float32)
    }
    m.upserted[id] = vector
    return nil
}

func (m *mockVectorStore) Search(queryVector []float32, topK int) ([]SearchResult, error) {
    return m.results, nil
}

func (m *mockVectorStore) Delete(id string) error {
    delete(m.upserted, id)
    return nil
}

func TestHub_SemanticSearch(t *testing.T) {
    ms := store.NewMemoryStore()
    bus := events.NewBus()
    vs := &mockVectorStore{
        results: []SearchResult{{ID: "kb-1", Score: 0.95, Metadata: nil}},
    }
    h := New(ms, bus, vs)

    results, err := h.SemanticSearch([]float32{0.1, 0.2, 0.3}, 5)
    if err != nil {
        t.Fatalf("SemanticSearch: %v", err)
    }
    if len(results) != 1 || results[0].ID != "kb-1" {
        t.Errorf("unexpected results: %v", results)
    }
}

func TestHub_SemanticSearch_NoVectorStore(t *testing.T) {
    ms := store.NewMemoryStore()
    bus := events.NewBus()
    h := New(ms, bus, nil) // no vector store

    _, err := h.SemanticSearch([]float32{0.1}, 5)
    if err == nil {
        t.Error("expected error when VectorStore is nil")
    }
}
```

> Note: the `SearchResult` type and updated `New()` signature are defined in Step 3 below. Run test after Step 3.

- [ ] **Step 2: Create VectorStore interface**

`core/internal/knowledge/vector.go`:
```go
package knowledge

// VectorStore defines the interface for vector similarity search.
// It is separate from store.Store to keep vector concerns out of the main storage interface.
// PGVectorStore in the store package implements this interface.
// When no VectorStore is configured (nil), semantic search is unavailable.
type VectorStore interface {
    // Upsert stores or replaces an embedding for a knowledge entry ID.
    Upsert(id string, vector []float32, meta map[string]any) error
    // Search returns the top-K most similar entries to queryVector by cosine similarity.
    Search(queryVector []float32, topK int) ([]SearchResult, error)
    // Delete removes the embedding for a knowledge entry ID.
    Delete(id string) error
}

// SearchResult is returned by VectorStore.Search.
type SearchResult struct {
    ID       string         `json:"id"`
    Score    float32        `json:"score"`    // 0.0–1.0 cosine similarity
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

- [ ] **Step 3: Update Hub to accept VectorStore**

Modify `core/internal/knowledge/hub.go` — update the struct and constructor:

```go
// Replace the Hub struct definition and New() function:

type Hub struct {
    store   store.Store
    bus     *events.Bus
    vectors VectorStore // nil if semantic search is not configured
}

func New(s store.Store, bus *events.Bus, vs VectorStore) *Hub {
    return &Hub{store: s, bus: bus, vectors: vs}
}

// Add SemanticSearch method at the end of the file:

// SemanticSearch returns knowledge entries ranked by cosine similarity to queryVector.
// Returns an error if no VectorStore is configured.
func (h *Hub) SemanticSearch(queryVector []float32, topK int) ([]SearchResult, error) {
    if h.vectors == nil {
        return nil, fmt.Errorf("semantic search requires PostgreSQL backend with pgvector")
    }
    return h.vectors.Search(queryVector, topK)
}

// AddEmbedding stores a vector embedding for an existing knowledge entry.
func (h *Hub) AddEmbedding(id string, vector []float32, meta map[string]any) error {
    if h.vectors == nil {
        return fmt.Errorf("semantic search requires PostgreSQL backend with pgvector")
    }
    return h.vectors.Upsert(id, vector, meta)
}
```

Add `"fmt"` to imports in hub.go.

- [ ] **Step 4: Fix compilation — all callers of knowledge.New() must be updated**

Search for all calls to `knowledge.New(` in the codebase:
```bash
cd core && grep -rn "knowledge.New(" .
```
Update each call site to pass `nil` as the third argument (VectorStore will be set in main.go in Task 9):
```go
kb := knowledge.New(s, bus, nil) // VectorStore injected later in main.go
```

- [ ] **Step 5: Run the hub tests**

```bash
cd core && go test ./internal/knowledge/... -v
```
Expected: PASS (including new SemanticSearch tests).

- [ ] **Step 6: Commit**

```bash
git add core/internal/knowledge/vector.go core/internal/knowledge/hub.go core/internal/knowledge/hub_test.go
git commit -m "feat(knowledge): VectorStore interface + Hub.SemanticSearch / AddEmbedding"
```

---

### Task 7: PGVectorStore implementation

**Files:**
- Create: `core/internal/store/pgvector.go`
- Create: `core/internal/store/pgvector_test.go`

- [ ] **Step 1: Write failing test**

`core/internal/store/pgvector_test.go`:
```go
package store_test

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/kienbui1995/magic/core/internal/store"
)

func TestPGVectorStore_UpsertAndSearch(t *testing.T) {
	url := os.Getenv("MAGIC_POSTGRES_URL")
	if url == "" {
		t.Skip("MAGIC_POSTGRES_URL not set — skipping pgvector integration tests")
	}
	if err := store.RunMigrations(url); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	pgStore, err := store.NewPostgreSQLStore(context.Background(), url)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	vs := store.NewPGVectorStore(pgStore.Pool(), 3) // dim=3 for test

	// Upsert two vectors
	v1 := []float32{1, 0, 0}
	v2 := []float32{0, 1, 0}
	if err := vs.Upsert("e-1", v1, map[string]any{"label": "x-axis"}); err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}
	if err := vs.Upsert("e-2", v2, map[string]any{"label": "y-axis"}); err != nil {
		t.Fatalf("Upsert v2: %v", err)
	}

	// Search with query closest to v1
	query := []float32{0.9, 0.1, 0}
	results, err := vs.Search(query, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}
	if results[0].ID != "e-1" {
		t.Errorf("expected e-1 as top result, got %s", results[0].ID)
	}
	if results[0].Score < 0.9 {
		t.Errorf("expected high similarity score, got %f", results[0].Score)
	}
	_ = math.Pi // avoid unused import

	// Delete
	if err := vs.Delete("e-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd core && MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" go test ./internal/store/... -run TestPGVectorStore -v
```
Expected: FAIL — `store.PGVectorStore` undefined.

- [ ] **Step 3: Implement PGVectorStore**

`core/internal/store/pgvector.go`:
```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kienbui1995/magic/core/internal/knowledge"
)

// PGVectorStore implements knowledge.VectorStore using PostgreSQL pgvector extension.
// The table schema is created by migration 002_pgvector.up.sql.
type PGVectorStore struct {
	pool *pgxpool.Pool
	dim  int // embedding dimension (default 1536)
}

// NewPGVectorStore creates a new PGVectorStore.
// dim must match the dimension of embeddings being stored (e.g., 1536 for text-embedding-3-small).
// The dimension is fixed at table creation time (see migration 002).
func NewPGVectorStore(pool *pgxpool.Pool, dim int) *PGVectorStore {
	if dim <= 0 {
		dim = 1536
	}
	return &PGVectorStore{pool: pool, dim: dim}
}

// Upsert stores or replaces an embedding for a knowledge entry ID.
func (s *PGVectorStore) Upsert(id string, vector []float32, meta map[string]any) error {
	if len(vector) != s.dim {
		return fmt.Errorf("vector dimension mismatch: got %d, want %d", len(vector), s.dim)
	}
	vecStr := encodeVector(vector)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO knowledge_embeddings (id, vector, meta)
		 VALUES ($1, $2::vector, $3::jsonb)
		 ON CONFLICT (id) DO UPDATE
		     SET vector = EXCLUDED.vector, meta = EXCLUDED.meta`,
		id, vecStr, metaJSON)
	return err
}

// Search returns the top-K most similar entries by cosine similarity.
// Score is 1 - cosine_distance, ranging from 0.0 (orthogonal) to 1.0 (identical).
func (s *PGVectorStore) Search(queryVector []float32, topK int) ([]knowledge.SearchResult, error) {
	if len(queryVector) != s.dim {
		return nil, fmt.Errorf("query vector dimension mismatch: got %d, want %d", len(queryVector), s.dim)
	}
	if topK <= 0 {
		topK = 10
	}
	vecStr := encodeVector(queryVector)
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, meta, 1 - (vector <=> $1::vector) AS score
		 FROM knowledge_embeddings
		 ORDER BY vector <=> $1::vector
		 LIMIT $2`,
		vecStr, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []knowledge.SearchResult
	for rows.Next() {
		var id string
		var metaJSON []byte
		var score float32
		if err := rows.Scan(&id, &metaJSON, &score); err != nil {
			continue
		}
		var meta map[string]any
		_ = json.Unmarshal(metaJSON, &meta)
		results = append(results, knowledge.SearchResult{ID: id, Score: score, Metadata: meta})
	}
	return results, nil
}

// Delete removes the embedding for a knowledge entry ID.
func (s *PGVectorStore) Delete(id string) error {
	result, err := s.pool.Exec(context.Background(),
		"DELETE FROM knowledge_embeddings WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// encodeVector formats a float32 slice as a pgvector literal: "[x,y,z,...]"
func encodeVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Ensure PGVectorStore satisfies the knowledge.VectorStore interface at compile time.
var _ knowledge.VectorStore = (*PGVectorStore)(nil)

// Unused import guard
var _ = errors.New
var _ = pgx.ErrNoRows
```

- [ ] **Step 4: Run tests**

```bash
cd core && MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" go test ./internal/store/... -run TestPGVectorStore -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/internal/store/pgvector.go core/internal/store/pgvector_test.go
git commit -m "feat(store): PGVectorStore — semantic search via pgvector extension"
```

---

### Task 8: Semantic search Gateway handlers + main.go wiring

**Files:**
- Modify: `core/internal/gateway/gateway.go`
- Modify: `core/internal/gateway/handlers.go`
- Modify: `core/cmd/magic/main.go`

- [ ] **Step 1: Add Knowledge dep to Gateway Deps + routes**

In `core/internal/gateway/gateway.go`, `Knowledge *knowledge.Hub` already exists in `Deps`. Add two routes inside `Handler()`:

```go
// After existing knowledge routes:
mux.HandleFunc("POST /api/v1/knowledge/{id}/embedding", g.handleAddEmbedding)
mux.HandleFunc("POST /api/v1/knowledge/search/semantic", g.handleSemanticSearch)
```

- [ ] **Step 2: Write handler tests**

Add to `core/internal/gateway/gateway_test.go`:
```go
func TestHandleSemanticSearch_NoVectorStore(t *testing.T) {
    // Hub created with nil VectorStore → expect 501
    gw, _ := newTestGateway(t) // existing test helper
    body := `{"query_vector":[0.1,0.2],"top_k":3}`
    req := httptest.NewRequest("POST", "/api/v1/knowledge/search/semantic",
        strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    gw.Handler().ServeHTTP(w, req)
    if w.Code != http.StatusNotImplemented {
        t.Errorf("expected 501, got %d: %s", w.Code, w.Body.String())
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd core && go test ./internal/gateway/... -run TestHandleSemanticSearch -v
```
Expected: FAIL — handler not yet defined.

- [ ] **Step 4: Implement handlers**

Append to `core/internal/gateway/handlers.go`:
```go
func (g *Gateway) handleAddEmbedding(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    var req struct {
        Vector   []float32      `json:"vector"`
        Metadata map[string]any `json:"metadata"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if len(req.Vector) == 0 {
        writeError(w, http.StatusBadRequest, "vector is required")
        return
    }
    if err := g.deps.Knowledge.AddEmbedding(id, req.Vector, req.Metadata); err != nil {
        if strings.Contains(err.Error(), "pgvector") {
            writeError(w, http.StatusNotImplemented, err.Error())
            return
        }
        writeError(w, http.StatusInternalServerError, "failed to store embedding")
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
    var req struct {
        QueryVector []float32 `json:"query_vector"`
        TopK        int       `json:"top_k"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if len(req.QueryVector) == 0 {
        writeError(w, http.StatusBadRequest, "query_vector is required")
        return
    }
    if req.TopK <= 0 {
        req.TopK = 10
    }
    results, err := g.deps.Knowledge.SemanticSearch(req.QueryVector, req.TopK)
    if err != nil {
        if strings.Contains(err.Error(), "pgvector") {
            writeError(w, http.StatusNotImplemented, err.Error())
            return
        }
        writeError(w, http.StatusInternalServerError, "semantic search failed")
        return
    }
    writeJSON(w, http.StatusOK, results)
}
```

- [ ] **Step 5: Update main.go to inject PGVectorStore into Hub**

In `core/cmd/magic/main.go`, after store creation, add VectorStore injection:

```go
// VectorStore — only available with PostgreSQL backend
var vs knowledge.VectorStore
if pgStore, ok := s.(*store.PostgreSQLStore); ok {
    dim := 1536
    if d := os.Getenv("MAGIC_PGVECTOR_DIM"); d != "" {
        if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
            dim = parsed
        }
    }
    vs = store.NewPGVectorStore(pgStore.Pool(), dim)
    fmt.Println("  Semantic search: enabled (pgvector)")
}

// Then pass vs to knowledge.New:
kb := knowledge.New(s, bus, vs)
```

Add imports: `"strconv"`, `"github.com/kienbui1995/magic/core/internal/knowledge"`.

- [ ] **Step 6: Build and run all tests**

```bash
cd core && go build ./... && go test ./...
```
Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add core/internal/gateway/gateway.go core/internal/gateway/handlers.go core/cmd/magic/main.go
git commit -m "feat(gateway): semantic search endpoints + PGVectorStore wiring in main.go"
```

---

## Self-Review

After writing this plan, reviewing against spec `docs/superpowers/specs/2026-04-08-magic-phase3-design.md`:

- ✅ PostgreSQL backend with JSONB — Task 3-4
- ✅ Auto-detect from MAGIC_POSTGRES_URL — Task 5
- ✅ golang-migrate with embedded SQL — Task 2
- ✅ token_hash dedicated column (special case vs JSONB pattern) — Task 4
- ✅ VectorStore interface in knowledge package (not Store) — Task 6
- ✅ PGVectorStore with cosine similarity — Task 7
- ✅ Workers submit pre-computed embeddings — Task 8 (POST /knowledge/{id}/embedding)
- ✅ 501 Not Implemented when vectors == nil — Task 8
- ✅ MAGIC_PGVECTOR_DIM config — Task 8 (main.go)
- ✅ pgxpool connection pooling — Task 3
- ⚠️ MAGIC_POSTGRES_POOL_MIN/MAX config not wired in main.go — add to Task 5:
  ```go
  config, _ := pgxpool.ParseConfig(pgURL)
  if min := os.Getenv("MAGIC_POSTGRES_POOL_MIN"); min != "" {
      if v, err := strconv.Atoi(min); err == nil {
          config.MinConns = int32(v)
      }
  }
  if max := os.Getenv("MAGIC_POSTGRES_POOL_MAX"); max != "" {
      if v, err := strconv.Atoi(max); err == nil {
          config.MaxConns = int32(v)
      }
  }
  pool, err := pgxpool.NewWithConfig(context.Background(), config)
  // Use pool directly instead of pgxpool.New(ctx, connStr)
  ```
  Update `NewPostgreSQLStore` to accept `*pgxpool.Config` OR add a separate `NewPostgreSQLStoreWithConfig` constructor. Simplest: update Task 5 to parse pool config in main.go and pass URL (pgxpool supports pool settings via connection string: `?pool_min_conns=2&pool_max_conns=20`).

Fix: In Task 5 main.go, append pool settings to connStr:
```go
pgURL := os.Getenv("MAGIC_POSTGRES_URL")
if min := os.Getenv("MAGIC_POSTGRES_POOL_MIN"); min != "" {
    pgURL += "&pool_min_conns=" + min
}
if max := os.Getenv("MAGIC_POSTGRES_POOL_MAX"); max != "" {
    pgURL += "&pool_max_conns=" + max
}
```
Note: This only works if MAGIC_POSTGRES_URL doesn't already have a `?` at the end without params. Use `url.ParseQuery` to merge properly, or document that pool settings can be included in the URL directly.
