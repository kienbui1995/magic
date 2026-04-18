# LlamaIndex + MagiC

Wrap an existing **LlamaIndex** RAG query engine as a MagiC worker — no
rewrite, no retrieval-glue duplication.

## Why it matters

LlamaIndex gives you best-in-class retrieval and response synthesis. What
it does not give you is fleet management: who is allowed to query, what
each query costs, which team's budget it hits, whether the retriever has
silently broken in production.

Drop your query engine into a `@worker.capability` handler and MagiC adds
cost tracking, budget caps, retries, RBAC, audit logs, and Prometheus
metrics — without touching your index pipeline.

**LlamaIndex retrieves. MagiC governs.**

## Architecture

```
    client (curl / SDK)
            │
            │  POST /api/v1/tasks  type=rag_query
            ▼
    ┌────────────────────┐
    │   MagiC Gateway    │  routing, auth, cost cap, policy
    └─────────┬──────────┘
              │  task.assign (HTTP)
              ▼
    ┌──────────────────────────────────┐
    │   LlamaIndexWorker (this file)   │
    │                                  │
    │   ┌──────────────────────────┐   │
    │   │  VectorStoreIndex        │   │
    │   │   ├─ OpenAIEmbedding     │   │
    │   │   ├─ similarity_top_k=3  │   │
    │   │   └─ OpenAI synth (LLM)  │   │
    │   └──────────────────────────┘   │
    └──────────────────────────────────┘
```

## Run

### 1. Set up a virtualenv and install deps

```bash
cd examples/integrations/llamaindex
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
```

### 2. Configure environment

```bash
cp .env.example .env
# edit .env — set OPENAI_API_KEY, or switch to local models (see .env.example)
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
# → registers with MagiC and serves on :9104
```

### 5. Submit a task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "rag_query",
    "input": {"query": "How does MagiC handle budgets?", "top_k": 3},
    "routing": {"required_capabilities": ["rag_query"]},
    "contract": {"timeout_ms": 60000, "max_cost": 0.25}
  }'
```

Response shape:

```json
{
  "status": "done",
  "output": {
    "answer": "...",
    "sources": ["...", "..."],
    "top_k": 3
  }
}
```

## What you get (for free)

| Feature | Provided by MagiC |
|---|---|
| Per-query cost tracking (LLM + embeddings) | CostCtrl |
| Daily / monthly budget caps per team | CostCtrl |
| RBAC — who can hit which knowledge base | OrgMgr |
| Automatic retry on flaky embedding APIs | Router |
| Audit log of every query + answer | Audit |
| Fallback worker on failure | Router |
| Prometheus metrics on query latency | Monitor |
| SSE streaming of long-running queries | Gateway |

## Local-only mode (no OpenAI bill)

LlamaIndex supports Ollama via the community package:

```bash
pip install llama-index-llms-ollama llama-index-embeddings-ollama
```

Then edit `main.py` to swap `OpenAI` / `OpenAIEmbedding` for their Ollama
equivalents. See the `llama_index.llms.ollama` docs for the exact class
names (they track fast). Example stubs:

```python
from llama_index.llms.ollama import Ollama
from llama_index.embeddings.ollama import OllamaEmbedding
Settings.llm = Ollama(model="llama3.2", request_timeout=60.0)
Settings.embed_model = OllamaEmbedding(model_name="nomic-embed-text")
```

## Pattern

```python
from magic_ai_sdk import Worker
from llama_index.core import VectorStoreIndex, Document

worker = Worker(name="MyRAG", endpoint="http://localhost:9104")
index = VectorStoreIndex.from_documents([Document(text=t) for t in my_docs])

@worker.capability("rag_query")
def rag_query(query: str, top_k: int = 3) -> dict:
    engine = index.as_query_engine(similarity_top_k=top_k)   # <-- your logic
    response = engine.query(query)
    return {"answer": str(response),
            "sources": [str(n.node.get_content()) for n in response.source_nodes]}

worker.run("http://localhost:8080", port=9104)
```

That is the whole integration.

## Disclaimer

LlamaIndex's Python API has been evolving quickly; class paths
(`llama_index.core` vs `llama_index.llms.openai`) can drift between
minor releases. If an import fails, pin `llama-index>=0.12,<0.13` or
check the official release notes for the version you installed.
