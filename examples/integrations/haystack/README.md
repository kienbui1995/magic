# Haystack + MagiC

Wrap a production **Haystack 2.x** pipeline as a MagiC worker — keep the
pipeline graph, gain enterprise fleet controls.

## Why it matters

Haystack is built for production RAG: typed components, explicit DAG,
serialisable pipelines. Teams ship it to customers and live with it for
years. What Haystack does **not** own is the layer *around* the pipeline
— who's allowed to call it, what each call costs, which team's monthly
budget just blew up, whether the embedder has started returning 500s.

Drop your `Pipeline` into a `@worker.capability` handler and MagiC
adds cost tracking, budget caps, retries, RBAC, audit logs, and
Prometheus metrics on top.

**Haystack runs production pipelines. MagiC runs the production around
them — enterprise observability, not just a POC.**

## Architecture

```
    client (curl / SDK)
            │
            │  POST /api/v1/tasks  type=qa_pipeline
            ▼
    ┌────────────────────┐
    │   MagiC Gateway    │  auth, cost cap, policy
    └─────────┬──────────┘
              │  task.assign (HTTP)
              ▼
    ┌────────────────────────────────────────┐
    │   HaystackWorker (this file)           │
    │                                        │
    │   Pipeline:                            │
    │   ┌─────────────┐    ┌──────────────┐  │
    │   │ TextEmbedder│───►│  Retriever   │  │
    │   └─────────────┘    └──────┬───────┘  │
    │                             ▼          │
    │                     ┌──────────────┐   │
    │                     │ PromptBuilder│   │
    │                     └──────┬───────┘   │
    │                            ▼           │
    │                    ┌──────────────┐    │
    │                    │   Generator  │    │
    │                    └──────────────┘    │
    └────────────────────────────────────────┘
```

## Run

### 1. Set up a virtualenv and install deps

```bash
cd examples/integrations/haystack
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
# → registers with MagiC and serves on :9105
```

### 5. Submit a task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "qa_pipeline",
    "input": {"question": "How does MagiC retry webhook deliveries?"},
    "routing": {"required_capabilities": ["qa_pipeline"]},
    "contract": {"timeout_ms": 60000, "max_cost": 0.25}
  }'
```

Response shape:

```json
{
  "status": "done",
  "output": {
    "answer": "Webhook delivery is at-least-once. Failed deliveries are ...",
    "retrieved_docs": ["...", "...", "..."]
  }
}
```

## What you get (for free)

| Feature | Provided by MagiC |
|---|---|
| Per-pipeline-run cost tracking (embed + LLM) | CostCtrl |
| Daily / monthly budget caps per team | CostCtrl |
| RBAC — which team can hit which pipeline | OrgMgr |
| Retry on flaky generator / embedder | Router |
| Circuit breaker when upstream APIs degrade | Router |
| Audit log of every question + answer | Audit |
| Prometheus metrics per pipeline stage | Monitor |

## Local-only mode (no OpenAI bill)

Install the Ollama connector and swap components:

```bash
pip install ollama-haystack
```

```python
from haystack_integrations.components.generators.ollama import OllamaGenerator
from haystack_integrations.components.embedders.ollama import (
    OllamaDocumentEmbedder, OllamaTextEmbedder,
)
# Replace OpenAIGenerator / OpenAI*Embedder in main.py with these.
```

Everything else — the pipeline wiring, the MagiC decorator — is unchanged.

## Pattern

```python
from magic_ai_sdk import Worker
from haystack import Pipeline

pipeline = build_my_haystack_pipeline()        # <-- your existing DAG
worker = Worker(name="MyHaystack", endpoint="http://localhost:9105")

@worker.capability("qa_pipeline")
def qa(question: str) -> dict:
    out = pipeline.run({"text_embedder": {"text": question},
                        "prompt_builder": {"question": question}})
    return {"answer": out["generator"]["replies"][0]}

worker.run("http://localhost:8080", port=9105)
```

One decorator. Typed production pipeline inside; one clean capability outside.

## Disclaimer

Haystack 2.x component paths (`haystack.components.*`,
`haystack_integrations.*`) shift between minor versions. If an import
fails, pin `haystack-ai>=2.5,<2.9` or check the release notes for the
version you installed.
