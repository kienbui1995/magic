# Migrate from v0.8 to v1.0

Guide for upgrading existing MagiC v0.x deployments to v1.0.

**Estimated time: 1-2 hours** depending on deployment size.

---

## Before You Start

This guide is for operators running MagiC v0.8.x or earlier. If you're starting fresh, skip to the quickstart in the main README.

**Who should read this:**
- Operators with MagiC in production
- Teams with existing workers deployed
- Deployments using custom storage (PostgreSQL, SQLite)

---

## Pre-Migration Checklist

Run these checks before touching anything:

- [ ] Read the **Breaking Changes** section below
- [ ] Read the `CHANGELOG.md` for v1.0.0 release notes
- [ ] Test in **staging** first (don't jump straight to prod)
- [ ] Take a fresh database backup:
  ```bash
  pg_dump "$MAGIC_POSTGRES_URL" > magic-v0.8-backup.sql
  ```
- [ ] Record current schema version:
  ```bash
  psql "$MAGIC_POSTGRES_URL" -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"
  ```
- [ ] Snapshot Prometheus dashboard (grab error rate, p95 latency, worker count)
- [ ] Announce maintenance window (internal team + customers if applicable)
- [ ] Have rollback plan ready (see **Rollback** section)

---

## Breaking Changes Summary

v1.0.0 introduces **3 breaking changes**. Most are minor; one requires action.

### 1. Store Interface: All Methods Take Context

**Impact:** If you use the **Go SDK directly** (not the Python/TypeScript SDK), method signatures changed.

**Before (v0.8):**
```go
worker, err := store.GetWorker("worker_123")
```

**After (v1.0):**
```go
worker, err := store.GetWorker(ctx, "worker_123")
```

**Who is affected:** Custom Go code calling `sdk/go/internal/store/` methods directly.

**Who is NOT affected:** Python SDK users, TypeScript SDK users, REST API users.

**Fix:** Add `context.Background()` or your request context to all store method calls:
```go
ctx := context.Background()
worker, err := store.GetWorker(ctx, "worker_123")
```

See `sdk/go/examples/` for updated patterns.

### 2. Health Check Response: `version` → `protocol_version`

**Impact:** If you scrape `/health` and parse the response, the field name changed.

**Before (v0.8):**
```json
{
  "status": "ok",
  "version": "0.8.0"
}
```

**After (v1.0):**
```json
{
  "status": "ok",
  "protocol_version": "1.0",
  "server_version": "1.0.0"
}
```

**Who is affected:** Monitoring scripts, load balancer health checks, custom dashboards parsing `/health`.

**Fix:** Update parsing to use `protocol_version` (for protocol compatibility checks) and `server_version` (for release version):
```bash
# Old
curl http://localhost:8080/health | jq -r .version

# New
curl http://localhost:8080/health | jq -r .server_version
```

### 3. Cost Metric Labels: New `org_id` Label

**Impact:** Prometheus metric `magic_cost_total_usd` now has an `org_id` label. Existing dashboards that don't account for labels will show zero.

**Before (v0.8):**
```
magic_cost_total_usd 45.67
```

**After (v1.0):**
```
magic_cost_total_usd{org_id="acme"} 30.00
magic_cost_total_usd{org_id="widgets"} 15.67
```

**Who is affected:** Grafana dashboards, Prometheus alert rules, custom metric parsers.

**Fix:** Update queries to sum across orgs or select a specific org:
```promql
# Old (will show 0 — wrong!)
magic_cost_total_usd

# New (correct)
sum(magic_cost_total_usd)
# or specific org
magic_cost_total_usd{org_id="acme"}
```

---

## New Features to Adopt (Optional but Recommended)

v1.0 adds powerful production features. Not required to upgrade, but recommended to enable during the upgrade window.

### OIDC / JWT Authentication

Replace API key with federated identity (Okta, Auth0, Azure AD):

```bash
# Set these env vars
export MAGIC_OIDC_ISSUER=https://your-idp.com
export MAGIC_OIDC_CLIENT_ID=...
export MAGIC_OIDC_CLIENT_SECRET=...
```

Workers and clients authenticate via OIDC tokens instead of API keys. Useful for multi-team deployments.

See `docs-site/guide/oidc.md` for setup.

### PostgreSQL Row-Level Security (RLS)

Enforce data isolation at the database layer:

```bash
export MAGIC_SECRETS_PROVIDER=env  # or vault/ssm/etc
export MAGIC_DB_ROLE_NAME=magic_app  # non-superuser role
```

With RLS enabled, each organization's data is automatically filtered by the database. Even a SQL injection in MagiC code can't leak data across orgs.

See `docs-site/guide/rls.md` for implementation.

### Redis Rate Limiting

If running **multiple replicas**, use Redis for distributed rate limiting:

```bash
export MAGIC_REDIS_URL=redis://redis:6379
```

Without Redis, each replica has its own rate limit counter (quota per-instance). With Redis, quotas are global across all replicas.

Only needed if: `replicas > 1` or multi-datacenter.

### OpenTelemetry Traces

Export traces to Jaeger, Tempo, or any OTel collector:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
export OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer%20token123
```

You'll see full request tracing from gateway → router → dispatcher → worker.

Optional but highly recommended for production.

### Helm Chart

If running on Kubernetes, v1.0 includes a production-ready Helm chart:

```bash
helm dependency update deploy/helm/magic/
helm install magic deploy/helm/magic/ --namespace magic --create-namespace
```

The chart handles:
- Rolling updates with zero downtime
- Pod disruption budgets
- Prometheus ServiceMonitor
- PostgreSQL subchart (optional)
- Network policies
- Resource limits

See `deploy/README.md` for options.

---

## Step-by-Step Migration

### Step 1: Upgrade the Binary or Image

**Option A: Single instance (systemd)**
```bash
# Get new binary
curl -LO https://github.com/kienbui1995/magic/releases/download/v1.0.0/magic-linux-amd64
chmod +x magic-linux-amd64
sudo mv magic-linux-amd64 /usr/local/bin/magic

# Verify
magic --version
# Should print: magic version 1.0.0
```

**Option B: Docker**
```bash
# Pull new image
docker pull kienbui1995/magic:v1.0.0

# Update docker-compose.yml or your deployment
image: kienbui1995/magic:v1.0.0
```

**Option C: Kubernetes / Helm**
```bash
helm upgrade magic deploy/helm/magic/ \
  --set image.tag=v1.0.0 \
  --wait \
  --timeout 10m
```

### Step 2: Stop the Old Version

```bash
# Systemd
sudo systemctl stop magic

# Docker Compose
docker compose down

# Kubernetes
kubectl scale deploy magic --replicas=0 -n magic
# or: helm upgrade ... --set replicaCount=0
```

### Step 3: Apply Database Migrations

Migrations run automatically on startup. But you can pre-run them if your policy requires separation:

```bash
# Check current version
migrate -database "$MAGIC_POSTGRES_URL" \
  -path core/internal/store/migrations \
  version

# Apply latest
migrate -database "$MAGIC_POSTGRES_URL" \
  -path core/internal/store/migrations \
  up
```

If using Kubernetes, the first pod to start will run migrations (safe with rolling update and additive migrations).

### Step 4: Restart the New Version

```bash
# Systemd
sudo systemctl start magic
journalctl -u magic -f  # watch logs

# Docker Compose
docker compose up -d
docker compose logs -f magic

# Kubernetes
kubectl scale deploy magic --replicas=2 -n magic
# or: helm upgrade ... --set replicaCount=2
kubectl -n magic rollout status deploy/magic
```

Watch for these messages in logs:
```
[INFO] Applying migration: ...
[INFO] Migration 005 completed
[INFO] Ready
```

### Step 5: Verify Health

```bash
# Health check
curl http://localhost:8080/health

# Should print:
# {
#   "status": "ok",
#   "protocol_version": "1.0",
#   "server_version": "1.0.0",
#   "uptime_seconds": 45
# }
```

### Step 6: Update Configuration (Optional)

v1.0 introduces optional YAML config files. You can continue using env vars, or migrate to `magic.yaml`:

```yaml
# magic.yaml
server:
  port: 8080
  cors_origin: https://yourdomain.com

database:
  postgres_url: postgres://...

auth:
  api_key: ${MAGIC_API_KEY}  # still from env
  oidc_issuer: https://your-idp.com  # new optional

storage:
  backend: postgres
  pgvector_dim: 1536

observability:
  otel_endpoint: http://collector:4318
```

```bash
# Run with config
./bin/magic serve --config magic.yaml
```

Env vars override config file values. You don't need to move everything at once.

### Step 7: Update Monitoring and Dashboards

Fix the three items from **Breaking Changes** above:

1. **Go SDK calls**: Add `ctx` parameter
2. **Health check parsing**: Use `server_version` instead of `version`
3. **Prometheus queries**: Add `org_id` label or use `sum()`

### Step 8: Monitor for 24 Hours

Watch these metrics:
- Error rate (`http_requests_total{status=~"5.."}`)
- Task success rate (`magic_tasks_completed_total / (magic_tasks_completed_total + magic_tasks_failed_total)`)
- Worker count (`magic_workers_online`)
- P95 latency (histogram: `http_requests_duration_seconds`)
- Webhook delivery queue depth (`magic_webhook_pending_deliveries`)

No spikes? Good. Stay in this state for at least 1 business day before decommissioning the old version.

---

## Zero-Downtime Deployment (Kubernetes)

If running on Kubernetes with PostgreSQL backend:

1. Database migrations are **additive only** in v1.0 (no destructive drops)
2. Enable rolling updates:
   ```bash
   helm upgrade magic deploy/helm/magic/ \
     --set image.tag=v1.0.0 \
     --set replicaCount=2 \  # minimum 2
     --set podDisruptionBudget.enabled=true \
     --wait \
     --timeout 15m
   ```
3. Watch rollout: `kubectl rollout status deploy/magic -n magic`
4. First pod starts, runs migrations, comes online. Other pods serve traffic. Second pod starts. Traffic transitions.

**Downtime: ~0 seconds** (assuming your client retries on 503).

---

## Rollback Procedure

If something goes wrong after upgrade:

### Option 1: No Schema Changes (Fastest)

If you didn't run migrations or only used additive ones (v1.0 default):

```bash
# Rollback deployment
helm rollback magic <previous-revision> -n magic
# or: change docker image tag, restart

# Old version starts up and reads current schema
# (it's compatible with both old and new code)
```

Done. Zero data loss.

### Option 2: Schema Rollback (Requires Restore)

If a migration broke something unexpectedly:

```bash
# 1. Stop new version
helm upgrade magic deploy/helm/magic/ --set replicaCount=0 -n magic

# 2. Restore backup
psql "$MAGIC_POSTGRES_URL" < magic-v0.8-backup.sql

# 3. Start old version
helm rollback magic <previous-revision> -n magic
kubectl rollout status deploy/magic -n magic
```

**You lose data between backup and rollback.** That's why backups are critical.

---

## Version Skew Tolerance

v1.0 server is compatible with v0.x clients (SDKs) **within the same MAJOR version**.

- **v1.0 server + v0.8 Python SDK**: Works (SDK is HTTP-based, doesn't care about internal Go changes)
- **v1.0 server + v0.8 Go SDK**: **Broken** (Go SDK imports store directly, method signatures changed)
- **v0.8 server + v1.0 Python SDK**: Works (newer client talks to older server via REST)

**Recommendation:** Upgrade SDKs after the server is stable (next day or week). Pin SDK versions in your apps.

---

## FAQ

**Q: Can I skip v0.9 and go straight to v1.0?**
A: Yes. v1.0 is backward compatible with v0.8 (migrations are additive). v0.9 doesn't exist; v0.8 → v1.0 is the path.

**Q: How long does the migration take?**
A: For in-memory (no persistence): 30 seconds. For PostgreSQL: depends on schema size (usually < 5 minutes for tables < 1GB).

**Q: Do workers need to be restarted?**
A: No. Workers keep their tokens and reconnect fine. No breaking changes to the worker protocol.

**Q: What if the migration fails partway?**
A: Stop MagiC, restore the backup, start v0.8 again. No partial state is left behind.

**Q: Can I run v0.8 and v1.0 side by side?**
A: Only with different databases. Sharing a database: not recommended (migrations will conflict).

**Q: Is there a YAML migration tool?**
A: Not yet. Edit env vars → YAML by hand. Usually 5 minutes for a production config.

---

## Related Documents

- [CHANGELOG](../../CHANGELOG.md) — Full list of changes by version
- [Upgrade Path (v0.x policy)](upgrade-path.md) — General versioning and deprecation policy
- [Backup & Restore](backup-restore.md) — Database backup procedures
- [Disaster Recovery](dr.md) — Multi-region / failover strategies
- [Deployment Guide](../../docs-site/guide/deployment.md) — Installation options
- [Observability Guide](../../docs-site/guide/observability.md) — Prometheus and logging

---

## Need Help?

- **GitHub Issues**: https://github.com/kienbui1995/magic/issues
- **Discussions**: https://github.com/kienbui1995/magic/discussions
- **Security**: See [SECURITY.md](../../SECURITY.md) for responsible disclosure
