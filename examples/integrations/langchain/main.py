"""
LangChain + MagiC integration example.

Wraps a LangChain tool-calling agent (calculator + optional web search) as a
single MagiC worker capability. Your LangChain logic is untouched; MagiC
adds retry, circuit breaking, cost limits, and observability on top.

Usage::

    cp .env.example .env
    pip install -r requirements.txt
    # separate terminal: cd core && go build ./cmd/magic && ./magic serve
    python main.py
"""

from __future__ import annotations

import logging
import os

from dotenv import load_dotenv
from langchain.agents import AgentExecutor, create_tool_calling_agent
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.tools import tool
from langchain_openai import ChatOpenAI
from magic_ai_sdk import Worker

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("langchain-worker")

MAGIC_URL = os.getenv("MAGIC_GATEWAY_URL", "http://localhost:8080")
WORKER_PORT = int(os.getenv("WORKER_PORT", "9102"))
LLM_MODEL = os.getenv("LANGCHAIN_LLM_MODEL", "gpt-4o-mini")


# ── Tools ──────────────────────────────────────────────────────────────────
@tool
def calculator(expression: str) -> str:
    """Evaluate a basic arithmetic expression. Supports + - * / ( ) and floats."""
    allowed = set("0123456789+-*/(). ")
    if not set(expression) <= allowed:
        return "error: unsupported characters"
    try:
        return str(eval(expression, {"__builtins__": {}}, {}))  # noqa: S307
    except Exception as e:
        return f"error: {e}"


@tool
def web_search(query: str) -> str:
    """Search the web via DuckDuckGo. Returns up to 3 result snippets."""
    try:
        from duckduckgo_search import DDGS  # optional dep
    except ImportError:
        return "web_search unavailable — install duckduckgo-search"
    with DDGS() as ddgs:
        hits = list(ddgs.text(query, max_results=3))
    if not hits:
        return "no results"
    return "\n\n".join(f"{h.get('title', '')}: {h.get('body', '')}" for h in hits)


TOOLS = [calculator, web_search]


def _build_agent() -> AgentExecutor:
    llm = ChatOpenAI(model=LLM_MODEL, temperature=0)
    prompt = ChatPromptTemplate.from_messages([
        ("system", "You are a careful assistant. Use tools when the question needs them."),
        ("human", "{input}"),
        ("placeholder", "{agent_scratchpad}"),
    ])
    agent = create_tool_calling_agent(llm, TOOLS, prompt)
    return AgentExecutor(agent=agent, tools=TOOLS, verbose=False, return_intermediate_steps=True)


_executor: AgentExecutor | None = None


def _executor_singleton() -> AgentExecutor:
    global _executor
    if _executor is None:
        _executor = _build_agent()
    return _executor


# ── MagiC worker ───────────────────────────────────────────────────────────
worker = Worker(
    name="LangChainWorker",
    endpoint=f"http://localhost:{WORKER_PORT}",
    worker_token=os.getenv("MAGIC_WORKER_TOKEN", ""),
)


@worker.capability(
    name="qa_with_tools",
    description="Answer a question using a LangChain tool-calling agent (calculator + web search). Args: question (str).",
    est_cost=0.02,
)
def qa_with_tools(question: str) -> dict:
    """Run the LangChain agent once and return the answer plus step trace."""
    log.info("LangChain run — question=%r", question)
    result = _executor_singleton().invoke({"input": question})
    steps = [
        {"tool": action.tool, "input": action.tool_input, "output": str(observation)[:500]}
        for action, observation in result.get("intermediate_steps", [])
    ]
    return {"answer": result.get("output", ""), "steps": steps}


if __name__ == "__main__":
    log.info("Starting LangChainWorker → MagiC at %s", MAGIC_URL)
    worker.run(MAGIC_URL, port=WORKER_PORT)
