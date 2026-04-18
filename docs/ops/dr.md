# Disaster Recovery

This guide describes the MagiC disaster recovery (DR) playbook: targets, scenarios, and procedures for restoring service after a significant failure.

"Disaster" here means events larger than a single-pod restart — database loss, region outage, corrupted state, compromise requiring rebuild.

## RTO and RPO Targets

These are **recommended defaults** for a production deployment. Your contracts and regulatory constraints may require tighter numbers.

| Metric | Target | What it means |
|--------|--------|---------------|
| **RTO** (Recovery Time Objective) | **1 hour** | Time from disaster declaration to service restored. |
| **RPO** (Recovery Point Objective) | **15 minutes** | Maximum tolerable data loss measured in wall-clock time. |

Achieving these requires:

- WAL archiving with `archive_timeout ≤ 60s` **or** streaming replication.
- Backups tested quarterly (see [Backup & Restore](backup-restore.md)).
- A warm standby in a second region, or managed geo-redundancy.
- An incident runbook people have practiced (see [Incident Runbook](runbook-incident.md)).

If your actual RTO/RPO are looser, **publish them to customers** — don't pretend.

## Architecture for DR

Recommended pattern for multi-region DR:

```
      Region A (primary)                 Region B (standby)
  ┌────────────────────────────┐      ┌────────────────────────────┐
  │ MagiC pods (active)        │      │ MagiC pods (scaled to 0    │
  │   ↕ Cloudflare / LB        │      │ or warm, cluster-mode)     │
  │ Postgres primary           │ ───► │ Postgres read replica      │
  │   ↕ WAL stream             │      │   ↕ can be promoted        │
  │ pgvector                   │      │ pgvector                   │
  │ Object store (backups)     │ ───► │ Object store (replicated)  │
  └────────────────────────────┘      └────────────────────────────┘
             │                                        ▲
             └──── DNS / Anycast failover ────────────┘
```

Key properties:

- Postgres primary in Region A, streaming to replica in Region B.
- Backups (dumps + WAL) replicated to Region B object storage.
- MagiC pods in Region B can start quickly — image pulled, config ready.
- DNS TTL ≤ 60s on the service hostname so failover propagates fast.
- Cloudflare (if used) can geo-route or hard-fail between origins.

For lower-cost setups, skip Region B and rely on same-region multi-AZ + a tested restore from backup. Your RTO and RPO numbers go up accordingly — document the tradeoff.

## DR Scenarios

### Scenario 1: Single pod or instance failure

**Impact:** one MagiC process dies.

**Detection:** Prometheus alert on pod restart / health check failure; Kubernetes events.

**Response:** automatic — Kubernetes restarts via liveness probe; leader election (Postgres advisory lock) reassigns cluster-mode tasks to a live pod.

**Manual action:** none, unless restarts are repeated; investigate the cause per the [Incident Runbook](runbook-incident.md).

**RTO:** seconds. **RPO:** zero.

### Scenario 2: All MagiC pods down (config issue, bad release)

**Impact:** API returns 5xx / unreachable.

**Detection:** health check failure + absence of `/metrics` scrape.

**Response:**

1. Declare SEV-1/2 (see [Incident Runbook](runbook-incident.md)).
2. Roll back the release — see [Upgrade Path](upgrade-path.md#rollback).
3. If rollback doesn't help, restore the previous image/binary manually.

**RTO:** 5-15 minutes. **RPO:** zero (DB unaffected).

### Scenario 3: Database failure

**Impact:** MagiC can read env but Postgres is unreachable or corrupt.

**Detection:** connection errors in logs; `magic_db_errors_total` metric; failed `/health` if DB health is part of readiness.

**Response:**

- **Replica available?** Promote the read replica:
  ```bash
  # Managed Postgres — use provider failover API
  aws rds promote-read-replica --db-instance-identifier magic-replica
  # or
  gcloud sql instances promote-replica magic-replica

  # Self-hosted
  pg_ctl promote -D /var/lib/postgresql/data
  ```
- Update `MAGIC_POSTGRES_URL` to point at the promoted instance; restart MagiC pods.
- Verify `/health`.

- **No replica, corruption only?** Restore from latest backup — see [Backup & Restore](backup-restore.md) — accept the RPO gap.

**RTO:** 10-30 minutes with a replica; 1+ hour from backup. **RPO:** streaming lag (typically <5s) with a replica; hours with backup-only.

### Scenario 4: Region failure

**Impact:** entire region unreachable — network, power, or hyperscaler outage.

**Detection:** multi-AZ alerts; external synthetic monitor failure.

**Response:**

1. Declare SEV-1.
2. Promote the Postgres replica in Region B.
3. Scale MagiC in Region B from 0 → target replicas (or start cold if warm wasn't maintained).
4. Update DNS to point the service hostname at Region B's load balancer.
5. Unregister workers that cannot reach the new region; re-register from their new homes. Worker auto-discovery helps for on-site workers.
6. Monitor until stable, then post customer communications per the [Incident Runbook](runbook-incident.md).

**RTO:** 30-60 minutes for warm standby; several hours for cold. **RPO:** seconds with streaming; longer without.

### Scenario 5: Data corruption (human error, bad migration, ransomware)

**Impact:** DB is running but data is wrong.

**Detection:** customer reports; integrity checks fail; audit log shows unauthorized changes.

**Response:**

1. **Freeze writes** — put MagiC into maintenance (stop ingress or scale to 0). Do this immediately; every second of writes narrows your PITR options.
2. Identify the corruption window — when did bad data appear? Audit log is your friend.
3. Restore to a **point-in-time before corruption** on a separate cluster — see [Backup & Restore Scenario 2](backup-restore.md#scenario-2--pitr-to-specific-timestamp).
4. Compare — diff interesting tables between the restored copy and the live DB.
5. Either swap traffic to the restored copy, or cherry-pick corrected rows back into the live DB. The safer choice is swap.
6. Investigate root cause before re-opening writes.

**RTO:** several hours. **RPO:** bounded by when the bad event started.

If corruption was caused by a compromise (ransomware, malicious insider), treat it as a **security incident** first. See [SECURITY.md](../../SECURITY.md) and consider regulatory notification under GDPR / HIPAA.

### Scenario 6: Compromised credentials

**Impact:** API keys, worker tokens, or webhook secrets leaked.

**Detection:** unusual traffic patterns; notifications from secret-scanning services; customer report.

**Response:**

1. Rotate `MAGIC_API_KEY`. This invalidates all clients — coordinate with API users.
2. Revoke affected worker tokens: `DELETE /api/v1/orgs/{orgID}/tokens/{id}`.
3. Rotate webhook secrets; affected customers must reconfigure their receivers.
4. Rotate LLM provider keys.
5. Audit the audit log for any actions taken with the compromised credentials since the suspected leak.
6. File a postmortem; notify customers if their tenants were affected.

**RTO:** 1-4 hours (including client coordination). **RPO:** N/A (data integrity not at stake unless the attacker made writes).

## DR Testing

A DR plan that hasn't been tested is a plan that will fail.

| Cadence | Exercise |
|---------|----------|
| **Quarterly — tabletop** | Walk through one of the scenarios above on paper. 60 minutes. Find gaps. |
| **Quarterly — live restore** | Restore the latest backup to a staging DB. Run smoke tests. Measure actual time. |
| **Annually — full failover drill** | Promote the standby, swap DNS, run the service on the standby for at least 30 minutes. Optionally fail back. |
| **After any incident** | Add the scenario to the next tabletop's rotation if it surfaced a gap. |

Document every drill with: date, participants, scenario, actual RTO, deviations from plan, follow-ups.

## Contact Tree

Every deployment should maintain a contact tree in a known, accessible place (Notion, GitHub Wiki, printed binder, or equivalent). Template:

| Role | Primary | Backup | Phone / Pager | Hours |
|------|---------|--------|----------------|-------|
| Incident Commander | TBD | TBD | TBD | 24/7 rotation |
| Database on-call | TBD | TBD | TBD | 24/7 rotation |
| Cloud infra on-call | TBD | TBD | TBD | 24/7 rotation |
| Executive sponsor | TBD | TBD | TBD | Business hours + SEV-1 |
| Legal counsel | TBD | TBD | TBD | Business hours |
| Communications lead | TBD | TBD | TBD | SEV-1 only |
| Cloud provider support | Per contract | Per contract | Per contract | Per contract |
| LLM provider support | Per contract | Per contract | Per contract | Per contract |
| Managed Postgres support | Per contract | Per contract | Per contract | Per contract |

**TODO: populate this table with real names and numbers for your org before publishing this doc internally.**

## Data Residency Considerations

If you are subject to GDPR, HIPAA, or a contractual data-residency clause, your DR plan **must** respect data location. Pitfalls:

- Object storage backups default to region A but replicate globally — configure explicit regional replication targets.
- Managed Postgres "cross-region read replicas" may cross the boundary you promised customers — verify the replica region.
- "Failover to a different region" might breach data residency — have a contingency that stays within the permitted geography (e.g., multi-AZ same region rather than multi-region).

See [GDPR](../compliance/gdpr.md) and [HIPAA](../compliance/hipaa.md) for more.

## Runbook References

- [Backup & Restore](backup-restore.md) — detailed restore procedures.
- [Incident Response Runbook](runbook-incident.md) — communication templates, severity definitions.
- [Upgrade Path](upgrade-path.md) — rollback procedures during a failed release.

## Related Compliance Documents

- [GDPR](../compliance/gdpr.md)
- [HIPAA](../compliance/hipaa.md)
- [SOC 2](../compliance/soc2.md)
