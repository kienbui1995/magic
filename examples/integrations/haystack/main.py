"""
Haystack 2.x + MagiC integration example.

Wraps a Haystack QA pipeline (text embedder → in-memory retriever → prompt
builder → OpenAI generator) as a single MagiC worker capability. Haystack
owns the retrieval/generation DAG; MagiC owns fleet management.

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
from haystack import Document, Pipeline
from haystack.components.builders import PromptBuilder
from haystack.components.embedders import (
    OpenAIDocumentEmbedder,
    OpenAITextEmbedder,
)
from haystack.components.generators import OpenAIGenerator
from haystack.components.retrievers.in_memory import InMemoryEmbeddingRetriever
from haystack.document_stores.in_memory import InMemoryDocumentStore
from magic_ai_sdk import Worker

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("haystack-worker")

MAGIC_URL = os.getenv("MAGIC_GATEWAY_URL", "http://localhost:8080")
WORKER_PORT = int(os.getenv("WORKER_PORT", "9105"))
LLM_MODEL = os.getenv("HAYSTACK_LLM_MODEL", "gpt-4o-mini")
EMBED_MODEL = os.getenv("HAYSTACK_EMBED_MODEL", "text-embedding-3-small")


# ── Sample docs (MagiC factsheet) ─────────────────────────────────────────
_DOCS: list[str] = [
    "MagiC Gateway is the HTTP entry point. It authenticates callers, applies "
    "policy, and hands accepted tasks to the router.",
    "The Orchestrator executes workflows as DAGs. Each node is a capability "
    "invocation; edges carry outputs forward and support conditional fan-out.",
    "CostCtrl records spend per task and enforces budgets. When a cap is hit "
    "the event bus emits budget.exceeded and the router stops dispatching.",
    "Webhook delivery is at-least-once. Failed deliveries are queued and "
    "retried with 30s → 5m → 30m → 2h → 8h backoff; max five attempts.",
    "Knowledge Hub supports pgvector semantic search. Default embedding "
    "dimension is 1536; configurable via MAGIC_PGVECTOR_DIM.",
]

_PROMPT = """Answer the question using only the context.
Context:
{% for doc in documents %}- {{ doc.content }}
{% endfor %}
Question: {{ question }}
Answer:"""


def _build_pipeline() -> Pipeline:
    """Build the 4-stage QA pipeline and index the sample docs."""
    store = InMemoryDocumentStore()
    docs = [Document(content=t) for t in _DOCS]
    doc_embedder = OpenAIDocumentEmbedder(model=EMBED_MODEL)
    embedded = doc_embedder.run(documents=docs)["documents"]
    store.write_documents(embedded)

    pipe = Pipeline()
    pipe.add_component("text_embedder", OpenAITextEmbedder(model=EMBED_MODEL))
    pipe.add_component("retriever", InMemoryEmbeddingRetriever(document_store=store, top_k=3))
    pipe.add_component("prompt_builder", PromptBuilder(template=_PROMPT))
    pipe.add_component("generator", OpenAIGenerator(model=LLM_MODEL))

    pipe.connect("text_embedder.embedding", "retriever.query_embedding")
    pipe.connect("retriever.documents", "prompt_builder.documents")
    pipe.connect("prompt_builder.prompt", "generator.prompt")
    return pipe


_pipe: Pipeline | None = None


def _pipeline_singleton() -> Pipeline:
    global _pipe
    if _pipe is None:
        _pipe = _build_pipeline()
    return _pipe


# ── MagiC worker ──────────────────────────────────────────────────────────
worker = Worker(
    name="HaystackWorker",
    endpoint=f"http://localhost:{WORKER_PORT}",
    worker_token=os.getenv("MAGIC_WORKER_TOKEN", ""),
)


@worker.capability(
    name="qa_pipeline",
    description="Answer a question using a Haystack RAG pipeline. Args: question (str).",
    est_cost=0.015,
)
def qa_pipeline(question: str) -> dict:
    """Run the full embed → retrieve → prompt → generate pipeline once."""
    log.info("Haystack QA — question=%r", question)
    pipe = _pipeline_singleton()
    out = pipe.run({
        "text_embedder": {"text": question},
        "prompt_builder": {"question": question},
    }, include_outputs_from={"retriever"})
    replies = out.get("generator", {}).get("replies", [])
    retrieved = [d.content for d in out.get("retriever", {}).get("documents", [])]
    return {"answer": replies[0] if replies else "", "retrieved_docs": retrieved}


if __name__ == "__main__":
    log.info("Starting HaystackWorker → MagiC at %s", MAGIC_URL)
    worker.run(MAGIC_URL, port=WORKER_PORT)
