# MagiC

> Don't build another AI. Manage the ones you have.

MagiC is an open-source framework for managing fleets of AI workers. Think **Kubernetes for AI agents** — it doesn't build agents, it manages any agents built with any tool (CrewAI, LangChain, custom bots, etc.) through an open protocol.

```
         You (CEO)
          |
     MagiC Server
    /    |    |    \
ContentBot  SEOBot  LeadBot  CodeBot
(Python)   (Node)  (Python)  (Go)
```

## Quick Start (< 5 minutes)

**1. Start the MagiC server**

```bash
git clone https://github.com/kienbm/magic-claw.git
cd magic-claw/core
go build -o ../bin/magic ./cmd/magic
../bin/magic serve
```

**2. Create a worker in Python (10 lines)**

```bash
pip install magic-claw
```

```python
from magic_claw import Worker

worker = Worker(name="HelloBot", endpoint="http://localhost:9000")

@worker.capability("greeting", description="Says hello to anyone")
def greet(name: str) -> str:
    return f"Hello, {name}! I'm managed by MagiC."

worker.register("http://localhost:8080")
worker.serve()
```

**3. Submit a task**

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "greeting",
    "input": {"name": "World"},
    "routing": {"strategy": "best_match", "required_capabilities": ["greeting"]},
    "contract": {"timeout_ms": 30000, "max_cost": 1.0}
  }'
```

MagiC finds the best worker, dispatches the task via HTTP, and returns the result. That's it.

## Why MagiC?

| Without MagiC | With MagiC |
|---|---|
| Each AI agent is a standalone script | Workers join an organization, get tasks assigned |
| No visibility into what agents are doing | Real-time monitoring, structured JSON logging |
| Manual coordination between agents | Automatic routing (best match, cheapest, fastest) |
| No cost control — surprise bills | Budget alerts at 80%, auto-pause at 100% |
| Agents can't collaborate | Workers delegate tasks to each other via protocol |
| Locked into one framework (CrewAI OR LangChain) | Any worker, any framework, any language |

### vs. Other Frameworks

| Feature | CrewAI | AutoGen | LangGraph | **MagiC** |
|---|---|---|---|---|
| Approach | Build agents | Build agents | Build graphs | **Manage any agent** |
| Protocol | Closed | Closed | Closed | **Open (MCP²)** |
| Language lock-in | Python | Python | Python | **Any (Go core, Python/Go SDK)** |
| Cost control | No | No | No | **Budget alerts + auto-pause** |
| Multi-step workflows | Flow | Event-driven | Graph | **DAG orchestrator** |
| Worker discovery | No | No | No | **Capability-based routing** |
| Organization model | Crew | GroupChat | Graph | **Org → Teams → Workers** |

**MagiC doesn't replace CrewAI/LangChain — it manages them.** Your CrewAI agent becomes a MagiC worker. Your LangChain chain becomes a MagiC worker. They join the same organization and work together.

## Architecture

```
                ┌──────────────────────────────────────────────┐
                │              MagiC Core (Go)                 │
                ├──────────────────────────────────────────────┤
  HTTP Request ─►  Gateway (auth, rate limit, request ID)      │
                │    │                                         │
                │    ▼                                         │
                │  Router ──► Registry (find best worker)      │
                │    │          │                               │
                │    ▼          ▼                               │
                │  Dispatcher ──► Worker A (HTTP POST)         │
                │    │              Worker B                    │
                │    │              Worker C                    │
                │    ▼                                         │
                │  Orchestrator (multi-step DAG workflows)     │
                │  Evaluator (output quality validation)       │
                │  Cost Controller (budget tracking)           │
                │  Org Manager (teams, policies)               │
                │  Knowledge Hub (shared context)              │
                │  Monitor (events, metrics, logging)          │
                └──────────────────────────────────────────────┘
```

### 9 Modules

| Tier | Module | Purpose |
|------|--------|---------|
| **Core** | Gateway | HTTP entry point, middleware, routing |
| **Core** | Registry | Worker registration, heartbeat, health checks |
| **Core** | Router | Task routing (best_match, cheapest, round_robin) |
| **Core** | Monitor | Event bus, structured JSON logging, metrics |
| **Diff** | Orchestrator | Multi-step workflow DAG execution |
| **Diff** | Evaluator | JSON schema validation for task output |
| **Diff** | Cost Controller | Budget tracking, alerts, auto-pause |
| **Diff** | Org Manager | Team CRUD, worker-to-team assignment |
| **Bonus** | Knowledge Hub | Shared knowledge base with search |

Plus: **Dispatcher** — HTTP task dispatch to worker endpoints.

### Protocol: MCP² (MagiC Protocol)

Transport-agnostic JSON messages. 14 message types:

- **Worker lifecycle:** register, heartbeat, deregister, update_capabilities
- **Task lifecycle:** assign, accept, reject, progress, complete, fail
- **Collaboration:** delegate, broadcast
- **Direct channel:** open_channel, close_channel

```json
{
  "protocol": "mcp2",
  "version": "1.0",
  "type": "task.assign",
  "id": "msg_abc123",
  "timestamp": "2026-03-16T10:00:00Z",
  "source": "org_magic",
  "target": "worker_001",
  "payload": { "task_id": "task_001", "task_type": "greeting", "input": {"name": "World"} }
}
```

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/v1/workers/register` | Register a worker |
| `POST` | `/api/v1/workers/heartbeat` | Worker heartbeat |
| `GET` | `/api/v1/workers` | List workers |
| `POST` | `/api/v1/tasks` | Submit a task (auto-routes + dispatches) |
| `POST` | `/api/v1/workflows` | Submit a multi-step workflow |
| `GET` | `/api/v1/workflows` | List workflows |
| `POST` | `/api/v1/teams` | Create a team |
| `GET` | `/api/v1/teams` | List teams |
| `GET` | `/api/v1/costs` | Organization cost report |
| `POST` | `/api/v1/knowledge` | Add knowledge entry |
| `GET` | `/api/v1/knowledge?q=<query>` | Search knowledge |
| `GET` | `/api/v1/metrics` | System metrics |

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
  content    leads       ← parallel
     |         |
    seo        |
      \       /
     outreach            ← waits for both branches
```

Failure handling per step: `retry`, `skip`, `abort`, `reassign`.

## Project Structure

```
magic-claw/
├── core/                           # Go server (9 modules)
│   ├── cmd/magic/main.go           # CLI entrypoint
│   └── internal/
│       ├── protocol/               # MCP² types & messages
│       ├── store/                   # Storage interface + in-memory
│       ├── events/                  # Event bus (pub/sub)
│       ├── gateway/                 # HTTP server + middleware
│       ├── registry/               # Worker registration
│       ├── router/                 # Task routing strategies
│       ├── dispatcher/             # HTTP dispatch to workers
│       ├── monitor/                # Logging + metrics
│       ├── orchestrator/           # Workflow DAG execution
│       ├── evaluator/              # Output validation
│       ├── costctrl/               # Budget tracking
│       ├── orgmgr/                 # Team management
│       └── knowledge/              # Knowledge hub
├── sdk/python/                     # Python SDK (pip install magic-claw)
│   ├── magic_claw/
│   │   ├── worker.py               # Worker class
│   │   ├── client.py               # HTTP client
│   │   └── decorators.py           # @capability decorator
│   └── tests/
├── examples/
│   └── hello-worker/main.py        # 10-line example
└── docs/
    └── superpowers/
        └── specs/                  # Design specification
```

## Development

```bash
# Build
cd core && go build -o ../bin/magic ./cmd/magic

# Run tests
cd core && go test ./... -v

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

- **Core:** Go 1.22+ (goroutines, small binary, K8s/Docker precedent)
- **SDK:** Python 3.11+ (AI/ML ecosystem)
- **Protocol:** JSON over HTTP (WebSocket/gRPC planned)
- **Storage:** In-memory (SQLite/PostgreSQL planned)
- **License:** Apache 2.0

## Roadmap

- [x] Foundation — Gateway, Registry, Router, Monitor
- [x] Differentiators — Orchestrator, Evaluator, Cost Controller, Org Manager
- [x] Knowledge Hub — Shared knowledge base
- [x] HTTP Dispatch — Actual task execution via worker endpoints
- [ ] Docker — `docker run magic-claw`
- [ ] Go SDK — Native Go workers
- [ ] Persistent storage — SQLite/PostgreSQL
- [ ] WebSocket — Real-time worker communication
- [ ] Dashboard — Web UI for monitoring
- [ ] Authentication — API key / JWT
- [ ] Rate limiting — Per-user/team throttling

## License

Apache 2.0 — see [LICENSE](LICENSE).
