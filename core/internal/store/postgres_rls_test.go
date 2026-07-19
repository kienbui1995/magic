package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// TestPostgreSQLStore_RLS_CrossTenantIsolation verifies that the RLS policies
// from migration 005 prevent cross-tenant leaks when app.current_org_id is set,
// and that an empty value bypasses RLS (admin/dev mode).
//
// The test seeds 2 workers and 2 tasks per org (orgA, orgB) then queries as
// each org and as "admin" (empty var), checking row visibility.
func TestPostgreSQLStore_RLS_CrossTenantIsolation(t *testing.T) {
	url := os.Getenv("MAGIC_POSTGRES_URL")
	if url == "" {
		t.Skip("MAGIC_POSTGRES_URL not set — skipping RLS integration test")
	}
	if err := store.RunMigrations(url); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	s, err := store.NewPostgreSQLStore(context.Background(), url)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	suffix := time.Now().Format("150405.000000")

	// Seed: 2 workers/org, 2 tasks/org, across orgA + orgB.
	orgs := []string{"rls-orgA-" + suffix, "rls-orgB-" + suffix}
	for _, org := range orgs {
		for i := 0; i < 2; i++ {
			id := org + "-w-" + string(rune('0'+i))
			if err := s.AddWorker(context.Background(), &protocol.Worker{
				ID: id, Name: id, OrgID: org,
				Status: protocol.StatusActive, RegisteredAt: time.Now(),
			}); err != nil {
				t.Fatalf("AddWorker: %v", err)
			}
			tid := org + "-t-" + string(rune('0'+i))
			if err := s.AddTask(context.Background(), &protocol.Task{
				ID:      tid,
				Type:    "test",
				Context: protocol.TaskContext{OrgID: org},
			}); err != nil {
				t.Fatalf("AddTask: %v", err)
			}
		}
	}
	t.Cleanup(func() {
		// Best-effort cleanup: RLS is bypassed here (empty var) so deletes see all.
		for _, org := range orgs {
			for i := 0; i < 2; i++ {
				_ = s.RemoveWorker(context.Background(), org + "-w-" + string(rune('0'+i)))
				// tasks: no Remove method in interface; leave them — test IDs are unique per run.
			}
		}
	})

	// Case 1: bypass mode (empty var) sees every seeded row.
	sawA := countWorkersForOrg(s, orgs[0])
	sawB := countWorkersForOrg(s, orgs[1])
	if sawA != 2 || sawB != 2 {
		t.Fatalf("bypass mode: expected 2+2 seeded workers, got A=%d B=%d", sawA, sawB)
	}

	// Case 2: scope to orgA — should only see orgA rows.
	if err := s.WithOrgContext(ctx, orgs[0], func(conn *pgxpool.Conn) error {
		if n := countViaConn(t, conn, "workers"); n != 2 {
			t.Errorf("orgA: expected 2 workers visible under RLS, got %d", n)
		}
		// RLS must hide orgB rows entirely, even without explicit WHERE.
		if n := countViaConnWhere(t, conn, "workers", "data->>'org_id'", orgs[1]); n != 0 {
			t.Errorf("orgA: leaked %d orgB workers through RLS", n)
		}
		if n := countViaConnWhere(t, conn, "tasks", "data->'context'->>'org_id'", orgs[1]); n != 0 {
			t.Errorf("orgA: leaked %d orgB tasks through RLS", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithOrgContext(orgA): %v", err)
	}

	// Case 3: scope to orgB — symmetric.
	if err := s.WithOrgContext(ctx, orgs[1], func(conn *pgxpool.Conn) error {
		if n := countViaConnWhere(t, conn, "workers", "data->>'org_id'", orgs[0]); n != 0 {
			t.Errorf("orgB: leaked %d orgA workers through RLS", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithOrgContext(orgB): %v", err)
	}

	// Case 4: after WithOrgContext returns, next pool user must start in bypass.
	// (set_config is session-scoped; WithOrgContext resets it before release.)
	if err := s.WithOrgContext(ctx, "", func(conn *pgxpool.Conn) error {
		if n := countViaConn(t, conn, "workers"); n < 4 {
			t.Errorf("bypass after org scope: expected >=4 rows, got %d", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithOrgContext(bypass): %v", err)
	}

	// Case 5: input sanity — quoting/injection via orgID must not escape.
	// We seed a worker whose name tries to look like a quote break; must still be
	// isolated correctly. (The real defence is parameterized queries, but RLS is
	// a second layer.)
	payload := &protocol.Worker{
		ID: "rls-quote-" + suffix, Name: "' OR 1=1 --", OrgID: orgs[0],
		Status: protocol.StatusActive, RegisteredAt: time.Now(),
	}
	if err := s.AddWorker(context.Background(), payload); err != nil {
		t.Fatalf("AddWorker(quoted): %v", err)
	}
	t.Cleanup(func() { _ = s.RemoveWorker(context.Background(), payload.ID) })
	if err := s.WithOrgContext(ctx, orgs[1], func(conn *pgxpool.Conn) error {
		if n := countViaConnWhere(t, conn, "workers", "id", payload.ID); n != 0 {
			t.Errorf("orgB: saw orgA worker with quoted name — RLS leak")
		}
		return nil
	}); err != nil {
		t.Fatalf("WithOrgContext(quote): %v", err)
	}
}

// countWorkersForOrg counts workers at the application layer (not RLS-filtered
// because the pool connection has no org var set).
func countWorkersForOrg(s *store.PostgreSQLStore, org string) int {
	ws := s.ListWorkersByOrg(context.Background(), org)
	return len(ws)
}

func countViaConn(t *testing.T, conn *pgxpool.Conn, table string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func countViaConnWhere(t *testing.T, conn *pgxpool.Conn, table, expr, val string) int {
	t.Helper()
	var n int
	q := "SELECT COUNT(*) FROM " + table + " WHERE " + expr + " = $1"
	if err := conn.QueryRow(context.Background(), q, val).Scan(&n); err != nil {
		t.Fatalf("count %s where: %v", table, err)
	}
	return n
}

