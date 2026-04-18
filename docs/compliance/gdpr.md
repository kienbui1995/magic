# GDPR Compliance Guide

> **Disclaimer.** This document is provided for engineering and architectural guidance only. It is **not legal advice.** GDPR compliance depends on your specific use case, data, jurisdiction, and contracts. Consult a qualified Data Protection Officer (DPO) or lawyer before making compliance claims about your deployment.

## Purpose

This guide describes how MagiC — as infrastructure software — supports operators who are subject to the [EU General Data Protection Regulation (GDPR)](https://gdpr-info.eu/) and similar regimes (UK GDPR, Swiss revDPA, California CCPA/CPRA by analogy).

## Role Under GDPR

| Role | Who is it? |
|------|-----------|
| **Data Controller** | The organization deploying MagiC and deciding what personal data to process (**you**, the operator). |
| **Data Processor** | MagiC the software, running under the Controller's control. The MagiC project authors act as a processor only when providing managed services (future SaaS). |
| **Sub-processors** | Any third-party services the Controller configures MagiC to call — LLM providers, managed Postgres, vector DBs, observability backends. |

Because MagiC is self-hostable open-source software, **you are the Controller** for any personal data passing through your deployment. The MagiC maintainers do not process your data.

## Data Subject Rights — How MagiC Supports Each

| Right | GDPR Art. | How MagiC helps | Gaps / operator responsibility |
|-------|-----------|-----------------|--------------------------------|
| **Access** (copy of personal data) | Art. 15 | Audit log (`GET /api/v1/orgs/{orgID}/audit`) records every action. Task inputs/outputs are stored in the `tasks` table (JSONB). | **TODO:** implement data export endpoint `GET /api/v1/orgs/{orgID}/export` that bundles all org-scoped rows. Until then, use `pg_dump` with filters. |
| **Rectification** | Art. 16 | Entities are stored in JSONB and can be updated via admin SQL. | No UI for data subject self-serve correction. |
| **Erasure / "right to be forgotten"** | Art. 17 | `DELETE /api/v1/workers/{id}`; cascading queries by `org_id` on every table. | **TODO:** implement a cascading `DELETE /api/v1/orgs/{orgID}/subjects/{subjectID}` that removes tasks, audit entries, knowledge entries, memory turns, prompts referencing the subject. Current workaround: org-level delete + redaction SQL. |
| **Restriction of processing** | Art. 18 | Worker pause via cost controller (`budget.exceeded`). Per-org policy engine can block specific capabilities. | No per-subject processing flag yet. |
| **Portability** | Art. 20 | Same as Access — JSONB blobs are trivially exportable to JSON. | See Access TODO. |
| **Objection** | Art. 21 | Policy Engine can block tasks by capability or metadata. | No UI. |
| **Automated decision-making / profiling** | Art. 22 | Task results are auditable. Evaluator output is logged. | Operator must inform subjects when AI makes automated decisions about them. |

## Lawful Basis

MagiC does not choose the lawful basis — that is the Controller's responsibility. Common bases for AI-assisted workloads:

- **Contract** (Art. 6(1)(b)) — processing required to fulfil a service the subject requested.
- **Legitimate interest** (Art. 6(1)(f)) — requires a documented LIA (Legitimate Interest Assessment). Caveat: high-risk AI processing often fails the balancing test.
- **Consent** (Art. 6(1)(a)) — explicit opt-in. Required for most marketing/personalization AI use.

Document your basis in your DPIA and privacy notice. Do **not** rely on "legitimate interest" by default for profiling or sensitive data.

## Data Retention

Retention is deployment-configurable. MagiC ships with **no automatic expiry** — entities live in PostgreSQL until deleted.

Recommended baseline:

| Entity | Recommended retention | Reason |
|--------|-----------------------|--------|
| Tasks (completed/failed) | 90 days rolling | Debug + audit, then purge |
| Audit log | 12 months minimum | Matches SOC 2 baseline; some regulators require 3 years |
| Workflow records | 90 days | Debug only |
| Knowledge entries | Indefinite until explicit deletion | Business data owned by Controller |
| Memory turns (chat history) | Configurable per session | Typically 30-90 days unless explicit retention use case |
| Webhook deliveries | 30 days | Debug only |
| Cost records | 12 months | Billing reconciliation |

Implement retention with a scheduled purge job (`pg_cron`, k8s `CronJob`) — there is no built-in reaper yet.

## Data Location

MagiC runs where you deploy it. Data residency is controlled by:

- **Database location** — `MAGIC_POSTGRES_URL` points to your managed or self-hosted Postgres. Pin the region.
- **Worker endpoints** — workers run as external HTTP servers. Audit their deployment region.
- **LLM providers** — all LLM calls go through the LLM Gateway. Check each provider's data-processing location and BAA/DPA terms.

For EU data, keep Postgres and workers in the EU. Most major LLM providers now offer EU regions — configure the gateway accordingly.

## Sub-processors

MagiC itself is a processor. Any service the Controller integrates is a sub-processor. Publish a sub-processor list to data subjects; below is a **template** you must complete before publishing.

| Sub-processor | Purpose | Data processed | Location | DPA |
|---------------|---------|----------------|----------|-----|
| PostgreSQL provider (e.g., AWS RDS, Supabase, Neon) | Primary storage | All entities | _TODO — your region_ | _Link to your DPA_ |
| LLM provider(s) (OpenAI, Anthropic, Google, Ollama self-hosted, etc.) | Model inference | Task input/output passed to the model | Per-provider, varies | Per-provider |
| Object storage (if used) | Large artifacts | Task payloads over size threshold | _TODO_ | _TODO_ |
| Observability (Prometheus, logs, APM) | Metrics and logs | Metadata, request IDs, error messages — **no PII if properly configured** | _TODO_ | _TODO_ |
| Email / SMTP (for alerts) | Notifications | Operator email addresses | _TODO — e.g., AWS SES_ | _TODO_ |
| **TODO:** _add your actual sub-processors_ | | | | |

Review this list when you change providers. Notify data subjects of material changes, per Art. 28.

## Breach Notification

GDPR Art. 33 requires notification of a personal data breach to the supervisory authority **within 72 hours** of awareness, with notification to affected subjects if risk is high (Art. 34).

MagiC support for breach detection:

- **Audit log** — `GET /api/v1/orgs/{orgID}/audit` captures access patterns.
- **Rate-limit metrics** — `magic_ratelimit_hits_total` catches brute-force.
- **Webhook events** — subscribe to `task.failed`, `audit.denied`, `budget.exceeded` and forward to your SIEM.
- **Prometheus** — `/metrics` exposes request/latency/error counters.

Breach response — see the [Incident Response Runbook](../ops/runbook-incident.md). Adapt the templates there for regulator-facing notifications. Maintain a breach log with:

- What happened (timeline, detection path).
- Nature of data and approximate number of subjects affected.
- Likely consequences.
- Mitigation taken and planned.

## Data Protection Impact Assessment (DPIA) — Template

Run a DPIA under GDPR Art. 35 when processing is likely to result in high risk — which includes "systematic evaluation of personal aspects using automated processing, including profiling" and "large-scale processing of special categories" (Art. 9 data). Most production AI agent workloads cross one of these triggers.

Minimum DPIA skeleton:

1. **Description** — what does the system do? Which MagiC modules are in scope?
2. **Necessity and proportionality** — why is processing needed? Could a less invasive approach work?
3. **Risks to data subjects** — re-identification, unauthorized access, model leakage, biased outputs.
4. **Mitigations** — RBAC roles, audit log review cadence, encryption in transit/at rest, retention, model choice, human-in-the-loop gates.
5. **Residual risk** — what is left after mitigations? Is it acceptable?
6. **Consultation** — DPO review, and supervisory authority consultation under Art. 36 if residual risk remains high.

A short DPIA (4-6 pages) is fine for most deployments. Keep it updated with material changes.

## Technical Safeguards Checklist

- [ ] TLS on all external endpoints (MagiC does not terminate TLS itself — use a proxy such as Cloudflare, Traefik, nginx, or the cloud load balancer).
- [ ] Encryption at rest — enable Postgres TDE / disk encryption.
- [ ] RBAC bindings created for every org (otherwise MagiC opens access — see `core/internal/rbac/rbac.go`).
- [ ] `MAGIC_API_KEY` set to at least 32 random bytes.
- [ ] Worker tokens rotated at least quarterly.
- [ ] Audit log shipped to an immutable sink (append-only store) with 12-month+ retention.
- [ ] Backups encrypted and tested — see [Backup & Restore](../ops/backup-restore.md).
- [ ] Breach response runbook tested at least annually — see [Incident Runbook](../ops/runbook-incident.md).
- [ ] Sub-processor list up to date and published to subjects.
- [ ] DPIA completed for each high-risk processing activity.

## Related Documents

- [SOC 2 Mapping](soc2.md)
- [HIPAA Considerations](hipaa.md)
- [Incident Response Runbook](../ops/runbook-incident.md)
- [Backup & Restore](../ops/backup-restore.md)
- [Disaster Recovery](../ops/dr.md)
