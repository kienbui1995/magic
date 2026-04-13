"""
OpenClaw → MagiC adapter.

Bridges an OpenClaw Gateway into MagiC as a worker so MagiC can
route tasks to it, track costs, and coordinate it alongside other agents.

Usage:
    OPENCLAW_URL=http://127.0.0.1:18789 \
    OPENCLAW_TOKEN=your-token \
    python openclaw_worker.py
"""

import os
import httpx
from magic_ai_sdk import Worker

OPENCLAW_URL = os.environ.get("OPENCLAW_URL", "http://127.0.0.1:18789")
OPENCLAW_TOKEN = os.environ.get("OPENCLAW_TOKEN", "")
MAGIC_URL = os.environ.get("MAGIC_URL", "http://localhost:8080")
WORKER_PORT = int(os.environ.get("WORKER_PORT", "9010"))

worker = Worker(
    name="OpenClawWorker",
    endpoint=f"http://localhost:{WORKER_PORT}",
    max_workers=3,
)


def _ask_openclaw(prompt: str) -> str:
    """Send a prompt to OpenClaw Gateway and return the response text."""
    headers = {}
    if OPENCLAW_TOKEN:
        headers["Authorization"] = f"Bearer {OPENCLAW_TOKEN}"

    resp = httpx.post(
        f"{OPENCLAW_URL}/v1/chat/completions",
        headers=headers,
        json={
            "model": "openclaw",
            "messages": [{"role": "user", "content": prompt}],
        },
        timeout=120,
    )
    resp.raise_for_status()
    return resp.json()["choices"][0]["message"]["content"]


@worker.capability(
    "ask",
    description="Ask the OpenClaw agent to perform any task via natural language",
    est_cost=0.01,
)
def ask(prompt: str) -> str:
    """Forward the prompt to OpenClaw and return its response."""
    return _ask_openclaw(prompt)


@worker.capability(
    "summarize",
    description="Summarize a piece of text using OpenClaw",
    est_cost=0.01,
)
def summarize(text: str) -> str:
    return _ask_openclaw(f"Summarize the following text concisely:\n\n{text}")


@worker.capability(
    "analyze",
    description="Analyze and extract insights from data or text using OpenClaw",
    est_cost=0.02,
)
def analyze(content: str, question: str = "What are the key insights?") -> str:
    return _ask_openclaw(f"{question}\n\n{content}")


if __name__ == "__main__":
    print(f"Connecting to OpenClaw at {OPENCLAW_URL}")
    print(f"Registering with MagiC at {MAGIC_URL}")
    worker.register(MAGIC_URL)
    worker.serve(port=WORKER_PORT)
