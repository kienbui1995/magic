# Postgres-backed E2E Tests

The suite in `core/internal/e2e/postgres_test.go` (build tag `e2e`) exercises
the real MagiC stack against an ephemeral PostgreSQL instance spun up by
[testcontainers-go](https://golang.testcontainers.org/). It complements the
MemoryStore E2E suite by catching regressions that only surface with a real
database — migrations, Row-Level Security (RLS), pgxpool hooks, and
concurrent pool pressure.

## Why

The MemoryStore E2E suite covers cross-module wiring but cannot validate:

- SQL migrations (up + down reversibility, ordering, table dependencies)
- RLS policies from migration 005 (`app.current_org_id` session variable)
- pgxpool `BeforeAcquire` / `AfterRelease` hooks that engage RLS at runtime
- Transactional rollback semantics (e.g. `UpdateWorkerToken` CAS)
- Concurrency against a shared connection pool

Running these against a real Postgres (with a non-superuser role so RLS is
enforced) is the only way to catch those classes of bug before production.

## Scenarios

| Test | What it catches |
|------|-----------------|
| `TestE2E_Postgres_Migrations` | up creates every MagiC table; down reverses cleanly |
| `TestE2E_Postgres_BasicCRUD` | Worker CRUD through `PostgreSQLStore` (not Memory) |
| `TestE2E_Postgres_RLS_CrossTenantIsolation` | Two orgs seeded; `WithOrgIDContext(orgA)` hides orgB rows |
| `TestE2E_Postgres_RLS_HTTPLevel` | Full gateway chain auth → rlsScopeMiddleware → store scopes correctly |
| `TestE2E_Postgres_ConnectionPool_Concurrent` | 100 goroutines `AddTask` concurrently; no deadlock, all persisted |
| `TestE2E_Postgres_BeforeAcquireHook` | Scoped ctx sets `app.current_org_id`; AfterRelease resets it for next caller |
| `TestE2E_Postgres_TransactionRollback` | `UpdateWorkerToken` CAS conflict leaves original binding intact |

Image: `pgvector/pgvector:pg16` (Postgres 16 + pgvector preinstalled — migration
002 creates `knowledge_embeddings vector(1536)`).

## Run locally

```bash
cd core
go test -tags=e2e -race -count=1 -timeout=600s \
    -run '^TestE2E_Postgres' ./internal/e2e/...
```

Requires a running Docker daemon reachable via the default socket
(`/var/run/docker.sock` on Linux, `~/.docker/run/docker.sock` on macOS with
Docker Desktop, or the named pipe on Windows).

Expected runtime on a warm machine (image cached):

| Component | Time |
|-----------|------|
| Container start + migrations | ~1.5 s per test |
| Full Postgres suite | ~15 s |
| With `-race` | ~25 s |

The first run after `docker system prune` will pull `pgvector/pgvector:pg16`
(~450 MB) and take 30–60 s longer.

## How RLS is actually tested

Postgres does not enforce RLS against superusers or table owners. The
testcontainers `postgres` role is a superuser, so the helpers create a
non-superuser `magic_app` role and hand the store a connection string
authenticated as that role. This mirrors the production guidance in
`docs/security/rls.md`.

Without this step, the RLS policies from migration 005 would silently
pass through every row — and the tests would give false confidence.

## Fail modes & hints

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `docker required: Cannot connect to the Docker daemon` | Docker not running | `systemctl start docker` or open Docker Desktop |
| `pull access denied for pgvector/pgvector` | Offline / registry blocked | Pre-pull: `docker pull pgvector/pgvector:pg16` |
| `context deadline exceeded` during container start | Slow disk / cold cache | Warm the image once, re-run |
| `relation "policies" does not exist` during migrate up | 005 references tables not in 001 | Fixed — 001 now creates `policies` and `role_bindings` |
| `expected 2 workers visible under RLS, got 4` | Tests ran as superuser | Helper must create non-superuser role (ensured by `setupPostgresStore`) |

If Docker is unavailable, each Postgres test calls `t.Skip` with the error —
the MemoryStore suite still runs in `go test -tags=e2e`.

## CI

`.github/workflows/ci.yml` runs two separate jobs:

- **e2e** — MemoryStore suite, no Docker dependency
- **e2e-postgres** — this suite; uses the preinstalled Docker daemon on
  GitHub-hosted `ubuntu-latest` runners

The jobs run in parallel. A failure in one does not short-circuit the other.
