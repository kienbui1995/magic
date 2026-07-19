# Upgrade Guide

This guide covers upgrading a running MagiC deployment: versioning policy, pre-upgrade checks, rollout strategies, database migrations, and rollback.

## Versioning Policy

MagiC follows [Semantic Versioning 2.0.0](https://semver.org/).

| Segment | Meaning | What to expect |
|---------|---------|----------------|
| **MAJOR** (X.y.z) | Breaking changes | API removals, protocol-incompatible changes, config-format rewrites. Requires deliberate planning and usually a deprecation cycle. |
| **MINOR** (x.Y.z) | Additive, backward-compatible | New endpoints, new fields, new config options with safe defaults. Can be rolled in place. |
| **PATCH** (x.y.Z) | Bug fixes | No new features. Safe to deploy without reading much. Security patches ship as patch releases. |

Before `1.0.0` we may ship breaking changes in MINOR releases but always document them in [`CHANGELOG.md`](../../CHANGELOG.md) with migration notes.

## Deprecation Policy

When a feature is deprecated:

1. It is announced in `CHANGELOG.md` under the deprecation's release.
2. A runtime warning is logged each time the deprecated path is hit.
3. The feature continues to work for at least **one full MINOR release**.
4. It is removed in the release after that MINOR, announced in the `### Removed` section.

Example timeline:

- `0.9.0` — feature deprecated, warning added.
- `0.10.0` — still works, still warns.
- `0.11.0` — removed.

## Pre-Upgrade Checklist

Run through this every time, no matter how small the bump looks.

- [ ] Read [`CHANGELOG.md`](../../CHANGELOG.md) from your current version to target. Flag any `### Changed` or `### Removed`.
- [ ] Read the release notes for the target version on GitHub Releases.
- [ ] Take a fresh backup — see [Backup & Restore](backup-restore.md).
- [ ] Record the current migration version:
      ```bash
      migrate -database "$MAGIC_POSTGRES_URL" -path core/internal/store/migrations version
      ```
- [ ] Snapshot Prometheus dashboards / grab a baseline for error rate, p95 latency, worker count.
- [ ] Test the target version in **staging** with production-like load, minimum 1 hour.
- [ ] Have the rollback plan open in another tab. Rehearse it.
- [ ] Announce the maintenance window internally and (if customer-impacting) externally.

## Version Skew

MagiC core ↔ SDK version skew is generally safe **within one MAJOR boundary**:

- A newer server accepts older clients (SDKs) that speak the same MAJOR version.
- A newer client may call endpoints that don't exist on an older server — expect 404.

Roll SDKs **after** the server is upgraded, and pin SDK versions in client apps.

## Rollout Strategies

### Single instance (dev, small prod)

```bash
# 1. Backup
pg_dump ... > pre-upgrade.dump

# 2. Stop MagiC
systemctl stop magic  # or kill the process / stop docker

# 3. Replace binary or image
mv magic magic.old
curl -LO https://github.com/kienbui1995/magic/releases/download/v0.9.0/magic-linux-amd64
chmod +x magic-linux-amd64 && mv magic-linux-amd64 magic

# 4. Start — migrations run on boot
systemctl start magic
journalctl -u magic -f  # watch migration output

# 5. Verify
curl http://localhost:8080/health
curl http://localhost:8080/metrics | head
```

Downtime: 30 seconds to 2 minutes depending on migration cost.

### Docker Compose

```bash
# Bump image tag in docker-compose.yml, then:
docker compose pull
docker compose up -d
docker compose logs -f magic
```

### Kubernetes / Helm (recommended for production)

```bash
# Point at the target chart version + image tag
helm upgrade magic ./deploy/helm/magic \
  --reuse-values \
  --set image.tag=v0.9.0 \
  --wait \
  --timeout 10m

# Check pod rollout
kubectl rollout status deploy/magic
```

Kubernetes does a rolling update by default. Ensure:

- `replicas >= 2`.
- PodDisruptionBudget allows a single pod down.
- `maxSurge=1`, `maxUnavailable=0` for zero-downtime.
- **Cluster mode** (leader election via Postgres advisory lock) is enabled when running multiple replicas.

Database migrations run on pod startup. With rolling update, the **first** pod to start the new version runs the migration while older pods still serve traffic. Additive migrations (add column, add table, add index concurrently) are safe with this pattern. Destructive migrations (drop column, rename) are **not** — plan these as two-phase migrations across two releases.

### Canary / Blue-Green

For higher-risk releases:

1. Deploy the new version behind a different service name (`magic-canary`).
2. Route a small fraction of traffic (10%) via your load balancer, service mesh, or API gateway.
3. Watch metrics and logs for 30-60 minutes.
4. Shift traffic gradually (10% → 50% → 100%).
5. Decommission the old version.

This assumes the release does not include incompatible migrations. If it does, either both versions must tolerate both schemas during the window, or you must do a full cutover with a short downtime.

## Database Migrations

MagiC uses [`golang-migrate`](https://github.com/golang-migrate/migrate) with SQL files in `core/internal/store/migrations/`.

Migrations are applied automatically on server startup by reading the `schema_migrations` table.

### Additive migrations (zero-downtime)

- Add new nullable columns.
- Add new tables.
- Add indexes `CREATE INDEX CONCURRENTLY` (Postgres).
- Add new JSONB fields.

These are safe with rolling update.

### Destructive migrations (requires planning)

- Dropping a column.
- Renaming a column.
- Making a nullable column `NOT NULL`.
- Changing types.

Handle these as a **two-phase release**:

1. **Release N:** new code tolerates both old and new schema (reads/writes both). Ship.
2. **Release N+1:** run destructive migration. Ship.

Never combine a destructive migration with a breaking code change in the same release.

### Manual migration control

If your deployment policy requires separating migration from deploy:

```bash
# Dry run — inspect what migrate would do
migrate -database "$MAGIC_POSTGRES_URL" -path core/internal/store/migrations version

# Apply up to target version
migrate -database "$MAGIC_POSTGRES_URL" -path core/internal/store/migrations up

# Roll back one migration (with a confirmed backup!)
migrate -database "$MAGIC_POSTGRES_URL" -path core/internal/store/migrations down 1
```

To disable the built-in migrator and run migrations externally, **pin the image to a version whose migrations you have already applied**. We do not yet expose a `MAGIC_AUTO_MIGRATE=false` flag; if you need one, open an issue.

## Rollback

Plan A: **roll back the deployment** (no schema change).

- Helm: `helm rollback magic <previous-revision>`
- Docker Compose: change tag, `docker compose up -d`.
- Systemd: swap the binary, `systemctl restart magic`.

Plan B: **destructive migration in the release — restore database.**

1. Stop all MagiC pods (scale to 0 or `systemctl stop`).
2. Restore the pre-upgrade backup — see [Backup & Restore](backup-restore.md).
3. Redeploy the previous version.
4. Start.

This is disruptive. Better to avoid destructive migrations in the same release as anything else.

## Post-Upgrade Verification

- [ ] `/health` reports ready.
- [ ] `/metrics` returns the expected Prometheus payload.
- [ ] A canary task round-trips end-to-end.
- [ ] Workers re-register (or resume heartbeats).
- [ ] Webhook deliveries are clearing the queue.
- [ ] Error rate, p95 latency, and saturation are within the baseline.
- [ ] No new entries in the DLQ for 15 minutes.
- [ ] Run a spot-check query against the audit log — entries should be landing.

Keep a rollback window of at least one business day. Don't take the previous version offline immediately.

## Upgrading the Go Runtime

MagiC currently targets **Go 1.25+**. The Go version is documented in `go.mod` and in the Dockerfile. Upgrading to a newer Go minor version is an internal concern; downstream consumers do not need to track it. If you vendor the source, ensure your Go toolchain matches the stated minimum.

## Upgrading Postgres

MagiC supports Postgres 13+ (pgvector requires 11+; 15+ recommended).

Major-version Postgres upgrades are a dedicated operation:

- Use `pg_upgrade` for in-place major version upgrades.
- Use `pg_dump` + `pg_restore` for cross-provider or cross-version migration.
- Test the pgvector extension version compatibility first (`SELECT * FROM pg_extension`).

After the Postgres upgrade, restart MagiC. Its migrations will re-check `schema_migrations` and no-op if already current.

## Upgrading SDKs

- **Python:** pin in `requirements.txt` or `pyproject.toml`. Upgrade with `pip install -U magic-ai-sdk`.
- **Go:** `go get github.com/kienbui1995/magic/sdk/go@<version>`.
- **TypeScript:** `npm install @magic-ai/sdk@<version>`.

SDK upgrades are independent of server upgrades within the same MAJOR version.

## Related Documents

- [Backup & Restore](backup-restore.md)
- [Disaster Recovery](dr.md)
- [Incident Response Runbook](runbook-incident.md)
- [CHANGELOG](../../CHANGELOG.md)
