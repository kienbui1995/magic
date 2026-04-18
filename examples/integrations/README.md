# Integration Examples — Make Your Existing Agents MagiC Workers

> **MagiC doesn't replace your agent framework. It manages the fleet around it.**

Already have a CrewAI crew, a LangChain agent, or an AutoGen GroupChat that
works? Don't rewrite it. Wrap it. In ~5 lines you get cost tracking, budget
caps, RBAC, retries, policy enforcement, audit logs, and Prometheus metrics —
without touching the agent logic you already trust.

## Examples in this folder

| Framework | Example | What you get |
|---|---|---|
| [CrewAI](./crewai) | 2-agent crew (researcher + writer) wrapped as one capability `research_and_write` | Cost tracking, RBAC, audit, retries for CrewAI kickoffs |
| [LangChain](./langchain) | Tool-calling agent (calculator + DuckDuckGo) as capability `qa_with_tools` | Retry + circuit breaker on flaky LLM calls, per-task cost cap, tool-use metrics |
| [AutoGen](./autogen) | 3-agent GroupChat (PM → Engineer → Critic) as capability `product_spec_review` | Timeout on runaway debates, budget protection, clean single-capability interface |

## The wrapper pattern

Every integration follows the same shape — because there's really only one
shape to follow:

```python
from magic_ai_sdk import Worker

# 1. Build your existing agent however you already do.
my_agent = build_my_existing_agent()

# 2. Create a MagiC worker.
worker = Worker(name="MyBot", endpoint="http://localhost:9100")

# 3. Expose agent invocations as capabilities.
@worker.capability("do_the_thing", description="What this does")
def do_the_thing(**inputs) -> dict:
    result = my_agent.run(**inputs)   # <-- your framework, unchanged
    return {"result": result}

# 4. Register + serve.
worker.run("http://localhost:8080", port=9100)
```

That is the whole concept. Every file under this directory is a concrete
version of that skeleton.

## When to use MagiC

**Good fit:**
- You have **multiple agents** and need routing, fallback, fleet health.
- You care about **cost and budgets** — per-task, per-team, per-day.
- You need **RBAC, audit, policy** for enterprise / regulated environments.
- You want **observability** (Prometheus, structured logs) without bolting it on per project.
- You want **orchestration** — chaining capabilities from different frameworks in one workflow DAG.

**Probably overkill:**
- A single script with one agent run — just run the script.
- Pure experimentation — stay in a notebook.
- Agents that don't hit external APIs (no cost, nothing to govern).

## Prerequisites (all examples)

1. MagiC gateway running locally:
   ```bash
   cd core && go build ./cmd/magic && ./magic serve
   ```
2. Python 3.11+ and a virtualenv per example (each has its own deps).
3. An LLM provider — OpenAI by default, Ollama works offline (each `.env.example`
   documents both).

## Contribute

Have a framework not listed here? **Send a PR.**

Good candidates next:
- **LlamaIndex** — expose a query engine over a knowledge base as a capability.
- **Haystack** — wrap a RAG pipeline.
- **DSPy** — expose a compiled DSPy program.
- **Smolagents** — expose a code-executing agent under a safety-constrained capability.
- **Custom FastAPI / Flask agents** — wrap an internal microservice you already run.

Checklist for a new example:
- Self-contained `examples/integrations/<framework>/` folder.
- `main.py` (~100 lines), `requirements.txt`, `.env.example`, `README.md`.
- Same README structure as the existing three (Why it matters / Architecture /
  Run / What you get / Local-only mode / Pattern).
- Runnable against a fresh `magic serve` on localhost.
- OpenAI default, Ollama fallback documented.
- No hardcoded secrets.
