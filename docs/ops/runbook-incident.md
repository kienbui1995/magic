# Incident Response Runbook

This runbook is the default response playbook for operational incidents in a MagiC deployment. Adapt it to your organization — severity thresholds, on-call tooling, and communication channels vary.

## Goals

1. Stop the bleeding — restore service faster than investigating root cause.
2. Communicate clearly — internal team, customers, and (if needed) regulators.
3. Learn — blameless postmortem, actionable follow-ups.

## Severity Levels

| Severity | Definition | Examples | Response time | Paging |
|----------|------------|----------|---------------|--------|
| **SEV-1** | Core service down for multiple customers; data loss; active security incident; regulatory breach. | API returning 5xx for >5 min; data corruption confirmed; suspected active intrusion; PHI/PII exposed. | Immediate. All hands. | Page on-call + tech lead + leadership. |
| **SEV-2** | Partial outage; significant degradation; single-customer impact on a critical path; degraded security posture. | Workflow execution stalled; DLQ growing; auth failing for one org; webhook deliveries failing for one customer. | 30 min to engage. Business hours primary. | Page on-call. |
| **SEV-3** | Minor degradation; cosmetic; workaround available. | Single worker offline with automatic failover; noisy metric; docs broken. | Next business day. | Ticket + #ops channel. |
| **SEV-4** | Informational — not an incident. | Planned maintenance, release notification. | Scheduled. | Announcement only. |

When in doubt, **overcall** the severity. It's cheaper to step down than to step up late.

## Escalation Path

```
  On-call engineer (primary)
           │
           │ (acknowledge within 5 min for SEV-1, 15 min for SEV-2)
           ▼
  Tech lead / module owner (see MAINTAINERS.md)
           │
           │ (for SEV-1 that lasts >30 min without a mitigation path)
           ▼
  Executive / delegated owner (CTO, VP Eng, founder)
           │
           │ (for customer-impacting SEV-1 or regulatory exposure)
           ▼
  Legal + Communications
```

Record every handoff in the incident channel with timestamp and decision.

## During the Incident — Commander's Checklist

The **Incident Commander (IC)** owns the response, not the investigation. For small teams the on-call engineer may be both.

1. [ ] Open an incident channel (Slack `#inc-<short-name>` or equivalent).
2. [ ] Declare the severity. Post it at the top of the channel and pin.
3. [ ] Acknowledge the pager. Silence duplicate alerts.
4. [ ] Identify the scope: which customers, which modules, since when.
5. [ ] Publish the first internal update within 10 minutes.
6. [ ] For customer-impacting SEV-1/2: update the public status page.
7. [ ] Keep the channel narrated — every action, every finding, with timestamp.
8. [ ] Rotate if the incident crosses 4 hours. Fatigue causes more incidents.
9. [ ] Declare resolved only after: metrics green for 15 min, customers notified, workaround removed or documented.

### Immediate Mitigation Playbook

Try these in order when symptoms point to MagiC itself:

- **API returning 5xx** — check `/metrics` (`magic_http_requests_total` by status), logs, Postgres health. Consider rolling back the most recent deployment.
- **DLQ growing** — see [`GET /api/v1/dlq`](../../README.md). Pause the affected worker, investigate the common error pattern, drain or purge once fixed.
- **Auth failures spiking** — check `audit.denied` events. Could be rotation of `MAGIC_API_KEY` without propagation, or brute-force attempt — engage security.
- **Workers heartbeat failing** — check network path to workers. Registry marks offline after missed heartbeats (respects `CurrentLoad > 0`).
- **Database unreachable** — confirm Postgres health; check connection pool (`MAGIC_POSTGRES_POOL_MAX`). Failover to replica if configured.
- **Cost controller pausing workers unexpectedly** — check budget policy and `TotalCostToday` (midnight UTC reset). Review `cost.recorded` / `budget.exceeded` events.
- **Memory / CPU spike** — check `magic_events_dropped_total` (event bus back-pressure). Consider restart with increased resources.

If you can't identify the root cause in 15 minutes on a SEV-1, **roll back** and then investigate in a clean environment.

## Communication Templates

### Internal — first update (within 10 min)

```
:rotating_light: INCIDENT: <short title>
Severity: SEV-1
Started: <UTC timestamp>
Detected by: <alert name / customer report>
Impact: <what customers see>
Commander: @<name>
Scribe: @<name>
Status: investigating
Next update: in 15 minutes
```

### Internal — status update

```
:wrench: UPDATE <time UTC>
Status: <investigating | identified | mitigating | monitoring | resolved>
What we know: <one line>
What we're doing: <one line>
Blockers: <one line or "none">
Next update: in <N> minutes
```

### Public status page — investigating

> We are investigating reports of elevated error rates affecting <service area>. We will post an update within 30 minutes or sooner. Customers may experience <symptom>.

### Public status page — identified + mitigating

> We have identified the cause of the elevated error rates as <brief, non-sensitive description>. A mitigation is in progress. Next update at <time UTC>.

### Public status page — resolved

> The incident affecting <service area> was resolved at <time UTC>. Duration: <HH:MM>. Root cause and follow-up actions will be published in a postmortem within 5 business days.

### Customer email — major incident

```
Subject: [Action required | FYI] MagiC service incident — <date>

Hi <customer>,

Between <start UTC> and <end UTC> you may have experienced <impact>.
Root cause: <short description, no internal jargon>.
Mitigation: <what we did>.
Follow-up: <what we will do, and by when>.
Data: <whether any customer data was affected — be specific>.

For details, see our postmortem: <link>.
For questions, reply to this email or write to <support contact>.

Thank you for your patience.
```

**Regulatory notifications** (GDPR Art. 33, HIPAA breach rule) follow separate timelines — see [GDPR](../compliance/gdpr.md) and [HIPAA](../compliance/hipaa.md). Legal owns the regulator-facing message.

## After the Incident — Postmortem

Write a **blameless** postmortem within 5 business days of any SEV-1 or SEV-2. Target audience: engineers who didn't participate.

Use this template:

```markdown
# Postmortem — <title> — <YYYY-MM-DD>

## Summary
1-2 sentence description of what happened and the impact.

## Impact
- Who was affected? (customers, orgs, internal teams)
- Over what time window? (UTC timestamps)
- What was the measurable impact? (errors, data, revenue)

## Timeline (all UTC)
- HH:MM — event or action.
- HH:MM — event or action.
...

## Root Cause
The underlying condition that allowed the incident. Not the trigger.

## 5 Whys
1. Why did X happen? Because Y.
2. Why did Y happen? Because Z.
...

## What Went Well
- ...

## What Went Poorly
- ...

## Where We Got Lucky
- ...

## Action Items

| # | Action | Owner | Target date | Ticket |
|---|--------|-------|-------------|--------|
| 1 | ...    | @...  | YYYY-MM-DD  | #...   |

## Glossary
Terms newcomers might not know.
```

### Principles

- **Blameless.** Describe actions without naming individuals where possible. Focus on systems, not people.
- **Honest.** Write the timeline as it happened, including missteps.
- **Actionable.** Every finding maps to at least one follow-up with an owner and a date.
- **Published.** Internally by default. Publish a redacted version externally for customer-impacting incidents.

## Tools and Integrations

Recommended stack (deployment-specific; mix and match):

- **Paging:** PagerDuty, Opsgenie, Grafana OnCall.
- **Status page:** Statuspage.io, Instatus, self-hosted cstate.
- **Incident channel:** Slack, Discord, Zulip.
- **Metrics / alerting:** Prometheus + Alertmanager (point it at MagiC's `/metrics`).
- **Logs:** ship structured JSON logs to your log store (Loki, Elasticsearch, Datadog).
- **Incident command:** templates in this file; FireHydrant, Jeli, incident.io for larger teams.

Wire MagiC events to your tooling by subscribing a webhook to `task.failed`, `budget.exceeded`, and `audit.denied`.

## Tabletop Exercises

Run a tabletop exercise at least quarterly. Pick a scenario from:

- Database primary fails.
- Region outage.
- Leaked `MAGIC_API_KEY`.
- Compromised worker token.
- Webhook delivery stalled.
- LLM provider outage.

Role-play the response for 60 minutes. Debrief. Update this runbook with any gap you find.

## Related Documents

- [Backup & Restore](backup-restore.md)
- [Disaster Recovery](dr.md)
- [Upgrade Path](upgrade-path.md)
- [GDPR Compliance](../compliance/gdpr.md)
- [HIPAA Considerations](../compliance/hipaa.md)
- [SOC 2 Mapping](../compliance/soc2.md)
