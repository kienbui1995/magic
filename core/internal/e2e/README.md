# E2E Tests

End-to-end tests exercising the full MagiC stack in-process:

- **Gateway** (HTTP handler with middleware + rate limiting)
- **Registry**, **Router**, **Dispatcher**, **Orchestrator**
- **Store** (MemoryStore — no Postgres required)
- **Event bus**, **CostCtrl**, **Evaluator**, **Monitor** + Prometheus metrics
- **Webhook manager** with HMAC-signed delivery
- Workers implemented as `httptest.NewServer` handlers

Gated by the `e2e` build tag so unit-test runs (`go test ./...`) are not affected.

## Run

```bash
cd core
go test -tags=e2e -race -count=1 -timeout=180s ./internal/e2e/...
```

Verbose output:

```bash
go test -tags=e2e -v ./internal/e2e/...
```

## Scenarios

| Test | What it catches |
|------|-----------------|
| `TestE2E_TaskLifecycle` | register → submit → complete; cost recorded; task.completed event; `magic_tasks_total` incremented |
| `TestE2E_WebhookDelivery` | task.completed triggers HMAC-signed POST to receiver (verifies X-MagiC-Event + X-MagiC-Signature + envelope) |
| `TestE2E_TaskCancel` | pending task → cancel → status cancelled + task.cancelled event; no task.completed raced in |
| `TestE2E_WorkerPauseResume` | paused worker skipped by router (503); resume restores routing |
| `TestE2E_WorkflowDAG` | 2-step workflow with `depends_on` runs sequentially |
| `TestE2E_RateLimit` | 60 parallel task submissions trigger at least one 429 at the per-IP burst of 20 |
| `TestE2E_AuditLog` | audit query endpoint returns filtered + paginated entries with expected JSON shape |

## Timing

- Runtime: < 30s total on a warm machine.
- `TestE2E_WebhookDelivery` dominates because the retry sender ticks on a 5s interval — up to ~15s wallclock there.

## Scope / non-scope

**In scope**: catching regressions across module boundaries (gateway ↔ dispatcher ↔ store ↔ bus ↔ webhook sender).

## Postgres-backed E2E

`postgres_test.go` (same build tag) covers migrations / RLS / pool under a
real Postgres via testcontainers-go. See `docs/testing/e2e-postgres.md` for
run instructions and fail modes. Tests auto-skip when Docker is unavailable.

**Out of scope** (future work):
- OIDC / JWT auth path — needs a fake issuer.
- OTel exporter verification — needs an in-process collector.
