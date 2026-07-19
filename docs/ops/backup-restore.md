# Backup and Restore

This guide covers operational backup and restore for a MagiC deployment running against PostgreSQL. SQLite deployments use the same principles — substitute a file-copy strategy.

MagiC has **no internal backup mechanism**. All persistence is in Postgres, and backup is a database-layer concern. This is intentional: Postgres-native tooling is battle-tested and your cloud provider already offers it.

## What to Back Up

| Artifact | Location | Backup? | Why |
|----------|----------|---------|-----|
| PostgreSQL data | `$MAGIC_POSTGRES_URL` database | **Yes** | All entities — workers, tasks, workflows, teams, knowledge, audit log, webhooks, tokens, DLQ, prompts, memory, costs. |
| `pg_vector` extension data | Same DB | **Yes** | Embeddings live in the `knowledge_embeddings` table. |
| Server config | env vars, `magic.yaml` | Version-control | `magic.yaml` belongs in git. Secrets go in your secrets manager. |
| `MAGIC_API_KEY`, worker tokens, webhook secrets, LLM keys | Secrets manager | Yes (by the secrets manager) | Rotate and back up per your KMS / SM policy. |
| Binaries / Docker images | Registry | Registry retention | Re-deploy from tag rather than backup. |
| Logs / metrics | Log store / Prometheus | Per your SIEM retention | Not part of DR path but needed for forensics. |
| Prometheus TSDB | Ephemeral | No | Scrape again after recovery; do not back up time-series. |

## Backup Methods (Postgres)

Pick one **primary** method based on RPO and scale. Most teams combine (a) managed snapshots + (b) WAL archiving for PITR.

### A. `pg_dump` (logical backup)

```bash
# Full dump of the MagiC database
pg_dump \
  --host="$PGHOST" \
  --username="$PGUSER" \
  --format=custom \
  --compress=9 \
  --file="magic-$(date -u +%Y%m%dT%H%M%SZ).dump" \
  magic

# Schema-only dump (for disaster-recovery smoke tests)
pg_dump --schema-only --format=plain --file=magic-schema.sql magic
```

- Pros: portable, easy to test, easy to filter.
- Cons: Snapshot-in-time only; not suitable for high-RPO requirements. Downtime or lock pressure on very large DBs.

### B. Continuous archiving + PITR

Point-in-time recovery with WAL archiving is the gold standard for production.

Key settings in `postgresql.conf`:

```conf
wal_level           = replica
archive_mode        = on
archive_command     = 'aws s3 cp %p s3://<bucket>/wal/%f'  # or equivalent
archive_timeout     = 60   # seconds — caps RPO
max_wal_senders     = 10
```

Take a base backup with `pg_basebackup`:

```bash
pg_basebackup \
  --host="$PGHOST" \
  --username=replicator \
  --pgdata=/backups/base-$(date -u +%Y%m%d) \
  --format=tar \
  --gzip \
  --wal-method=stream \
  --checkpoint=fast
```

- Pros: restore to any transaction in the archived window.
- Cons: more moving parts; test end-to-end quarterly.

### C. Managed database snapshots

If you use AWS RDS / Aurora, GCP Cloud SQL, Azure DB for Postgres, Supabase, Neon, or similar — **use the provider's snapshot + PITR feature.**

- AWS RDS: automated backups with 1-35 day retention + manual snapshots.
- GCP Cloud SQL: automated backups + binary log PITR.
- Azure DB for Postgres: automatic geo-redundant backups.
- Neon / Supabase / Crunchy Bridge: built-in PITR.

Delegating to the managed provider removes most of the operational burden. **Verify** that snapshots are encrypted and that the provider holds a SOC 2 / HIPAA BAA if required.

## Retention Policy

Default recommendation for production:

| Tier | Frequency | Retain | Storage class |
|------|-----------|--------|---------------|
| WAL / PITR window | Continuous | 7-14 days | Hot |
| Daily full | Daily | 7 dailies | Warm |
| Weekly full | Weekly | 4 weeklies | Warm |
| Monthly full | Monthly | 12 monthlies | Cold (Glacier / Archive / Coldline) |
| Annual full | Yearly | 7 years (or per your retention policy) | Cold |

Tune to your RPO target and your regulatory obligations. HIPAA demands 6 years of documentation; GDPR demands retention to be no longer than necessary — balance.

## Encryption and Access Control

- Encrypt backups at rest. S3 with SSE-KMS, GCS with CMEK, Azure Blob with CMK.
- Use a **different** key for backup storage than for the live DB so a compromised DB key does not unlock backups.
- Restrict IAM to backup-writer and restore-reader roles. No human should have read access to all backups; require a break-glass review.
- Keep backup bucket versioning + MFA Delete enabled for immutability.

## Restore — Step by Step

Restore is a **drill-until-boring** procedure. Do it on a non-prod cluster first, always.

### Scenario 1 — Restore latest `pg_dump`

```bash
# 1. Spin up target Postgres (empty). Let MagiC run migrations first.
./magic serve & sleep 5 && kill %1
# (MagiC runs golang-migrate on startup, creating tables.)

# 2. Or restore the dump directly, which creates tables:
pg_restore \
  --host="$PGHOST" \
  --username="$PGUSER" \
  --dbname=magic \
  --clean --if-exists \
  --no-owner --no-privileges \
  --jobs=4 \
  magic-20260418T120000Z.dump

# 3. Verify row counts against the source.
psql -c "SELECT 'workers' t, COUNT(*) FROM workers
         UNION ALL SELECT 'tasks', COUNT(*) FROM tasks
         UNION ALL SELECT 'audit_log', COUNT(*) FROM audit_log;"

# 4. Start MagiC.
./magic serve
```

### Scenario 2 — PITR to specific timestamp

Using standard Postgres recovery; steps vary by managed provider. Generalized:

```bash
# 1. Stop traffic to the primary (if still reachable). Put MagiC in maintenance.

# 2. Take down the primary; bring up a recovery cluster from the base backup.
tar -xzf base-20260418.tar.gz -C /var/lib/postgresql/data

# 3. Configure recovery:
cat > /var/lib/postgresql/data/recovery.signal <<'EOF'
EOF
cat >> /var/lib/postgresql/data/postgresql.conf <<'EOF'
restore_command = 'aws s3 cp s3://<bucket>/wal/%f %p'
recovery_target_time = '2026-04-18 14:32:00+00'
recovery_target_action = 'promote'
EOF

# 4. Start Postgres; it will replay WAL up to the target and promote.
systemctl start postgresql

# 5. Smoke test: connect, count rows, hit /health.
curl http://localhost:8080/health

# 6. Point MAGIC_POSTGRES_URL at the restored instance and restart MagiC.
```

### Scenario 3 — Managed provider snapshot restore

```bash
# AWS RDS example
aws rds restore-db-instance-from-db-snapshot \
  --db-instance-identifier magic-restored \
  --db-snapshot-identifier magic-2026-04-18-1200 \
  --db-subnet-group-name magic-private

# GCP Cloud SQL example
gcloud sql backups restore BACKUP_ID \
  --restore-instance=magic-primary \
  --backup-instance=magic-primary
```

Always restore to a **new** instance name, validate, then swap traffic. Never overwrite the primary until you are certain.

## Post-Restore Checklist

- [ ] `curl /health` returns healthy.
- [ ] `curl /metrics` exports metrics.
- [ ] Audit log query returns recent entries.
- [ ] A tasks query returns expected count (compare to backup metadata).
- [ ] Worker registration works.
- [ ] A canary task round-trips end-to-end.
- [ ] Webhook deliveries resume (check `webhook_deliveries` with `status = 'pending'`).
- [ ] Rotate any credentials that may have leaked during the incident.
- [ ] Update the status page + postmortem with the timeline.

## Testing — Restore Drills

**Untested backups are wishes, not backups.**

- **Quarterly:** restore the latest daily dump to a staging DB. Run the smoke test. Record the elapsed time — this is your **actual** RTO for this scenario.
- **Annually:** full DR drill — simulated region loss, restore from cold storage, run the app against it, have a customer-facing team run through their workflow.
- Log every drill with: scenario, steps executed, deltas from plan, elapsed time. Publish to the maintainers channel.

If a drill uncovers a gap (e.g., a new table missing from your logical backup filter), update the runbook **immediately**.

## Cross-Region Replication

For multi-region DR:

- **Streaming replication** — Postgres streaming to a warm standby in another region. Lag is typically <1 s; RPO ≈ replication lag.
- **Logical replication** — per-table replication; flexible but more operationally heavy.
- **Managed providers** — AWS RDS read replicas, Aurora Global Database; GCP Cloud SQL cross-region replicas; Azure DB for Postgres cross-region read replicas.

Promotion to writer is a DR decision, not an incident response one. See the [Disaster Recovery guide](dr.md).

## Schema Migrations

MagiC uses [`golang-migrate`](https://github.com/golang-migrate/migrate). Migrations live in `core/internal/store/migrations/`. On startup, MagiC runs `migrate up` automatically.

For restore into a version older than current:

```bash
# Check current migration version
migrate -database "$MAGIC_POSTGRES_URL" -path core/internal/store/migrations version

# Forward migrate after restoring an older dump
migrate -database "$MAGIC_POSTGRES_URL" -path core/internal/store/migrations up

# Roll back one migration (DANGEROUS — only with a confirmed backup)
migrate -database "$MAGIC_POSTGRES_URL" -path core/internal/store/migrations down 1
```

For version-skew during upgrade, see the [Upgrade Path guide](upgrade-path.md).

## Related Documents

- [Disaster Recovery](dr.md)
- [Upgrade Path](upgrade-path.md)
- [Incident Response Runbook](runbook-incident.md)
- [SOC 2 Mapping](../compliance/soc2.md)
