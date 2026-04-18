# SOC 2 Type II Control Mapping

> **Disclaimer.** This is engineering guidance, not an audit report. SOC 2 attestation is issued by an independent CPA after reviewing your controls and evidence over a 6-12 month observation period. MagiC can support your control environment; it cannot by itself make your deployment "SOC 2 compliant." Engage a qualified CPA and consult your compliance team before making any claims.

## Purpose

Map MagiC's built-in features to the [AICPA Trust Services Criteria (TSC) 2017, revised 2022](https://www.aicpa-cima.com/topic/audit-assurance/audit-and-assurance-greater-than-soc-2) that underpin SOC 2 Type II. This helps teams:

- Identify which controls MagiC provides out of the box.
- See which controls are the operator's responsibility (deployment, process, people).
- Plan the gap analysis before engaging an auditor.

## Scope

- MagiC core server (Go), `core/`.
- SDKs (Python / Go / TypeScript) are in-scope only when they are part of the product being audited.
- Workers are third-party systems — audit them separately.

## Trust Services Criteria

SOC 2 Type II covers five TSCs. **Security** is mandatory; the others are optional and selected based on your commitments to customers.

| TSC | MagiC covers it? |
|-----|------------------|
| Security (Common Criteria) | Partially — see CC1–CC9 below |
| Availability | Partially — depends on deployment (HA, backup) |
| Processing Integrity | Partially — evaluator + audit log |
| Confidentiality | Partially — RBAC + encryption at rest depends on operator |
| Privacy | Partially — see [GDPR](gdpr.md); some gaps around consent/notice |

## Common Criteria (Security) Mapping

Below, **control** describes what MagiC provides, and **operator responsibility** describes what the deployment team must add.

### CC6 — Logical and Physical Access Controls

| TSC | MagiC control | Operator responsibility |
|-----|---------------|-------------------------|
| **CC6.1** Logical access to information assets | **RBAC** (`core/internal/rbac/`) with three roles: `owner`, `admin`, `viewer`. Role bindings scoped per org. **Policy Engine** (`core/internal/policy/`) blocks disallowed capabilities. | Create role bindings for every org (empty bindings = open access in dev mode). Integrate with IdP via future SSO/OIDC. |
| **CC6.2** Provisioning and deprovisioning | Worker token issuance via `POST /api/v1/orgs/{orgID}/tokens`; per-org `DELETE /api/v1/orgs/{orgID}/tokens/{id}`. Human subjects via role bindings. | Document joiner/mover/leaver workflow. Rotate tokens when a contractor leaves. |
| **CC6.3** Access modifications | Audit log records all role and token changes. | Review audit log quarterly. |
| **CC6.6** Restriction of logical access | Per-endpoint rate limiting; per-org rate limits; SSRF protection on webhook URLs. | Add a WAF (Cloudflare, AWS WAF) in front of the gateway for volumetric protection. |
| **CC6.7** Identity management | Worker tokens (HMAC-verified `token_hash` column); API keys (32+ bytes enforced). | Store `MAGIC_API_KEY` in a secrets manager (Vault, AWS SM, GCP SM). Never commit. |
| **CC6.8** System controls for malicious software | Dockerfile runs as non-root; multi-stage build; minimal base image. | Scan images in CI (e.g., Trivy, Grype). Subscribe to security advisories. |

### CC7 — System Operations

| TSC | MagiC control | Operator responsibility |
|-----|---------------|-------------------------|
| **CC7.1** Monitoring and logging | Structured JSON logs; Prometheus `/metrics` (14 metrics); audit log API; W3C Trace Context propagation. | Ship logs to a SIEM (Datadog, Elastic, Loki). Alert on error-rate and auth-failure patterns. |
| **CC7.2** Change management | Git history, semantic versioning, `CHANGELOG.md`, release tags. Migrations via `golang-migrate`. | Enforce PR review, require CI green, tag releases, document rollout in change records. |
| **CC7.3** Incident detection and response | Event bus publishes `task.failed`, `budget.exceeded`, webhook delivery failures. | Follow the [Incident Response Runbook](../ops/runbook-incident.md); wire events to PagerDuty / Opsgenie. |
| **CC7.4** Incident response | Runbook templates provided. | Run tabletop exercises quarterly; postmortem every SEV-1/2. |
| **CC7.5** Recovery | Database migrations reversible (`.down.sql`); backup scripts documented. | Follow the [Backup & Restore](../ops/backup-restore.md) and [DR](../ops/dr.md) guides; run restore drills quarterly. |

### CC8 — Change Management

| TSC | MagiC control | Operator responsibility |
|-----|---------------|-------------------------|
| **CC8.1** Change authorization | CODEOWNERS-based review; branch protection on `main`; signed releases (future). | Require 2-person review on main; block direct push; enable branch protection. |

### CC9 — Risk Mitigation

| TSC | MagiC control | Operator responsibility |
|-----|---------------|-------------------------|
| **CC9.1** Risk mitigation | Defense-in-depth: API key, RBAC, policy engine, rate limiting, SSRF block, CORS, body size limit. | Threat-model your deployment; document residual risks. |
| **CC9.2** Vendor management | Sub-processor list (see [GDPR guide](gdpr.md)). `SECURITY.md` discloses scope. | Track vendor SOC 2 reports in your vendor risk register. |

## Availability

| Criterion | MagiC control | Operator responsibility |
|-----------|---------------|-------------------------|
| **A1.1** Capacity planning | Prometheus metrics support trend analysis. Cluster mode with PostgreSQL advisory-lock leader election. | Set autoscaling rules, size DB correctly, monitor RPS and tail latency. |
| **A1.2** Environmental protections | None — MagiC is software only. | Deploy to a cloud provider with data-center controls (SOC 2 attested IaaS). |
| **A1.3** Recovery | Migration up/down; `pg_dump` / PITR supported. | See [DR guide](../ops/dr.md). Target RTO 1h, RPO 15m (deployment-dependent). |

## Processing Integrity

| Criterion | MagiC control | Operator responsibility |
|-----------|---------------|-------------------------|
| **PI1.1** Processing definitions | Task contract enforces timeout, max cost. Evaluator validates outputs against JSON schema. | Define per-task schemas and SLAs. |
| **PI1.4** Detected errors | DLQ (`GET /api/v1/dlq`), webhook retry with exponential backoff, event bus publishes failures. | Monitor DLQ; investigate sustained failures. |
| **PI1.5** System inputs and outputs | `request_id` on every request; `trace_id` on every task/workflow. | Retain request/trace IDs in downstream logs for end-to-end correlation. |

## Confidentiality

| Criterion | MagiC control | Operator responsibility |
|-----------|---------------|-------------------------|
| **C1.1** Identification | Entities tagged with `org_id` for tenancy isolation. | Enforce tenant boundaries in your client code. |
| **C1.2** Encryption in transit | Not terminated by MagiC. | Terminate TLS at the proxy / load balancer. Use TLS 1.2+ with modern ciphers. |
| Encryption at rest | Not built-in. | Enable Postgres TDE or use an encrypted volume. Managed Postgres usually has this on by default. |

## Privacy

See the [GDPR Guide](gdpr.md) for a fuller treatment. Summary:

- MagiC provides audit log, RBAC, and org-scoped storage.
- Gaps: no built-in export or cascading-delete endpoint yet (see TODOs in GDPR doc).
- Consent management, notice, and data subject request tracking are operator responsibilities.

## Gap Analysis — Operator Responsibilities

These items are **not** shipped by MagiC and must be designed and operated by the team running it. An auditor will expect evidence for each.

| Area | What you must do |
|------|------------------|
| TLS termination | Configure reverse proxy / load balancer with modern TLS. Enforce HSTS. |
| Encryption at rest | Enable disk / tablespace encryption on Postgres. |
| Key management | Use a secrets manager for `MAGIC_API_KEY`, worker tokens, webhook secrets, LLM keys. |
| Backups | Daily full + WAL archiving for PITR. Tested quarterly. |
| DR drills | Quarterly tabletop, annual full failover. |
| SIEM / log shipping | Aggregate logs and metrics; alert on anomalies. |
| Access review | Quarterly review of role bindings + tokens. |
| Employee onboarding/offboarding | Document the process; integrate with HR. |
| Vendor risk register | Track sub-processor SOC 2 reports and DPAs. |
| Security training | Annual training for engineers. |
| Penetration testing | At least annually; document findings + remediation. |

## Audit Log Retention

SOC 2 baseline guidance for the audit log:

- **Minimum**: 12 months online, easily queryable.
- **Recommended**: 12 months online + 3 years archived (S3 Glacier, Azure Archive, GCS Archive) for forensic use.
- Integrity: ship audit log events to an append-only sink (S3 Object Lock, WORM) so an attacker with DB access cannot tamper with history.

MagiC writes audit entries to the `audit_log` table and publishes them to the event bus. Subscribe a webhook to `audit.*` events and forward to your archival sink.

## Recommended Evidence Package

For a SOC 2 Type II audit, collect the following over the observation period:

- Role-binding change history (audit log export).
- Sample request logs showing request IDs and trace IDs.
- Backup job logs (success/failure) and at least one restore drill log.
- Incident runbook entries and postmortems.
- Change management records (PRs merged, CI green, release tags).
- Access review reports (quarterly).
- Vulnerability scan reports and dependency upgrade PRs.
- Employee onboarding/offboarding tickets.

## Related Documents

- [GDPR Compliance Guide](gdpr.md)
- [HIPAA Considerations](hipaa.md)
- [Incident Response Runbook](../ops/runbook-incident.md)
- [Backup & Restore](../ops/backup-restore.md)
- [Disaster Recovery](../ops/dr.md)
- [Upgrade Path](../ops/upgrade-path.md)
