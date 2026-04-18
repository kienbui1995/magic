"""
LlamaIndex + MagiC integration example.

Wraps a LlamaIndex RAG query engine (VectorStoreIndex over a small in-memory
document set) as a MagiC worker capability. LlamaIndex owns retrieval and
synthesis; MagiC owns routing, cost tracking, budgets, RBAC, and audit.

Usage::

    cp .env.example .env              # fill in OPENAI_API_KEY
    pip install -r requirements.txt
    # in another terminal: cd core && go build ./cmd/magic && ./magic serve
    python main.py
"""

from __future__ import annotations

import logging
import os

from dotenv import load_dotenv
from llama_index.core import Document, Settings, VectorStoreIndex
from llama_index.embeddings.openai import OpenAIEmbedding
from llama_index.llms.openai import OpenAI
from magic_ai_sdk import Worker

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("llamaindex-worker")

MAGIC_URL = os.getenv("MAGIC_GATEWAY_URL", "http://localhost:8080")
WORKER_PORT = int(os.getenv("WORKER_PORT", "9104"))
LLM_MODEL = os.getenv("LLAMAINDEX_LLM_MODEL", "gpt-4o-mini")
EMBED_MODEL = os.getenv("LLAMAINDEX_EMBED_MODEL", "text-embedding-3-small")
# For Ollama: OPENAI_API_KEY=ollama, OPENAI_API_BASE=http://localhost:11434/v1,
# LLAMAINDEX_LLM_MODEL=llama3.2, and swap embeddings to a local provider.


# ── Sample knowledge base (MagiC documentation snippets) ──────────────────
_DOCS: list[str] = [
    "MagiC is an open-source framework for managing fleets of AI workers — "
    "think 'Kubernetes for AI agents'. It does not build agents; it routes, "
    "governs, and observes agents built in any framework.",
    "A MagiC worker is any HTTP server that registers capabilities with the "
    "gateway. Workers receive task.assign messages and return a result. The "
    "MagiC Protocol (MCP²) is transport-agnostic JSON.",
    "The router supports three strategies: best_match (default, scores on "
    "capabilities + load), round_robin (fair distribution), and cheapest "
    "(lowest est_cost_per_call wins). Strategy is configurable per task.",
    "Cost Controller records every task's LLM spend and enforces daily / "
    "monthly budgets per team. Exceeding a hard cap raises budget.exceeded "
    "on the event bus; the orchestrator will refuse further dispatches.",
    "Knowledge Hub stores shared facts with pgvector-backed semantic search. "
    "Workers publish knowledge.added events; other workers can retrieve "
    "relevant context before acting. Embeddings default to 1536-dim.",
]


def _build_query_engine():
    """Build a LlamaIndex RAG query engine over the inline document set."""
    Settings.llm = OpenAI(model=LLM_MODEL, temperature=0)
    Settings.embed_model = OpenAIEmbedding(model=EMBED_MODEL)
    documents = [Document(text=t, metadata={"doc_id": f"magic-doc-{i}"}) for i, t in enumerate(_DOCS)]
    index = VectorStoreIndex.from_documents(documents)
    return index


_index: VectorStoreIndex | None = None


def _index_singleton() -> VectorStoreIndex:
    global _index
    if _index is None:
        _index = _build_query_engine()
    return _index


# ── MagiC worker ──────────────────────────────────────────────────────────
worker = Worker(
    name="LlamaIndexWorker",
    endpoint=f"http://localhost:{WORKER_PORT}",
    worker_token=os.getenv("MAGIC_WORKER_TOKEN", ""),
)


@worker.capability(
    name="rag_query",
    description="Answer a question using a LlamaIndex RAG query engine. Args: query (str), top_k (int).",
    est_cost=0.01,
)
def rag_query(query: str, top_k: int = 3) -> dict:
    """Run a retrieve-then-generate pass and return the answer with source snippets."""
    log.info("LlamaIndex RAG — query=%r top_k=%d", query, top_k)
    engine = _index_singleton().as_query_engine(similarity_top_k=top_k)
    response = engine.query(query)
    sources = [str(n.node.get_content())[:300] for n in getattr(response, "source_nodes", [])]
    return {"answer": str(response), "sources": sources, "top_k": top_k}


if __name__ == "__main__":
    log.info("Starting LlamaIndexWorker → MagiC at %s", MAGIC_URL)
    worker.run(MAGIC_URL, port=WORKER_PORT)
