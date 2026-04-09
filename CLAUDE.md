# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**MagiC** (capital C = Company / Crew / Claw) is an open-source framework for managing fleets of AI workers. Think "Kubernetes for AI agents" — it doesn't build agents, it manages any agents built with any tool (CrewAI, LangChain, custom bots, etc.) through an open protocol.

- **Core:** Go (all 9 modules implemented)
- **SDK:** Python (`pip install magic-claw`) + Go SDK (`sdk/go/`)
- **Protocol:** MagiC Protocol (MCP²) — transport-agnostic JSON messages over HTTP
- **License:** Apache 2.0

## Current Status

**Implementation is complete.** All modules built, tested, and merged to main.

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1 — Foundation | ✅ Done | Gateway, Registry, Router, Monitor, Python SDK |
| Phase 2 — Tier 2 | ✅ Done | Orchestrator, Evaluator, CostCtrl, OrgMgr |
| Phase 3a — Storage | ✅ Done | PostgreSQL backend, pgvector semantic search |
| Phase 3b — Real-time | ✅ Done | SSE streaming, Webhooks, Prometheus metrics |

Design specs: `docs/superpowers/specs/`
Implementation plans: `docs/superpowers/plans/`

## Architecture

### Project Structure

```
magic-claw/
├── core/                          # Go — MagiC framework server
│   ├── cmd/magic/main.go          # CLI entrypoint: magic serve
│   └── internal/
│       ├── protocol/              # MCP² message types & entity definitions
│       ├── store/                 # Store interface + Memory/SQLite/PostgreSQL impls
│       │   └── migrations/        # golang-migrate SQL migrations
│       ├── events/                # Event bus (pub/sub)
│       ├── gateway/               # HTTP server, middleware, handlers, rate limiting
│       ├── registry/              # Worker registration & health monitoring
│       ├── router/                # Task routing (best_match, round_robin, cheapest)
│       ├── monitor/               # Structured logging + Prometheus metrics
│       ├── orchestrator/          # Multi-step workflow DAG execution
│       ├── evaluator/             # Output quality assessment
│       ├── costctrl/              # Budget & cost tracking
│       ├── orgmgr/                # Organization, teams, RBAC
│       ├── knowledge/             # Shared knowledge hub + semantic search
│       ├── webhook/               # Event-driven webhook delivery (at-least-once)
│       └── audit/                 # Audit log
├── sdk/python/                    # Python SDK
├── sdk/go/                        # Go SDK
└── examples/                      # Example workers
```

### Module Tiers

| Tier | Modules | Purpose |
|------|---------|---------|
| **Core** | Gateway, Registry, Router, Monitor | Minimum viable framework |
| **Differentiator** | Orchestrator, Evaluator, Cost Controller, Org Manager | Key value props |
| **Bonus** | Knowledge Hub, Webhook, Audit | Production features |

### Storage Backends

Auto-selected from env vars at startup:
```bash
MAGIC_POSTGRES_URL=postgres://...   # PostgreSQL (production)
MAGIC_STORE=path/to/db.sqlite       # SQLite (file persistence)
# neither → in-memory (dev/default)
```

PostgreSQL extras:
- `MAGIC_POSTGRES_POOL_MIN/MAX` — connection pool size
- `MAGIC_PGVECTOR_DIM` — embedding dimension for semantic search (default: 1536)

### Key API Endpoints

```
POST   /api/v1/workers/register              Register a worker
POST   /api/v1/tasks                         Submit a task
POST   /api/v1/tasks/stream                  Submit + stream result (SSE)
GET    /api/v1/tasks/{id}/stream             Re-subscribe to task stream
POST   /api/v1/workflows                     Submit a workflow
GET    /api/v1/knowledge                     Search knowledge
POST   /api/v1/knowledge/{id}/embedding      Store embedding (pgvector)
POST   /api/v1/knowledge/search/semantic     Semantic search
POST   /api/v1/orgs/{orgID}/webhooks         Register webhook
GET    /api/v1/metrics                       JSON stats
GET    /metrics                              Prometheus metrics (no auth)
GET    /health                               Health check
```

## Build Commands

```bash
# Go core
cd core && go build ./cmd/magic/
cd core && go test ./...
cd core && go test ./internal/gateway/ -v  # single package

# Run server (dev — in-memory store)
cd core && go build ./cmd/magic/ && ./magic serve

# Run server (with SQLite)
cd core && MAGIC_STORE=./dev.db ./magic serve

# Run server (with PostgreSQL)
cd core && MAGIC_POSTGRES_URL="postgres://user:pass@localhost/magic" ./magic serve

# Python SDK
cd sdk/python && pip install -e ".[dev]"
cd sdk/python && pytest
```

## Key Design Decisions

- **Go for core** — performance, goroutines, small binary; follows K8s/Docker precedent
- **JSONB storage pattern** — entities stored as JSON blobs (not normalized), easy to evolve schema
- **VectorStore separate from Store** — avoids import cycle (`knowledge` imports `store`; `store` must not import `knowledge`). `knowledge.VectorStore = store.VectorStore` (type alias).
- **WorkerToken.TokenHash** — has `json:"-"`, stored in dedicated `token_hash TEXT` column (not in JSONB blob)
- **Event bus pub/sub** — all modules communicate through events, not direct calls
- **Workers are external HTTP servers** — MagiC routes tasks, proxies SSE; workers implement capabilities
- **LLM calls never from MagiC core** — workers call LiteLLM proxy; MagiC is infrastructure only
- **Webhooks at-least-once** — queue in store, retry with 30s→5m→30m→2h→8h backoff, max 5 attempts

## Actual Bus Event Types

(Workers and internal modules publish these exact strings — match carefully in webhook subscriptions)

```
task.dispatched    task.completed    task.failed
worker.registered  worker.deregistered  worker.heartbeat
workflow.completed workflow.failed  workflow.started
cost.recorded      budget.threshold  budget.exceeded
knowledge.added    knowledge.deleted  knowledge.queried
```
