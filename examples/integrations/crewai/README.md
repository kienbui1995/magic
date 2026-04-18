# CrewAI + MagiC

Wrap an existing **CrewAI** crew as a MagiC worker — no rewrite, no glue code.

## Why it matters

You already have a CrewAI crew that works. You don't want to rewrite it just
to get production niceties. Drop it inside a `@worker.capability` handler and
MagiC's orchestrator now manages it — cost tracking, budget enforcement,
RBAC, policy controls, retries, audit logs — without touching any agent logic.

Your crew stays a crew. MagiC is the fleet manager around it.

## Architecture

```
    client (curl / SDK)
            │
            │  POST /api/v1/tasks  type=research_and_write
            ▼
    ┌────────────────────┐
    │   MagiC Gateway    │  routing, auth, cost, policy
    └─────────┬──────────┘
              │  task.assign (HTTP)
              ▼
    ┌────────────────────┐
    │   CrewAIWorker     │  this file — @worker.capability
    │                    │
    │   ┌────────────┐   │
    │   │ Researcher │   │   CrewAI Agent 1
    │   └─────┬──────┘   │
    │         ▼          │
    │   ┌────────────┐   │
    │   │   Writer   │   │   CrewAI Agent 2
    │   └────────────┘   │
    └────────────────────┘
```

## Run

### 1. Set up a virtualenv and install deps

```bash
cd examples/integrations/crewai
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
```

### 2. Configure environment

```bash
cp .env.example .env
# edit .env — set OPENAI_API_KEY, or switch to Ollama (see .env.example)
```

### 3. Start MagiC gateway (in another terminal)

```bash
cd core
go build ./cmd/magic
./magic serve         # listens on :8080
```

### 4. Run the worker

```bash
python main.py
# → registers with MagiC and serves on :9101
```

### 5. Submit a task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "research_and_write",
    "input": {"topic": "Retrieval-Augmented Generation in 2026"},
    "routing": {"required_capabilities": ["research_and_write"]},
    "contract": {"timeout_ms": 120000, "max_cost": 1.0}
  }'
```

Or from Python:

```python
import httpx

r = httpx.post("http://localhost:8080/api/v1/tasks", json={
    "type": "research_and_write",
    "input": {"topic": "Retrieval-Augmented Generation in 2026"},
    "routing": {"required_capabilities": ["research_and_write"]},
}, timeout=120)
print(r.json())
```

## What you get (for free)

| Feature | Provided by MagiC |
|---|---|
| Automatic retry on worker crash | Registry + router |
| Per-task cost tracking | CostCtrl |
| Daily / monthly budget caps | CostCtrl |
| RBAC (who can submit which task type) | OrgMgr |
| Policy engine (block dangerous topics, etc.) | Gateway middleware |
| Audit log (who ran what, when, output) | Audit |
| Fallback worker on failure | Router |
| Prometheus metrics on every run | Monitor |

## Local-only mode (no OpenAI bill)

Install Ollama, pull `llama3.2`, and set:

```env
OPENAI_API_KEY=ollama
OPENAI_API_BASE=http://localhost:11434/v1
CREWAI_LLM_MODEL=ollama/llama3.2
```

Everything else stays the same.

## Pattern

The pattern is small enough to memorise:

```python
from magic_ai_sdk import Worker
from crewai import Crew  # your existing crew

worker = Worker(name="MyCrew", endpoint="http://localhost:9101")

@worker.capability("my_crew_run")
def run_crew(**kwargs) -> dict:
    crew = build_my_existing_crew()      # <-- your code, unchanged
    result = crew.kickoff(inputs=kwargs)
    return {"result": str(result)}

worker.run("http://localhost:8080", port=9101)
```

That is the whole integration.
