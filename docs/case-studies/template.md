# Case Study: [Company/Project Name]

Share how you built production AI with MagiC. This template guides you through the key sections.

> **To submit a case study:** Fork the repo, fill in this template, save as `docs/case-studies/your-company-name.md`, and open a PR. Or email `hello@magic-ai.dev` with a completed template.

---

## Company Profile

**Company / Project Name:**
- [Your company or open-source project name]

**Industry:**
- [Healthcare, Finance, E-commerce, Media, Enterprise SaaS, etc.]

**Team Size:**
- [Number of engineers, AI researchers, data scientists]

**MagiC Version:**
- [e.g., 0.8.0 or 1.0+]

**Deployment:**
- [ ] Kubernetes (Helm)
- [ ] Docker Compose
- [ ] Self-hosted VMs
- [ ] Cloud (AWS/GCP/Azure)

---

## The Problem

**What problem were you solving?**

Describe the business challenge:
- What were you trying to build?
- Why did existing solutions fall short?
- What was the technical debt or scaling challenge?

Example:
> We were running 20+ AI agents for content moderation, customer support, and data extraction. Each agent was a monolithic Python script with hardcoded retry logic, no cost tracking, and no way to balance load across workers. When one agent crashed, the entire pipeline went down.

**Scale & Context:**
- Tasks per day / week / month
- Number of agents / workers
- Primary use cases (e.g., content moderation, customer support, research)
- Pain points with previous approach

---

## Why MagiC?

**Why did you choose MagiC instead of alternatives?**

Consider:
- Temporal (if you use that)
- Dapr (distributed application runtime)
- Build-your-own orchestration
- Other frameworks (Celery, RQ, Kafka, etc.)

Example comparison:
> We evaluated Temporal, but its learning curve was steep, and it didn't understand LLM semantics (token counting, cost tracking, fallback strategies). We considered building our own scheduler, but that's a 2-month project. MagiC gave us worker orchestration + cost tracking + RBAC out of the box. One engineer integrated the first agent in a day.

**Key decision factors:**
- Built-in AI features (cost tracking, token counting, semantic search)
- Language support (Go core, Python/Go/TS SDKs)
- Multi-tenancy (teams, RBAC, billing)
- Extensibility (plugins for routing, evaluation, policies)
- Operational maturity (persistence, monitoring, resilience)

---

## Architecture

**Diagram (ASCII or description):**

```
Client Applications
    │
    ├─→ Content Moderation API
    │       │
    │       └─→ MagiC Gateway
    │           (auth, cost tracking, policy)
    │
    ├─→ Support Chatbot
    │       │
    │       └─→ MagiC Worker Registry
    │           (track 8 agents)
    │
    └─→ Data Extraction Pipeline
            │
            └─→ MagiC Router
                (load balance across agents)
                    │
                    ├─→ CrewAI Agent (3 instances)
                    ├─→ LangChain Agent (2 instances)
                    └─→ Custom Agent (3 instances)
                        │
                        └─→ PostgreSQL
                            (tasks, costs, audit logs)
                        │
                        └─→ Prometheus / Grafana
                            (dashboards)
                        │
                        └─→ Slack Webhooks
                            (budget alerts)
```

**Key components:**
- How many workers / agents?
- Storage backend (PostgreSQL, SQLite, in-memory)?
- Routing strategy (best_match, round_robin, cheapest)?
- Persistent features used (knowledge hub, webhooks, cost tracking)?

---

## Implementation

**Workers deployed:**
- Total count: [e.g., 15 workers]
- Languages: [e.g., 8 Python, 4 Go, 3 TypeScript]
- Frameworks wrapped:
  - [e.g., 5 CrewAI crews]
  - [e.g., 3 LangChain agents]
  - [e.g., 2 AutoGen agents]
  - [e.g., 2 custom HTTP servers]

**Task volume:**
- Baseline (QA/staging): [e.g., 500 tasks/day]
- Peak (production): [e.g., 15K tasks/day]
- Latency targets: [e.g., P50: 200ms, P95: 2s, P99: 10s]

**Key configuration decisions:**

> **Cost limit per task:** We set `max_cost_per_task = $0.50` to prevent runaway OpenAI bills. Agents that exceed this get auto-paused by MagiC's cost controller until the next day.

> **Routing strategy:** Started with `best_match` (find agent with highest capability score), switched to `cheapest` once we had cost data. Saved 30% on LLM spend.

> **Persistence:** Used SQLite in dev, switched to PostgreSQL with read replicas in prod. RLS (row-level security) ensures team A can't see team B's tasks.

> **Multi-tenancy:** Each customer org has its own token + API key. Webhooks send cost reports to their Slack channel daily.

**Integration effort:**
- Time to first worker integrated: [e.g., 4 hours]
- Time to productionize (auth, monitoring, backups): [e.g., 1 week]
- Team size working on integration: [e.g., 2 engineers]

---

## Results

### Quantitative

| Metric | Before MagiC | After MagiC | Impact |
|--------|--------------|------------|--------|
| Task latency (P95) | 5s | 500ms | 10x faster |
| Task failure rate | 15% | 0.5% | 30x more reliable |
| Cost per task | $0.12 | $0.08 | 33% cheaper |
| Time to deploy new agent | 3 days | 2 hours | 36x faster |
| Unplanned downtime / month | 6 hours | 0 | 100% uptime |
| Ops cost (monitoring time) | 20 hrs/week | 2 hrs/week | 10x savings |

### Qualitative

**Developer experience:**
> "Before MagiC, adding a new agent meant writing 500 lines of boilerplate (queues, retries, monitoring). Now it's 50 lines — just a `@worker.capability` decorator. Agents stay in their domain language (Python, JavaScript, etc.), and MagiC handles the hard parts."

**Operational confidence:**
> "We never worry about budget overruns or worker crashes anymore. MagiC's dashboard shows real-time costs and worker health. When an agent is unhealthy, we get a Slack alert within seconds. We sleep better."

**Time to market:**
> "Three months ago, adding a new agent to production took a week of engineering + a week of QA. Now it's 1 day. We shipped 5 new agents last quarter instead of 1."

---

## Lessons Learned

**What worked well:**

1. **Wrapping, not rewriting.** We didn't touch the CrewAI crews or LangChain agents. We just wrapped them as MagiC workers. Zero risk of breaking existing logic.

2. **Cost transparency.** Once we could see per-agent costs, we optimized prompts and model selection. Saved $2K/month just by switching from GPT-4 to GPT-3.5 for certain agents.

3. **RBAC from day one.** We set up team-based role bindings early. Prevented a customer from accessing another's audit logs (a compliance issue that could've been expensive).

**What we'd do differently:**

1. **Cluster mode earlier.** We ran a single MagiC instance for 3 months, then hit latency walls at 10K tasks/day. Switched to 3-pod cluster with PostgreSQL, and problems disappeared. Could've done this from month 1.

2. **Monitoring from the start.** We didn't set up Prometheus until month 2. Spent days debugging task latency blind. Now Grafana dashboards are part of day 1 setup.

3. **Knowledge hub sooner.** We built a manual knowledge cache before discovering MagiC's semantic search. Replaced it with pgvector in a day. Agents now share context automatically.

---

## Looking Forward

**Roadmap:**

- [ ] Add 10 more agents (targeting 40 total by Q3 2026)
- [ ] Migrate to OIDC authentication (replace API keys)
- [ ] Multi-region deployment (Asia + US + EU)
- [ ] Open-source our agent framework for the community
- [ ] Implement dynamic routing (AI-driven agent selection based on historical performance)

**Scaling plans:**

> We expect to handle 100K tasks/day by end of 2026. PostgreSQL + Redis + multi-region Kubernetes should handle that. We're also exploring worker auto-scaling based on queue depth.

---

## Quotes

> "MagiC transformed our ops. Before, AI orchestration was invisible and fragile. Now it's transparent, reliable, and scalable."
> — **Alice Chen, VP Engineering**

> "I can focus on building better agents instead of plumbing. That's huge."
> — **Bob Santos, ML Engineer**

---

## About the Author

**Name:** [Your name]

**Title:** [Your role]

**Company:** [Your company]

**LinkedIn / GitHub / Website:** [Your profile]

**How to reach out:** [Email or message]

---

## Supporting Materials

**Optional attachments:**

- [ ] Grafana dashboard screenshot
- [ ] Architecture diagram (high-res)
- [ ] Benchmark results (latency / throughput graphs)
- [ ] Cost report (monthly spending, savings)
- [ ] Sample worker code (anonymized if needed)

**Links:**

- Internal case study wiki: [link if public]
- GitHub repo: [link if open-source]
- Blog post: [link if published]

---

**Template version:** 1.0

**Last updated:** 2026-04-18

**Questions?** Open an issue or email hello@magic-ai.dev
