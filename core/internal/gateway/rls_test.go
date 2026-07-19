package gateway_test

import (
	"context"
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

// TestRLS_CrossTenantIsolation_Postgres verifies that, when backed by
// PostgreSQL, the gateway enforces tenant isolation at the database layer:
// a worker token for orgB cannot observe orgA workers/tasks over HTTP.
//
// Skips when MAGIC_POSTGRES_URL is unset — CI without a Postgres instance
// falls through to the in-memory test matrix.
func TestRLS_CrossTenantIsolation_Postgres(t *testing.T) {
	url := os.Getenv("MAGIC_POSTGRES_URL")
	if url == "" {
		t.Skip("MAGIC_POSTGRES_URL not set — skipping postgres RLS HTTP integration test")
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
	orgA := "rls-http-A-" + suffix
	orgB := "rls-http-B-" + suffix

	// Seed 2 workers + 2 tasks per org.
	seed := func(org string) {
		for i := 0; i < 2; i++ {
			wid := org + "-w-" + string(rune('0'+i))
			if err := s.AddWorker(ctx, &protocol.Worker{
				ID: wid, Name: wid, OrgID: org,
				Status: protocol.StatusActive, RegisteredAt: time.Now(),
			}); err != nil {
				t.Fatalf("AddWorker: %v", err)
			}
			tid := org + "-t-" + string(rune('0'+i))
			if err := s.AddTask(ctx, &protocol.Task{
				ID:      tid,
				Type:    "test",
				Context: protocol.TaskContext{OrgID: org},
			}); err != nil {
				t.Fatalf("AddTask: %v", err)
			}
		}
	}
	seed(orgA)
	seed(orgB)

	// Issue one worker token per org (pre-bound to a worker for simplicity).
	mkToken := func(org string) string {
		raw, hash := protocol.GenerateToken()
		wt := &protocol.WorkerToken{
			ID:        protocol.GenerateID("tok"),
			OrgID:     org,
			WorkerID:  org + "-w-0",
			TokenHash: hash,
			CreatedAt: time.Now(),
		}
		if err := s.AddWorkerToken(ctx, wt); err != nil {
			t.Fatalf("AddWorkerToken: %v", err)
		}
		return raw
	}
	tokenA := mkToken(orgA)
	tokenB := mkToken(orgB)

	// Build a gateway wired to this postgres store.
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

	// Helper to GET /api/v1/workers with a bearer token and decode the list.
	listWorkers := func(token string) []map[string]any {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/workers", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET workers: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET workers: status=%d", resp.StatusCode)
		}
		var out []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	// Assert: orgA token sees ONLY orgA workers (both entries seeded for orgA,
	// neither for orgB). orgB symmetric.
	checkScoped := func(label, token, wantOrg, leakOrg string) {
		list := listWorkers(token)
		for _, w := range list {
			if org, _ := w["org_id"].(string); org == leakOrg {
				t.Errorf("%s: RLS leak — saw %s worker %v", label, leakOrg, w["id"])
			}
		}
		// Must see at least our seeded workers for wantOrg.
		count := 0
		for _, w := range list {
			if org, _ := w["org_id"].(string); org == wantOrg {
				count++
			}
		}
		if count < 2 {
			t.Errorf("%s: expected >=2 workers of %s visible, got %d", label, wantOrg, count)
		}
	}
	checkScoped("orgA-token", tokenA, orgA, orgB)
	checkScoped("orgB-token", tokenB, orgB, orgA)

	// Admin (no token) in dev bypass mode: since we DO have tokens registered,
	// worker endpoints require auth — but /api/v1/workers GET is unauth'd.
	// That path has no worker token in ctx and no OIDC claims, so orgID is
	// empty → RLS bypasses → admin sees both orgs' rows. This is the
	// documented behaviour (see docs/security/rls.md "Bypass mode").
	all := listWorkers("")
	sawA, sawB := 0, 0
	for _, w := range all {
		switch w["org_id"] {
		case orgA:
			sawA++
		case orgB:
			sawB++
		}
	}
	if sawA < 2 || sawB < 2 {
		t.Errorf("bypass mode: expected both orgs visible, got A=%d B=%d", sawA, sawB)
	}
}
