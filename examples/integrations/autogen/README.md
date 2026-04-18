# AutoGen + MagiC

Expose a multi-agent **AutoGen** GroupChat as a single MagiC worker capability.

## Why it matters

AutoGen shines at multi-agent conversations: PMs debating engineers, critics
stress-testing specs, solvers arguing their way to a plan. But to the *caller*,
that internal debate is noise. MagiC lets you hide a 3-agent GroupChat behind
one clean capability — `product_spec_review(feature_idea)` — and exposes only
the final artifact (`spec` + a trimmed `discussion_summary`).

**Multi-agent complexity inside, one clean interface outside.** That is the
contract MagiC makes possible.

## Architecture

```
    client (curl / SDK)
            │
            │  POST /api/v1/tasks  type=product_spec_review
            ▼
    ┌────────────────────┐
    │   MagiC Gateway    │  auth, cost cap, timeout
    └─────────┬──────────┘
              │  task.assign (HTTP)
              ▼
    ┌──────────────────────────────────┐
    │   AutoGenWorker (this file)      │
    │                                  │
    │   ┌──────────────────────────┐   │
    │   │  GroupChat (round robin) │   │
    │   │  ├─ product_manager      │   │
    │   │  ├─ engineer             │   │
    │   │  └─ critic               │   │
    │   │  max_round = 5           │   │
    │   └──────────────────────────┘   │
    └──────────────────────────────────┘
```

## Run

### 1. Install

```bash
cd examples/integrations/autogen
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
```

### 2. Configure

```bash
cp .env.example .env
# edit .env — set OPENAI_API_KEY, or switch to Ollama
```

### 3. Start MagiC gateway

```bash
cd core
go build ./cmd/magic
./magic serve       # :8080
```

### 4. Run the worker

```bash
python main.py      # registers with MagiC and serves on :9103
```

### 5. Submit a task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "product_spec_review",
    "input": {"feature_idea": "A dashboard that shows AI-worker cost per team in real time"},
    "routing": {"required_capabilities": ["product_spec_review"]},
    "contract": {"timeout_ms": 180000, "max_cost": 1.50}
  }'
```

Response shape:

```json
{
  "status": "done",
  "output": {
    "spec": "Problem statement: ...\nUser stories: ...\nMetrics: ...",
    "discussion_summary": "[product_manager]: ...\n\n[engineer]: ...\n\n[critic]: ...",
    "rounds_limit": 5
  }
}
```

## What you get (for free)

| Feature | Provided by MagiC |
|---|---|
| Hard timeout on runaway GroupChats | Gateway contract |
| Per-task `max_cost` (caps LLM spend on a debate) | CostCtrl |
| Budget alerts when the team blows their monthly cap | CostCtrl |
| RBAC — only PM role can run spec reviews | OrgMgr |
| Audit log of every spec produced | Audit |
| Retry on transient failures | Router |
| Prometheus metrics on GroupChat runtime | Monitor |

## Local-only mode

Use Ollama — AutoGen honours `base_url`:

```env
OPENAI_API_KEY=ollama
OPENAI_API_BASE=http://localhost:11434/v1
AUTOGEN_LLM_MODEL=llama3.2
```

Group chats with weaker local models tend to ramble; keep `AUTOGEN_MAX_ROUNDS`
small (3–4) and temperature low.

## Pattern

```python
from magic_ai_sdk import Worker
import autogen  # your existing agents

worker = Worker(name="MyAutoGen", endpoint="http://localhost:9103")

@worker.capability("product_spec_review")
def product_spec_review(feature_idea: str) -> dict:
    group = build_my_group_chat()        # <-- unchanged
    manager = autogen.GroupChatManager(groupchat=group, llm_config=cfg)
    user_proxy.initiate_chat(manager, message=feature_idea)
    return {"spec": summarize(group.messages)}

worker.run("http://localhost:8080", port=9103)
```

One decorator. Multi-agent system inside; single capability outside.
