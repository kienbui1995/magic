# LangChain + MagiC

Expose a **LangChain** tool-calling agent as a MagiC worker capability.

## Why it matters

LangChain is fantastic for composing agents and tools, but on its own it
doesn't give you production scaffolding: retries on flaky LLM calls, per-tenant
cost caps, budget alerts, RBAC, or a circuit breaker when your OpenAI quota
burns. MagiC wraps all that around the agent you already wrote. Your
`AgentExecutor` is the same; the surrounding infrastructure is just better.

LangChain agents + MagiC orchestration = a production deployment path.

## Architecture

```
    client (curl / SDK / another worker)
            │
            │  POST /api/v1/tasks  type=qa_with_tools
            ▼
    ┌────────────────────┐
    │   MagiC Gateway    │  retry, circuit break, cost limit
    └─────────┬──────────┘
              │  task.assign (HTTP)
              ▼
    ┌─────────────────────────────────┐
    │   LangChainWorker  (this file)  │
    │                                 │
    │   AgentExecutor                 │
    │   ├─ calculator tool            │
    │   └─ web_search (DuckDuckGo)    │
    │                                 │
    │   ChatOpenAI(gpt-4o-mini)       │
    └─────────────────────────────────┘
```

## Run

### 1. Install

```bash
cd examples/integrations/langchain
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
python main.py      # registers with MagiC and serves on :9102
```

### 5. Submit a task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "qa_with_tools",
    "input": {"question": "What is 17 * 23, and what is the capital of Vietnam?"},
    "routing": {"required_capabilities": ["qa_with_tools"]},
    "contract": {"timeout_ms": 60000, "max_cost": 0.50}
  }'
```

Expected response shape:

```json
{
  "id": "tsk_...",
  "status": "done",
  "output": {
    "answer": "17 * 23 = 391. The capital of Vietnam is Hanoi.",
    "steps": [
      {"tool": "calculator", "input": {"expression": "17 * 23"}, "output": "391"},
      {"tool": "web_search", "input": {"query": "capital of Vietnam"}, "output": "..."}
    ]
  }
}
```

## What you get (for free)

| Feature | Provided by MagiC |
|---|---|
| Retry on 5xx from LLM provider | Router |
| Circuit breaker when worker crashes repeatedly | Registry |
| Per-task `max_cost` enforcement | CostCtrl |
| Daily cost cap per worker | CostCtrl |
| RBAC on who can call `qa_with_tools` | OrgMgr |
| Audit trail of every question and answer | Audit |
| Prometheus metrics on tool-use patterns | Monitor |

## Local-only mode

Use Ollama:

```env
OPENAI_API_KEY=ollama
OPENAI_API_BASE=http://localhost:11434/v1
LANGCHAIN_LLM_MODEL=llama3.2
```

Note: tool-calling quality varies by local model. `llama3.2` and `qwen2.5`
handle function calls reasonably; small models struggle.

## Pattern

```python
from magic_ai_sdk import Worker
from langchain.agents import AgentExecutor  # your existing executor

worker = Worker(name="MyLCWorker", endpoint="http://localhost:9102")
executor = build_my_agent_executor()         # <-- unchanged

@worker.capability("qa_with_tools")
def qa_with_tools(question: str) -> dict:
    result = executor.invoke({"input": question})
    return {"answer": result["output"]}

worker.run("http://localhost:8080", port=9102)
```

The entire integration is one decorator and one call.
