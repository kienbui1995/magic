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

### Option A: From source

**Prerequisites:** Go 1.22+, Python 3.11+

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

### Option B: Docker

```bash
docker build -t magic .
docker run -p 8080:8080 magic
```

### Create your first worker

Save as `worker.py`:

```python
from magic_claw import Worker

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
| Language lock-in | Python | Python | Python | **Any (Go core, Python SDK)** |
| Cost control | No | No | No | **Budget alerts + auto-pause** |
| Multi-step workflows | Flow | Event-driven | Graph | **DAG orchestrator** |
| Worker discovery | No | No | No | **Capability-based routing** |
| Organization model | Crew | GroupChat | Graph | **Org > Teams > Workers** |

**MagiC doesn't replace CrewAI/LangChain — it manages them.** Your CrewAI agent becomes a MagiC worker. Your LangChain chain becomes a MagiC worker. They join the same organization and work together.

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
| `MAGIC_API_KEY` | _(empty = no auth)_ | API key for authentication |
| `MAGIC_CORS_ORIGIN` | `*` | Allowed CORS origin |

## Project Structure

```
magic/
├── core/                           # Go server (9 modules)
│   ├── cmd/magic/main.go           # CLI entrypoint
│   └── internal/
│       ├── protocol/               # MCP² types & messages
│       ├── store/                  # Storage interface + in-memory
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
│       └── knowledge/              # Knowledge hub
├── sdk/python/                     # Python SDK (pip install magic-claw)
│   ├── magic_claw/
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

- **Core:** Go 1.22+ (goroutines, small binary, K8s/Docker precedent)
- **SDK:** Python 3.11+ (AI/ML ecosystem)
- **Protocol:** MCP² — JSON over HTTP (WebSocket/gRPC planned)
- **Storage:** In-memory (SQLite/PostgreSQL planned)
- **License:** Apache 2.0

## Roadmap

- [x] Foundation — Gateway, Registry, Router, Monitor
- [x] Differentiators — Orchestrator, Evaluator, Cost Controller, Org Manager
- [x] Knowledge Hub — Shared knowledge base
- [x] HTTP Dispatch — Actual task execution via worker endpoints
- [x] Security — API key auth, SSRF protection, body limits
- [x] Docker — Multi-stage Dockerfile
- [ ] Go SDK — Native Go workers
- [ ] Persistent storage — SQLite/PostgreSQL
- [ ] WebSocket — Real-time worker communication
- [ ] Dashboard — Web UI for monitoring

## License

Apache 2.0 — see [LICENSE](LICENSE).
