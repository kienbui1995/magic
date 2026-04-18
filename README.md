# MagiC

[![CI](https://github.com/kienbui1995/magic/actions/workflows/ci.yml/badge.svg)](https://github.com/kienbui1995/magic/actions/workflows/ci.yml)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![Python 3.11+](https://img.shields.io/badge/Python-3.11+-3776AB?logo=python)](https://python.org)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/kienbui1995/magic/badge)](https://scorecard.dev/viewer/?uri=github.com/kienbui1995/magic)
[![govulncheck](https://github.com/kienbui1995/magic/actions/workflows/ci.yml/badge.svg)](https://github.com/kienbui1995/magic/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kienbui1995/magic/core)](https://goreportcard.com/report/github.com/kienbui1995/magic/core)

> Don't build another AI. Manage the ones you have.

MagiC is an **AI-native framework** for building and managing fleets of AI agents. It provides the infrastructure layer that every AI application needs: **LLM gateway** (multi-provider routing, cost tracking), **prompt management** (versioned templates, A/B testing), **agent memory** (conversation history, vector recall), and **worker orchestration** (DAG workflows, capability-based routing).

Unlike generic task queues, MagiC is purpose-built for AI workloads — it understands tokens, models, costs, and agent state.

```
         Your App
            │
       MagiC Server
      ┌─────┼─────────────┐
      │     │             │
  LLM Gateway  Prompt Registry  Agent Memory
  (OpenAI,     (versioned,      (conversation +
   Anthropic,   A/B testing)     vector search)
   Ollama)          │
      │        Worker Orchestration
      └─────── DAG workflows ──────┘
              /    |    \
         SearchBot  SumBot  AnalyzeBot
         (Python)  (Node)   (Go)
```

## Quick Start (< 5 minutes)

### Option A: pip install (fastest)

```bash
pip install magic-ai-sdk
```

Then create `worker.py`:

```python
from magic_ai_sdk import Worker

worker = Worker(name="HelloBot", endpoint="http://localhost:9000")

@worker.capability("greeting", description="Says hello")
def greet(name: str) -> str:
    return f"Hello, {name}! Managed by MagiC."

worker.register("http://localhost:8080")
worker.serve()
```

```bash
python worker.py  # Registered and serving on :9000
```

Submit a task:
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"type":"greeting","input":{"name":"World"}}'
```

### Option B: From source

**Prerequisites:** Go 1.25+, Python 3.11+

```bash
# 1. Clone and build
git clone https://github.com/kienbui1995/magic.git
cd magic
cd core && go build -o ../bin/magic ./cmd/magic && cd ..

# 2. Start the server
./bin/magic serve

# 3. Install Python SDK
cd sdk/python && pip install -e . && cd ../..
```

### Option C: Docker

```bash
# From Docker Hub
docker run -p 8080:8080 kienbui1995/magic:latest

# Or build locally
docker build -t magic .
docker run -p 8080:8080 magic
```

### Option D: One-click cloud deploy

[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template/magic)
[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy)
[![Deploy on Fly.io](https://img.shields.io/badge/Deploy%20on-Fly.io-purple?logo=fly.io)](https://fly.io/docs/getting-started/)

### Create your first worker

Save as `worker.py`:

```python
from magic_ai_sdk import Worker

worker = Worker(name="HelloBot", endpoint="http://localhost:9000")

@worker.capability("greeting", description="Says hello to anyone")
def greet(name: str) -> str:
    return f"Hello, {name}! I'm managed by MagiC."

if __name__ == "__main__":
    worker.register("http://localhost:8080")  # connect to MagiC server
    worker.serve()                            # start listening on :9000
```

```bash
python worker.py
# Output: Registered as worker_abc123
#         HelloBot serving on 0.0.0.0:9000
```

### Submit a task

```bash
# Submit — MagiC routes to HelloBot and dispatches automatically
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "greeting",
    "input": {"name": "World"},
    "routing": {"strategy": "best_match", "required_capabilities": ["greeting"]},
    "contract": {"timeout_ms": 30000, "max_cost": 1.0}
  }'

# Check result (use the task_id from response above)
curl http://localhost:8080/api/v1/tasks/{task_id}
```

### Enable authentication

```bash
MAGIC_API_KEY=your-secret-key ./bin/magic serve
```

Workers and API calls must include the key:
```bash
curl -H "Authorization: Bearer your-secret-key" http://localhost:8080/api/v1/workers
```

## Examples

| Example | Description | Location |
|---------|-------------|----------|
| **Hello Worker** | Minimal 10-line worker | [`examples/hello-worker/`](examples/hello-worker/) |
| **Multi-Worker** | 2 workers + workflow + cost tracking | [`examples/multi-worker/`](examples/multi-worker/) |

Run the multi-worker example:
```bash
# Terminal 1: Start MagiC server
./bin/magic serve

# Terminal 2: Start workers + submit tasks
pip install httpx  # required for the example
python examples/multi-worker/main.py
```

## Why MagiC?

### AI-Native Infrastructure

| Feature | Generic Queue (Temporal/Celery) | MagiC |
|---|---|---|
| LLM Gateway | ❌ Build yourself | ✅ Multi-provider routing, fallback, cost tracking |
| Prompt Management | ❌ Build yourself | ✅ Versioned templates, A/B testing |
| Agent Memory | ❌ Build yourself | ✅ Conversation history + vector recall |
| Token Counting | ❌ N/A | ✅ Automatic per-request |
| Model Cost Tracking | ❌ N/A | ✅ Per-model, per-worker, budget alerts |

### vs. AI Frameworks

| Feature | CrewAI | LangGraph | **MagiC** |
|---|---|---|---|
| Approach | Build agents | Build graphs | **Manage + power any agent** |
| LLM Gateway | Single provider | Single provider | **Multi-provider routing** |
| Prompt Registry | No | No | **Versioned + A/B testing** |
| Agent Memory | Basic | Checkpoints | **Conversation + vector search** |
| Language | Python only | Python only | **Any (Go core, Python/Go/TS SDK)** |
| Cost Control | No | No | **Budget alerts + auto-pause** |
| Worker Orchestration | Crew flow | Graph | **DAG with parallel execution** |

**MagiC doesn't replace CrewAI/LangChain — it powers them.** Your CrewAI agent becomes a MagiC worker with LLM routing, prompt management, and memory built in.

## Architecture

```
                ┌──────────────────────────────────────────────┐
                │              MagiC Core (Go)                 │
                ├──────────────────────────────────────────────┤
  HTTP Request ─>  Gateway (auth, body limit, request ID)      │
                │    │                                         │
                │    v                                         │
                │  Router ──> Registry (find best worker)      │
                │    │          │                               │
                │    v          v                               │
                │  Dispatcher ──> Worker A (HTTP POST)         │
                │    │              Worker B                    │
                │    │              Worker C                    │
                │    v                                         │
                │  Orchestrator (multi-step DAG workflows)     │
                │  Evaluator (output quality validation)       │
                │  Cost Controller (budget tracking)           │
                │  Org Manager (teams, policies)               │
                │  Knowledge Hub (shared context)              │
                │  Monitor (events, metrics, logging)          │
                └──────────────────────────────────────────────┘
```

### How it works

1. **Worker registers** with MagiC, declaring its capabilities (e.g., "content_writing", "data_analysis")
2. **User submits a task** via REST API with required capabilities
3. **Router finds the best worker** based on strategy (best_match, cheapest, etc.)
4. **Dispatcher sends HTTP POST** to the worker's endpoint with `task.assign` message
5. **Worker processes and responds** with `task.complete` or `task.fail`
6. **Cost Controller tracks spending**, Monitor logs everything, Evaluator validates output

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/dashboard` | Web UI — monitor workers, tasks, costs |
| `POST` | `/api/v1/workers/register` | Register a worker |
| `POST` | `/api/v1/workers/heartbeat` | Worker heartbeat |
| `GET` | `/api/v1/workers` | List workers (`?limit=&offset=`) |
| `GET` | `/api/v1/workers/{id}` | Get worker by ID |
| `POST` | `/api/v1/tasks` | Submit a task (auto-routes + dispatches) |
| `GET` | `/api/v1/tasks` | List tasks (`?limit=&offset=`) |
| `GET` | `/api/v1/tasks/{id}` | Get task by ID (poll for completion) |
| `POST` | `/api/v1/workflows` | Submit a multi-step workflow |
| `GET` | `/api/v1/workflows` | List workflows (`?limit=&offset=`) |
| `GET` | `/api/v1/workflows/{id}` | Get workflow by ID |
| `POST` | `/api/v1/teams` | Create a team |
| `GET` | `/api/v1/teams` | List teams |
| `GET` | `/api/v1/costs` | Organization cost report |
| `POST` | `/api/v1/knowledge` | Add knowledge entry |
| `GET` | `/api/v1/knowledge?q=<query>` | Search knowledge |
| `GET` | `/api/v1/metrics` | System metrics |
| `POST` | `/api/v1/tasks/stream` | Submit task + stream result as SSE |
| `GET` | `/api/v1/tasks/{id}/stream` | Re-subscribe to task SSE stream |
| `POST` | `/api/v1/knowledge/{id}/embedding` | Store vector embedding (pgvector) |
| `POST` | `/api/v1/knowledge/search/semantic` | Semantic similarity search |
| `POST` | `/api/v1/orgs/{orgID}/webhooks` | Register webhook |
| `GET` | `/api/v1/orgs/{orgID}/webhooks` | List webhooks |
| `GET` | `/metrics` | Prometheus metrics (no auth) |

## Multi-Step Workflows (DAG)

Submit a workflow with dependencies — MagiC handles parallel execution, failure handling, and step sequencing:

```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Product Launch Campaign",
    "steps": [
      {"id": "research", "task_type": "market_research", "input": {"topic": "AI trends"}},
      {"id": "content", "task_type": "content_writing", "depends_on": ["research"], "input": {"tone": "professional"}},
      {"id": "seo", "task_type": "seo_optimization", "depends_on": ["content"], "on_failure": "skip", "input": {}},
      {"id": "leads", "task_type": "lead_generation", "depends_on": ["research"], "input": {}},
      {"id": "outreach", "task_type": "email_outreach", "depends_on": ["leads", "content"], "input": {}}
    ]
  }'
```

```
      research
       /    \
  content    leads       <- parallel
     |         |
    seo        |
      \       /
     outreach            <- waits for both branches
```

Failure handling per step: `retry`, `skip`, `abort`, `reassign`.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `MAGIC_PORT` | `8080` | Server port |
| `MAGIC_API_KEY` | _(empty = no auth)_ | API key — **minimum 32 characters** (`openssl rand -hex 32`) |
| `MAGIC_CORS_ORIGIN` | _(none)_ | Allowed CORS origin (e.g. `https://yourdomain.com`) |
| `MAGIC_RATE_LIMIT_DISABLE` | `false` | Set `true` to disable rate limiting (dev/testing only) |
| `MAGIC_POSTGRES_URL` | _(empty)_ | PostgreSQL connection URL (enables PostgreSQL backend) |
| `MAGIC_STORE` | _(empty)_ | SQLite file path (e.g. `./magic.db`) |
| `MAGIC_PGVECTOR_DIM` | `1536` | Embedding dimension for semantic search |

### Rate Limiting

MagiC includes built-in per-endpoint rate limiting (token bucket):

| Endpoint | Limit |
|----------|-------|
| `POST /api/v1/workers/register` | 10 req / IP / min |
| `POST /api/v1/workers/heartbeat` | 4 req / IP / min |
| `POST /api/v1/orgs/{id}/tokens` | 20 req / org / min |
| `POST /api/v1/tasks` | 200 req / org / min |

**For production deployments**, supplement with Cloudflare or nginx for distributed attack protection:

```nginx
# nginx example
limit_req_zone $binary_remote_addr zone=magic:10m rate=30r/m;
limit_req zone=magic burst=10 nodelay;
```

```
# Cloudflare: Zero Trust → WAF → Rate Limiting Rules
# Recommended: 60 req/min per IP on /api/v1/*
```

## Project Structure

```
magic/
├── core/                           # Go server (9 modules)
│   ├── cmd/magic/main.go           # CLI entrypoint
│   └── internal/
│       ├── protocol/               # MCP² types & messages
│       ├── store/                  # Storage interface + Memory/SQLite/PostgreSQL
│       │   └── migrations/         # golang-migrate SQL migrations
│       ├── events/                 # Event bus (pub/sub)
│       ├── gateway/                # HTTP server + middleware
│       ├── registry/               # Worker registration
│       ├── router/                 # Task routing strategies
│       ├── dispatcher/             # HTTP dispatch to workers
│       ├── monitor/                # Logging + metrics
│       ├── orchestrator/           # Workflow DAG execution
│       ├── evaluator/              # Output validation
│       ├── costctrl/               # Budget tracking
│       ├── orgmgr/                 # Team management
│       ├── knowledge/              # Knowledge hub
│       ├── webhook/                # At-least-once webhook delivery
│       └── audit/                  # Structured audit log
├── sdk/python/                     # Python SDK (pip install magic-ai-sdk)
│   ├── magic_ai_sdk/
│   │   ├── worker.py               # Worker class
│   │   ├── client.py               # HTTP client
│   │   └── decorators.py           # @capability decorator
│   └── tests/
├── examples/
│   ├── hello-worker/main.py        # 10-line minimal example
│   └── multi-worker/main.py        # 2 workers + workflow + costs
├── Dockerfile                      # Multi-stage Docker build
└── docs/
    └── superpowers/
        └── specs/                  # Design specification
```

## Development

```bash
# Build
cd core && go build -o ../bin/magic ./cmd/magic

# Run tests (with race detection)
cd core && go test ./... -v -race

# Run single package test
cd core && go test ./internal/router/ -v

# Start dev server
cd core && go run ./cmd/magic serve

# Python SDK
cd sdk/python
python -m venv .venv && .venv/bin/pip install -e ".[dev]"
.venv/bin/pytest tests/ -v
```

## Tech Stack

- **Core:** Go 1.25+ (goroutines, small binary, K8s/Docker precedent)
- **SDK:** Python 3.11+ (AI/ML ecosystem)
- **Protocol:** MCP² — JSON over HTTP
- **Storage:** Memory (dev) · SQLite (file) · PostgreSQL (production)
- **Observability:** Prometheus metrics (`GET /metrics`) + structured JSON logging
- **License:** Apache 2.0

## Roadmap

- [x] Foundation — Gateway, Registry, Router, Monitor
- [x] Differentiators — Orchestrator, Evaluator, Cost Controller, Org Manager
- [x] Knowledge Hub — Shared knowledge base + pgvector semantic search
- [x] HTTP Dispatch — Task execution via worker HTTP endpoints
- [x] Security — API key auth, worker tokens, SSRF protection, audit log
- [x] Docker — Multi-stage Dockerfile, Railway/Render/Fly deploy
- [x] Go SDK — Native Go workers (`sdk/go/`)
- [x] Persistent storage — SQLite + PostgreSQL with auto-migrations
- [x] SSE Streaming — Real-time task output streaming
- [x] Webhooks — At-least-once event delivery with HMAC-SHA256
- [x] Prometheus metrics — Full observability via `/metrics`
- [x] Dashboard — Web UI for monitoring
- [x] LLM Gateway — Multi-provider routing (OpenAI, Anthropic, Ollama), cost tracking, fallback
- [x] Prompt Registry — Versioned templates, variable interpolation, A/B testing
- [x] Agent Memory — Conversation history (sliding window) + long-term vector recall
- [x] TypeScript SDK — Native TypeScript workers + npm publish
- [x] CLI — `magic workers`, `magic tasks`, `magic submit`, `magic status`
- [x] Distributed tracing — W3C Trace Context propagation
- [x] Task DLQ — Dead letter queue for permanently failed tasks
- [x] Worker auto-discovery — UDP broadcast for local dev
- [x] Cluster mode — Leader election via PostgreSQL advisory locks
- [x] VitePress docs site
- [x] Docker Hub image
- [ ] SaaS managed platform

## License

Apache 2.0 — see [LICENSE](LICENSE).
