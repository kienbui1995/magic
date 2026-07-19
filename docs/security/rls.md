# PostgreSQL Row-Level Security (RLS)

MagiC enforces tenant isolation at the database layer using PostgreSQL Row-Level
Security. RLS is a defence-in-depth layer **below** the application's `org_id`
filtering: even if a Go handler forgets to scope a query, the database itself
refuses to return another org's rows.

## Tables under RLS

Migration `005_rls.up.sql` enables RLS and installs one policy per table:

| Table                | Predicate                                   |
| -------------------- | ------------------------------------------- |
| `workers`            | `data->>'org_id' = current_org`             |
| `webhooks`           | `data->>'org_id' = current_org`             |
| `webhook_deliveries` | `data->>'org_id' = current_org`             |
| `policies`           | `data->>'org_id' = current_org`             |
| `role_bindings`      | `data->>'org_id' = current_org`             |
| `worker_tokens`      | `data->>'org_id' = current_org`             |
| `audit_log`          | `data->>'org_id' = current_org`             |
| `tasks`              | `data->'context'->>'org_id' = current_org`  |
| `workflows`          | `data->'context'->>'org_id' = current_org`  |
| `knowledge`          | `scope <> 'org' OR scope_id = current_org`  |

`current_org` is `COALESCE(current_setting('app.current_org_id', true), '')`,
wrapped in a stable SQL function `magic_current_org()`.

## Bypass mode (empty variable)

Every policy starts with `magic_current_org() = '' OR ...`. When the session
variable is unset or empty, RLS passes all rows through — this is the default
state of a freshly acquired pool connection. Rationale:

- **Backward compatibility.** Existing Go code that has not been updated to
  call `SetOrgContext` continues to work.
- **Admin/cron paths.** Webhook dispatcher, migration runners, audit exporters
  need to read across all orgs.
- **Opt-in hardening.** RLS only kicks in once the gateway's auth middleware
  starts setting `app.current_org_id` to the authenticated token's org.

## Application API

`store.PostgreSQLStore` exposes two complementary entry points.

### 1. Context-scoped (automatic, used by the gateway)

```go
ctx = store.WithOrgIDContext(ctx, orgID)
// every store call on this ctx runs under RLS for orgID
workers := s.ListWorkers(ctx)
```

The pool is configured with `pgxpool.Config.BeforeAcquire` / `AfterRelease`
hooks. When a connection is acquired with a ctx that carries an orgID,
BeforeAcquire runs `SET app.current_org_id`; AfterRelease always resets the
value before the connection returns to the pool so it can't leak to the next
request. Empty orgID (or a non-postgres backend) is a no-op.

### 2. Explicit closure (for code paths that hold a conn directly)

```go
func (s *PostgreSQLStore) WithOrgContext(
    ctx context.Context,
    orgID string,
    fn func(conn *pgxpool.Conn) error,
) error
```

Useful for tests and for any caller that needs to run multiple statements on
a single pinned connection.

## Runtime wiring (gateway)

The middleware chain is (outer → inner):

```
otel → cors → securityHeaders → apiVersion → oidc → auth → bodySize
     → requestID → rbac → rlsScope → mux
```

`rlsScopeMiddleware` (in `internal/gateway/middleware.go`) runs after auth/rbac
have populated the context and extracts the orgID from, in priority order:

1. OIDC JWT claims (`auth.ClaimsFromContext(ctx).OrgID`)
2. Worker token (`gateway.TokenFromContext(ctx).OrgID`)
3. Path parameter `{orgID}` for `/api/v1/orgs/{orgID}/...`

It then stamps the context via `store.WithOrgIDContext`. The first store call
triggers `BeforeAcquire`, which engages RLS for the rest of the request.

When no source is available (health checks, dev mode without auth, admin
bootstrap) the ctx is left untouched → `app.current_org_id` stays empty →
RLS bypasses (see "Bypass mode" above). This preserves backward compatibility
and keeps cross-org admin flows working.

## Production deployment checklist

1. Apply migration 005 (`RunMigrations` does this automatically).
2. Connect MagiC as a **non-superuser** PostgreSQL role.
   Superusers and table owners bypass RLS by default:
   ```sql
   CREATE ROLE magic_app LOGIN PASSWORD '...' NOSUPERUSER NOBYPASSRLS;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO magic_app;
   ```
3. The gateway's `rlsScopeMiddleware` now stamps `ctx` with the caller's
   orgID on every request — no application-level action required. Verify
   by checking the middleware chain in `internal/gateway/gateway.go`.
4. Admin CLIs / migration runners should connect with an account that is
   expected to bypass RLS, or leave `app.current_org_id` unset.

## Performance

All RLS predicates are backed by expression indexes (see migration 005):

- `idx_tasks_context_org`, `idx_workflows_context_org` for nested JSONB paths.
- `idx_webhooks_org`, `idx_policies_org`, `idx_role_bindings_org`,
  `idx_worker_tokens_org`, `idx_webhook_deliveries_org`.
- `idx_knowledge_scope` compound index for the scope/scope_id predicate.

EXPLAIN a typical `SELECT ... FROM tasks` under RLS to confirm the planner
uses the index; if it doesn't, run `ANALYZE` to refresh statistics.

## Testing

`core/internal/store/postgres_rls_test.go` verifies:

1. Two orgs are seeded with disjoint workers + tasks.
2. Admin (empty var) sees all rows.
3. Under `WithOrgContext(orgA)`, orgB rows are invisible at the SQL layer
   (even an unscoped `SELECT COUNT(*) FROM workers` returns only orgA).
4. After `WithOrgContext` returns, the pool connection is back in bypass mode.
5. A worker with a SQL-injection-shaped name (`' OR 1=1 --`) stays isolated
   to its org — RLS is enforced regardless of payload content.

`core/internal/gateway/rls_test.go` adds an HTTP-level integration test
(`TestRLS_CrossTenantIsolation_Postgres`) that spins up the full gateway
against a real postgres, issues two worker tokens for two orgs, and
asserts an orgB bearer token cannot observe orgA workers via
`GET /api/v1/workers` — RLS enforced end-to-end, not just at the store.

Run either suite with:

```bash
MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic_test?sslmode=disable" \
  go test -race -count=1 ./internal/store/... ./internal/gateway/...
```

Tests skip when `MAGIC_POSTGRES_URL` is not set, following the existing
pattern.

## Troubleshooting

RLS not filtering as expected? Run the checklist:

1. **Connected as a superuser or table owner?** These bypass RLS by default.
   Run `SELECT current_user;` and confirm it's `magic_app` (or your
   non-superuser role).
2. **Migration 005 applied?** `SELECT relname, relrowsecurity FROM pg_class
   WHERE relname IN ('workers','tasks','workflows');` — all three must show
   `t`.
3. **`app.current_org_id` actually set?** Add a log line around the failing
   query: `SELECT current_setting('app.current_org_id', true);`. If empty,
   the BeforeAcquire hook didn't fire → ctx was never stamped → check that
   the handler received an authenticated request (OIDC claim or worker
   token).
4. **Using the right ctx?** Store methods must receive the request-scoped
   ctx. A goroutine spawned with `context.Background()` will acquire a
   connection with no orgID → RLS bypass. Propagate ctx or re-stamp.
5. **Mixed statements on one conn?** If you use `WithOrgContext` and
   simultaneously another goroutine uses the same store, each acquires its
   own conn — they don't interfere, but each is responsible for its own
   scope.

## Rollback

`005_rls.down.sql` drops every policy, disables RLS on each table, removes
the supporting indexes, and drops `magic_current_org()`. Safe to apply on a
running system.
