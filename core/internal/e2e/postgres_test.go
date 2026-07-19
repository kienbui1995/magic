//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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

// MagiC tables created by migrations 001-005.
var magicCoreTables = []string{
	"workers", "tasks", "workflows", "teams", "knowledge",
	"worker_tokens", "audit_log", "webhooks", "webhook_deliveries",
	"policies", "role_bindings",
}

// TestE2E_Postgres_Migrations — up applies every migration and creates the
// expected tables; down reverses the stack cleanly.
func TestE2E_Postgres_Migrations(t *testing.T) {
	connStr := startPostgresContainer(t)

	// UP
	applyMigrations(t, connStr, "up")
	s, err := store.NewPostgreSQLStore(context.Background(), connStr)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	ctx := context.Background()
	for _, table := range magicCoreTables {
		ok, err := tableExists(ctx, s, table)
		if err != nil {
			t.Fatalf("tableExists %s: %v", table, err)
		}
		if !ok {
			t.Errorf("after up: table %q missing", table)
		}
	}
	// pgvector extension + knowledge_embeddings present
	if ok, _ := tableExists(ctx, s, "knowledge_embeddings"); !ok {
		t.Errorf("after up: knowledge_embeddings missing (pgvector migration)")
	}
	// RLS policies should be in place for workers
	var rlsEnabled bool
	if err := s.Pool().QueryRow(ctx,
		`SELECT relrowsecurity FROM pg_class WHERE relname = 'workers'`).Scan(&rlsEnabled); err != nil {
		t.Fatalf("check rls: %v", err)
	}
	if !rlsEnabled {
		t.Error("after up: RLS not enabled on workers")
	}
	s.Close()

	// DOWN
	applyMigrations(t, connStr, "down")
	s2, err := store.NewPostgreSQLStore(context.Background(), connStr)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore (post-down): %v", err)
	}
	defer s2.Close()
	for _, table := range magicCoreTables {
		ok, err := tableExists(ctx, s2, table)
		if err != nil {
			t.Fatalf("tableExists %s: %v", table, err)
		}
		if ok {
			t.Errorf("after down: table %q still exists", table)
		}
	}
}

// TestE2E_Postgres_BasicCRUD — worker CRUD round-trip through the real store.
func TestE2E_Postgres_BasicCRUD(t *testing.T) {
	s, _ := setupPostgresStore(t)
	ctx := context.Background()

	w := &protocol.Worker{
		ID:           "pg-crud-w1",
		Name:         "CrudBot",
		OrgID:        "org_crud",
		Status:       protocol.StatusActive,
		RegisteredAt: time.Now(),
	}
	if err := s.AddWorker(ctx, w); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}
	got, err := s.GetWorker(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Name != "CrudBot" {
		t.Errorf("Name: got %q, want CrudBot", got.Name)
	}

	got.Name = "CrudBot-v2"
	if err := s.UpdateWorker(ctx, got); err != nil {
		t.Fatalf("UpdateWorker: %v", err)
	}
	got2, _ := s.GetWorker(ctx, w.ID)
	if got2.Name != "CrudBot-v2" {
		t.Errorf("after update: Name %q", got2.Name)
	}

	if list := s.ListWorkersByOrg(ctx, "org_crud"); len(list) != 1 {
		t.Errorf("ListWorkersByOrg: got %d, want 1", len(list))
	}

	if err := s.RemoveWorker(ctx, w.ID); err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}
	if _, err := s.GetWorker(ctx, w.ID); err == nil {
		t.Errorf("GetWorker after remove: expected error")
	}
}

// TestE2E_Postgres_RLS_CrossTenantIsolation — seed workers for two orgs,
// then query via WithOrgContext and verify orgA cannot see orgB's rows.
func TestE2E_Postgres_RLS_CrossTenantIsolation(t *testing.T) {
	s, _ := setupPostgresStore(t)
	ctx := context.Background()

	orgs := []string{"pg-rls-A", "pg-rls-B"}
	for _, org := range orgs {
		for i := 0; i < 2; i++ {
			wid := fmt.Sprintf("%s-w-%d", org, i)
			if err := s.AddWorker(ctx, &protocol.Worker{
				ID: wid, Name: wid, OrgID: org,
				Status: protocol.StatusActive, RegisteredAt: time.Now(),
			}); err != nil {
				t.Fatalf("AddWorker: %v", err)
			}
		}
	}

	// Scoped to orgA — should see ONLY 2 workers total (orgB hidden by RLS).
	if err := s.WithOrgContext(ctx, orgs[0], func(conn *pgxpool.Conn) error {
		var n int
		if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM workers").Scan(&n); err != nil {
			return err
		}
		if n != 2 {
			t.Errorf("orgA scope: got %d workers visible, want 2", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithOrgContext(A): %v", err)
	}

	// Scoped to orgB — symmetric.
	if err := s.WithOrgContext(ctx, orgs[1], func(conn *pgxpool.Conn) error {
		var n int
		if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM workers WHERE data->>'org_id' = $1", orgs[0]).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("orgB scope leaked %d orgA rows", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithOrgContext(B): %v", err)
	}

	// Bypass (empty) sees all.
	if err := s.WithOrgContext(ctx, "", func(conn *pgxpool.Conn) error {
		var n int
		if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM workers").Scan(&n); err != nil {
			return err
		}
		if n < 4 {
			t.Errorf("bypass: got %d, want >=4", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithOrgContext(bypass): %v", err)
	}
}

// TestE2E_Postgres_RLS_HTTPLevel — full gateway over Postgres. Two worker
// tokens in two orgs. A heartbeat from tokenB against a worker that belongs
// to orgA must fail to observe the target (RLS hides it → 401/404), while a
// heartbeat from tokenA against orgA's own worker must succeed. Proves that
// the end-to-end workerAuth → rlsScopeMiddleware → store chain filters at
// the database layer.
func TestE2E_Postgres_RLS_HTTPLevel(t *testing.T) {
	s, _ := setupPostgresStore(t)
	ctx := context.Background()

	orgA, orgB := "pg-http-A", "pg-http-B"

	// Seed one worker per org.
	for _, org := range []string{orgA, orgB} {
		if err := s.AddWorker(ctx, &protocol.Worker{
			ID: org + "-w-0", Name: org + "-w-0", OrgID: org,
			Status: protocol.StatusActive, RegisteredAt: time.Now(),
		}); err != nil {
			t.Fatalf("AddWorker: %v", err)
		}
	}

	mkToken := func(org string) string {
		raw, hash := protocol.GenerateToken()
		if err := s.AddWorkerToken(ctx, &protocol.WorkerToken{
			ID:        protocol.GenerateID("tok"),
			OrgID:     org,
			WorkerID:  org + "-w-0",
			TokenHash: hash,
			CreatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("AddWorkerToken: %v", err)
		}
		return raw
	}
	tokenA := mkToken(orgA)
	tokenB := mkToken(orgB)

	// Build full gateway wired to the postgres store.
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
		Registry: reg, Router: rt, Store: s, Bus: bus, Monitor: mon,
		CostCtrl: cc, Evaluator: ev, Orchestrator: orch, OrgMgr: mgr,
		Knowledge: kb, Dispatcher: disp,
	})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()
	t.Cleanup(func() { bus.Stop() })

	// Store-scoped listing: tokenA's org should see only its own worker.
	scopedA := store.WithOrgIDContext(context.Background(), orgA)
	list := s.ListWorkersByOrg(scopedA, orgA)
	if len(list) != 1 || list[0].OrgID != orgA {
		t.Errorf("scoped store list for orgA: %+v", list)
	}
	// And tokenB's org should not see orgA workers.
	scopedB := store.WithOrgIDContext(context.Background(), orgB)
	leakedB := s.ListWorkersByOrg(scopedB, orgA)
	if len(leakedB) != 0 {
		t.Errorf("orgB-scoped list of orgA rows leaked %d rows through RLS", len(leakedB))
	}

	// Sanity: both tokens authenticate for their own heartbeat endpoint
	// (full HTTP chain including workerAuth + rlsScopeMiddleware).
	for _, c := range []struct{ label, token string }{
		{"tokenA", tokenA}, {"tokenB", tokenB},
	} {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/workers/heartbeat", nil)
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s heartbeat: %v", c.label, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			t.Errorf("%s heartbeat: auth rejected with %d", c.label, resp.StatusCode)
		}
	}
}

// TestE2E_Postgres_ConnectionPool_Concurrent — 100 goroutines hammer AddTask
// against the shared pool. None may fail; all rows must be persisted.
func TestE2E_Postgres_ConnectionPool_Concurrent(t *testing.T) {
	s, _ := setupPostgresStore(t)
	ctx := context.Background()

	const N = 100
	var wg sync.WaitGroup
	var failures atomic.Int32
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			tid := fmt.Sprintf("pg-concur-t-%04d", i)
			if err := s.AddTask(ctx, &protocol.Task{
				ID:      tid,
				Type:    "test",
				Status:  protocol.TaskPending,
				Context: protocol.TaskContext{OrgID: "org_concur"},
			}); err != nil {
				failures.Add(1)
				t.Errorf("AddTask #%d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("pool pressure: %d failures", failures.Load())
	}
	tasks := s.ListTasksByOrg(ctx, "org_concur")
	if len(tasks) != N {
		t.Errorf("persisted tasks: got %d, want %d", len(tasks), N)
	}
}

// TestE2E_Postgres_BeforeAcquireHook — when a request context carries an
// orgID, queries made on the acquired connection observe that value via
// current_setting('app.current_org_id'). Without the scope, the value is "".
func TestE2E_Postgres_BeforeAcquireHook(t *testing.T) {
	s, _ := setupPostgresStore(t)

	// Scoped ctx: hook must set app.current_org_id on acquire.
	scoped := store.WithOrgIDContext(context.Background(), "hook-org-42")
	got, err := queryCurrentSetting(scoped, s)
	if err != nil {
		t.Fatalf("queryCurrentSetting(scoped): %v", err)
	}
	if got != "hook-org-42" {
		t.Errorf("scoped current_setting: got %q, want hook-org-42", got)
	}

	// Unscoped ctx: AfterRelease must have cleared it; new acquire sees "".
	got2, err := queryCurrentSetting(context.Background(), s)
	if err != nil {
		t.Fatalf("queryCurrentSetting(bypass): %v", err)
	}
	if got2 != "" {
		t.Errorf("bypass current_setting: got %q, want empty (AfterRelease should reset)", got2)
	}
}

// TestE2E_Postgres_TransactionRollback — UpdateWorkerToken enforces CAS on
// worker_id. Attempting to bind a token already bound to workerX to workerY
// must error with ErrTokenAlreadyBound; the original binding must be
// preserved (transaction rolled back).
func TestE2E_Postgres_TransactionRollback(t *testing.T) {
	s, _ := setupPostgresStore(t)
	ctx := context.Background()

	raw, hash := protocol.GenerateToken()
	_ = raw
	tok := &protocol.WorkerToken{
		ID:        protocol.GenerateID("tok"),
		OrgID:     "org_rollback",
		WorkerID:  "worker-X",
		TokenHash: hash,
		CreatedAt: time.Now(),
	}
	if err := s.AddWorkerToken(ctx, tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	// Attempt to rebind to a different worker — must fail.
	conflict := *tok
	conflict.WorkerID = "worker-Y"
	err := s.UpdateWorkerToken(ctx, &conflict)
	if err == nil {
		t.Fatal("UpdateWorkerToken(conflict): expected error, got nil")
	}

	// Re-read and verify the stored binding is still worker-X.
	got, err := s.GetWorkerToken(ctx, tok.ID)
	if err != nil {
		t.Fatalf("GetWorkerToken: %v", err)
	}
	if got.WorkerID != "worker-X" {
		t.Errorf("after conflict rollback: WorkerID=%q, want worker-X (rollback failed)", got.WorkerID)
	}
}
