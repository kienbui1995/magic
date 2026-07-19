# HIPAA Considerations

> **Disclaimer.** This document is engineering guidance and is **not legal advice**. HIPAA compliance is jurisdiction-specific (United States), depends on your role (Covered Entity vs. Business Associate), and requires legal review. Engage a qualified healthcare compliance attorney before processing Protected Health Information (PHI) with any AI system, including MagiC.

## The Most Important Line

**MagiC open-source is not HIPAA-compliant out of the box.** It is a toolkit that can be part of a HIPAA-compliant deployment if — and only if — the operator designs, deploys, contracts, and operates it according to HIPAA's Administrative, Physical, and Technical Safeguards.

If you are processing PHI, you must:

1. Sign a **Business Associate Agreement (BAA)** with every sub-processor that will touch PHI — including your LLM provider, vector DB, Postgres provider, and log/observability provider.
2. Implement all three categories of HIPAA safeguards.
3. Conduct a documented risk analysis (45 CFR § 164.308(a)(1)(ii)(A)).
4. Have legal counsel review.

## Business Associate Agreements (BAA)

HIPAA requires a BAA with each Business Associate. For AI systems the critical ones are:

| Role | Who it is | BAA required? |
|------|-----------|---------------|
| **LLM provider** | OpenAI, Anthropic, Google, Azure OpenAI, etc. | **Yes** — each provider's BAA is separate and often requires an enterprise contract tier. Do **not** send PHI to free or consumer API tiers. |
| **Vector DB / semantic search** | Pinecone, Weaviate, Qdrant Cloud, managed pgvector. | Yes. |
| **Database provider** | AWS RDS, Google Cloud SQL, Azure DB, Supabase (enterprise). | Yes — most managed Postgres providers offer a BAA on enterprise tiers only. |
| **Observability** | Datadog, Sentry, New Relic, Splunk. | Yes, if logs or metrics can contain PHI. Prefer PHI-free logging. |
| **Cloud infra (IaaS)** | AWS, GCP, Azure. | Yes — all three major clouds offer BAAs. |
| **MagiC maintainers** | The MagiC open-source project. | **No** — the maintainers do not run the software on your behalf. If / when a managed MagiC SaaS exists, a BAA option will be offered separately. |

**Ollama / self-hosted open-source LLMs** are an alternative to external providers when no BAA can be obtained — the model runs inside your BAA boundary. Performance and quality tradeoffs apply.

## PHI Handling Warnings

- **Never** place PHI in `MAGIC_API_KEY`, worker names, capability names, or any URL path.
- **Never** include PHI in Prometheus metric labels — they are low-cardinality and permanent.
- **Never** include PHI in log messages or trace-attribute values. Redact before logging.
- Task `input` and `output` fields **may** contain PHI if the deployment is fully inside a BAA boundary. Label such tasks with `metadata.contains_phi = true` so you can filter in audit review.
- Knowledge entries and memory turns may persist PHI. Apply retention + deletion policies.
- LLM Gateway fallback to an unsupported provider can leak PHI. Pin providers in your BAA and disable fallback to non-BAA providers.

## Safeguards Checklist

HIPAA Security Rule safeguards mapped to MagiC capabilities.

### Administrative Safeguards (45 CFR § 164.308)

| Safeguard | Operator action | MagiC support |
|-----------|-----------------|---------------|
| Security Management Process (risk analysis, risk management, sanction policy, activity review) | Document annual risk analysis. Define sanction policy. Review activity quarterly. | Audit log (`GET /api/v1/orgs/{orgID}/audit`) is your activity evidence source. |
| Assigned Security Responsibility | Name a Security Officer. | N/A (process). |
| Workforce Security (authorization, clearance, termination) | Document joiner/mover/leaver. Revoke access on termination. | `DELETE /api/v1/orgs/{orgID}/tokens/{id}`; remove role bindings. |
| Information Access Management | Least-privilege role assignments. | RBAC roles `owner`/`admin`/`viewer`; policy engine for capability gating. |
| Security Awareness and Training | Annual training for engineers and support staff. | N/A (process). |
| Security Incident Procedures | Runbook + postmortem. | See [Incident Response Runbook](../ops/runbook-incident.md). |
| Contingency Plan (backup, disaster recovery, emergency mode) | Document + test. | See [Backup & Restore](../ops/backup-restore.md) and [DR](../ops/dr.md). |
| Evaluation | Periodic technical + non-technical evaluation. | Track in your compliance management system. |
| Business Associate Contracts | Sign BAAs (see above). | N/A (contract). |

### Physical Safeguards (45 CFR § 164.310)

MagiC is software; physical safeguards are the responsibility of the IaaS provider and the operator's office policy. Ensure your cloud provider's BAA covers data-center access controls, workstation use, and device & media controls. Do not run MagiC on laptops that may touch PHI without full-disk encryption and MDM.

### Technical Safeguards (45 CFR § 164.312)

| Safeguard | MagiC control | Operator responsibility |
|-----------|---------------|-------------------------|
| **Access Control (§ 164.312(a)(1))** | RBAC with `owner/admin/viewer`; per-org isolation; worker tokens. | Enforce unique user IDs. Integrate with SSO/MFA for human subjects. Automatic logoff — configure at the client / UI layer. |
| **Audit Controls (§ 164.312(b))** | `audit_log` table; bus subscriber records `worker.registered`, `task.routed`, `task.completed`, `task.failed`, etc. | Ship audit entries to an append-only archive (S3 Object Lock). Retain 6 years minimum (HIPAA documentation rule). Review regularly. |
| **Integrity (§ 164.312(c)(1))** | Audit entries are immutable in-app (no update endpoint). Entities have IDs and timestamps. | Use WORM storage for archive. Consider hash-chained audit log (future MagiC feature). |
| **Person or Entity Authentication (§ 164.312(d))** | `MAGIC_API_KEY` for API clients; worker tokens (hashed storage via `token_hash` column); `Authorization: Bearer` header. | Rotate tokens on schedule. Use SSO/MFA for human access paths. |
| **Transmission Security (§ 164.312(e)(1))** | No TLS termination by MagiC itself. Outbound webhook calls can use HTTPS. SSRF protection blocks private IP ranges and DNS rebinding. | Terminate TLS at reverse proxy (nginx, Traefik, cloud LB, Cloudflare). Enforce TLS 1.2+, modern ciphers, HSTS. Internal traffic between MagiC and workers must also be TLS if crossing untrusted networks. |

## Encryption

HIPAA's encryption is "addressable" — you must implement it or document why not. In practice, encrypt always for PHI.

- **In transit:** TLS 1.2 or higher on every hop. MagiC does not terminate TLS; your reverse proxy must.
- **At rest:** enable Postgres tablespace or disk-level encryption. Managed Postgres providers typically enable this by default (verify with your provider's compliance documentation). Backup snapshots inherit encryption only if explicitly configured.
- **Backups:** encrypted, with separate key management from the primary DB keys where possible.
- **Keys:** stored in a KMS (AWS KMS, GCP KMS, Azure Key Vault, Vault). Never in environment variables committed to source control.

## Minimum Necessary Rule

HIPAA's Minimum Necessary standard (45 CFR § 164.502(b)) says you must limit PHI access and use to the minimum necessary for the task.

MagiC mechanisms that help:

- **RBAC viewer role** — read-only accounts for support / analytics.
- **Policy Engine** — block capabilities or tags (`allowed_capabilities`, `blocked_capabilities`) from touching PHI-labeled tasks.
- **Per-org isolation** — tenant boundary; cross-org access requires explicit binding.
- **Audit log** — evidence for review.

Operator responsibilities:

- Redact PHI before passing to agents that don't need it.
- Use the Evaluator to block outputs that leak unexpected PHI.
- Restrict human access to audit log contents — it may contain PHI in request/response payloads.

## Breach Notification

HIPAA Breach Notification Rule (45 CFR §§ 164.400-414):

- Notify affected individuals within **60 days** of discovery.
- Notify HHS — within 60 days for breaches of 500+ individuals; annually for smaller breaches.
- Media notification for breaches of 500+ in a single state.

Use the [Incident Response Runbook](../ops/runbook-incident.md) as the operational backbone and add HIPAA-specific communication templates to your organization's incident plan.

## Recommended Deployment Pattern for PHI

```
    [Clinical apps / EHR]
           │ HTTPS
           ▼
    [TLS-terminating proxy — cloud LB / nginx / Traefik]
           │
           ▼
    [MagiC core] ── audit log → [S3 Object Lock archive — BAA]
       │
       ├─► [Postgres — managed, BAA, encryption at rest, PITR]
       │
       └─► [Worker fleet, all in same BAA/VPC boundary]
                  │
                  └─► [LLM provider — enterprise tier with BAA]
                      (or self-hosted Ollama inside VPC)
```

Key design rules:

- Every component is inside the BAA perimeter.
- No egress to non-BAA services for PHI-carrying traffic.
- Observability stack (logs, metrics, traces) either inside the BAA perimeter or PHI-free by construction.

## Related Documents

- [GDPR Compliance Guide](gdpr.md)
- [SOC 2 Mapping](soc2.md)
- [Incident Response Runbook](../ops/runbook-incident.md)
- [Backup & Restore](../ops/backup-restore.md)
- [Disaster Recovery](../ops/dr.md)
