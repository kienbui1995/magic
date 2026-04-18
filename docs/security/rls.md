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

`store.PostgreSQLStore` exposes:

```go
func (s *PostgreSQLStore) WithOrgContext(
    ctx context.Context,
    orgID string,
    fn func(conn *pgxpool.Conn) error,
) error
```

It acquires one connection from the pool, runs `SELECT set_config(...)` to set
`app.current_org_id`, invokes `fn`, then resets the variable before the
connection is released back to the pool. This prevents leakage of the scope
across requests that happen to reuse the same pooled connection.

## Production deployment checklist

1. Apply migration 005 (`RunMigrations` does this automatically).
2. Connect MagiC as a **non-superuser** PostgreSQL role.
   Superusers and table owners bypass RLS by default:
   ```sql
   CREATE ROLE magic_app LOGIN PASSWORD '...' NOSUPERUSER NOBYPASSRLS;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO magic_app;
   ```
3. Wire the gateway auth middleware to call `WithOrgContext(ctx, token.OrgID, ...)`
   before any store access (future work — tracked under P1 hardening).
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

Tests skip when `MAGIC_POSTGRES_URL` is not set, following the existing
pattern.

## Rollback

`005_rls.down.sql` drops every policy, disables RLS on each table, removes
the supporting indexes, and drops `magic_current_org()`. Safe to apply on a
running system.
