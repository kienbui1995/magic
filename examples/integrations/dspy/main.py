"""
DSPy + MagiC integration example.

Wraps a DSPy ChainOfThought program (intent / sentiment classifier) as a
MagiC worker capability. DSPy owns the prompt program and its signatures;
MagiC owns routing, cost tracking, and observability — a natural fit for
A/B testing different DSPy-compiled variants behind the same capability name.

Usage::

    cp .env.example .env              # fill in OPENAI_API_KEY
    pip install -r requirements.txt
    # in another terminal: cd core && go build ./cmd/magic && ./magic serve
    python main.py
"""

from __future__ import annotations

import logging
import os

import dspy
from dotenv import load_dotenv
from magic_ai_sdk import Worker

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("dspy-worker")

MAGIC_URL = os.getenv("MAGIC_GATEWAY_URL", "http://localhost:8080")
WORKER_PORT = int(os.getenv("WORKER_PORT", "9106"))
LLM_MODEL = os.getenv("DSPY_LLM_MODEL", "openai/gpt-4o-mini")
# For Ollama: DSPY_LLM_MODEL=ollama_chat/llama3.2 and set OPENAI_API_BASE
# to the Ollama OpenAI-compat URL.

# Configure the global DSPy LM. DSPy uses a LiteLLM-style model string.
dspy.configure(lm=dspy.LM(model=LLM_MODEL, temperature=0.0, max_tokens=200))


# ── DSPy signature + program ──────────────────────────────────────────────
class ClassifyIntent(dspy.Signature):
    """Classify the sentiment/intent of a short text.

    Produce one label from {positive, negative, neutral} and a brief reason.
    """

    text: str = dspy.InputField(desc="Short text snippet from a user.")
    label: str = dspy.OutputField(desc="One of: positive, negative, neutral.")
    reasoning: str = dspy.OutputField(desc="One sentence justifying the label.")


class IntentClassifier(dspy.Module):
    """ChainOfThought wrapper over the ClassifyIntent signature."""

    def __init__(self) -> None:
        super().__init__()
        self.classify = dspy.ChainOfThought(ClassifyIntent)

    def forward(self, text: str) -> dspy.Prediction:
        return self.classify(text=text)


_program: IntentClassifier | None = None


def _program_singleton() -> IntentClassifier:
    global _program
    if _program is None:
        # For a compiled variant, call BootstrapFewShot(...).compile(program, trainset=...)
        # here and cache it. Zero-shot works out of the box for the demo.
        _program = IntentClassifier()
    return _program


_ALLOWED = {"positive", "negative", "neutral"}


def _normalize_label(raw: str) -> str:
    """Defensive normalisation — LLMs occasionally return extra punctuation."""
    lowered = (raw or "").strip().lower().strip(".").strip()
    return lowered if lowered in _ALLOWED else "neutral"


# ── MagiC worker ──────────────────────────────────────────────────────────
worker = Worker(
    name="DSPyWorker",
    endpoint=f"http://localhost:{WORKER_PORT}",
    worker_token=os.getenv("MAGIC_WORKER_TOKEN", ""),
)


@worker.capability(
    name="classify_intent",
    description="Classify a short text as positive / negative / neutral using a DSPy ChainOfThought program. Args: text (str).",
    est_cost=0.005,
)
def classify_intent(text: str) -> dict:
    """Run the DSPy program once and return label + one-sentence reasoning."""
    log.info("DSPy classify — len=%d model=%s", len(text or ""), LLM_MODEL)
    prediction = _program_singleton()(text=text)
    return {
        "label": _normalize_label(getattr(prediction, "label", "")),
        "reasoning": str(getattr(prediction, "reasoning", "")).strip(),
        "model": LLM_MODEL,
    }


if __name__ == "__main__":
    log.info("Starting DSPyWorker → MagiC at %s", MAGIC_URL)
    worker.run(MAGIC_URL, port=WORKER_PORT)
