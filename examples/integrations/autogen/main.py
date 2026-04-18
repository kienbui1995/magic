"""
AutoGen + MagiC integration example.

Hides a 3-agent AutoGen GroupChat behind a single MagiC worker capability.
The caller submits `{feature_idea: str}`; internally a product manager,
an engineer, and a critic converse up to 5 rounds to produce a spec. The
caller sees a clean `{spec, discussion_summary}` — the multi-agent complexity
is an implementation detail.

Usage::

    cp .env.example .env
    pip install -r requirements.txt
    # separate terminal: cd core && go build ./cmd/magic && ./magic serve
    python main.py
"""

from __future__ import annotations

import logging
import os

import autogen
from dotenv import load_dotenv
from magic_ai_sdk import Worker

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("autogen-worker")

MAGIC_URL = os.getenv("MAGIC_GATEWAY_URL", "http://localhost:8080")
WORKER_PORT = int(os.getenv("WORKER_PORT", "9103"))
LLM_MODEL = os.getenv("AUTOGEN_LLM_MODEL", "gpt-4o-mini")
MAX_ROUNDS = int(os.getenv("AUTOGEN_MAX_ROUNDS", "5"))


def _llm_config() -> dict:
    """Build AutoGen's llm_config. Honours OPENAI_API_BASE for Ollama users."""
    config = {"model": LLM_MODEL, "api_key": os.getenv("OPENAI_API_KEY", "")}
    if base := os.getenv("OPENAI_API_BASE"):
        config["base_url"] = base
    return {"config_list": [config], "temperature": 0.2, "cache_seed": None}


def _run_groupchat(feature_idea: str) -> tuple[str, str]:
    """Spin up PM → Engineer → Critic group chat; return (final_spec, summary)."""
    llm_config = _llm_config()

    product_manager = autogen.AssistantAgent(
        name="product_manager",
        system_message=(
            "You are a product manager. Turn the user's feature idea into a crisp "
            "problem statement, user stories, and success metrics. Hand off to the engineer."
        ),
        llm_config=llm_config,
    )
    engineer = autogen.AssistantAgent(
        name="engineer",
        system_message=(
            "You are a staff engineer. Propose a pragmatic technical design for the "
            "feature: data model, API shape, edge cases. Keep it short."
        ),
        llm_config=llm_config,
    )
    critic = autogen.AssistantAgent(
        name="critic",
        system_message=(
            "You are a skeptical critic. Poke holes in the spec: hidden assumptions, "
            "failure modes, scope creep. Be concrete."
        ),
        llm_config=llm_config,
    )
    user_proxy = autogen.UserProxyAgent(
        name="user_proxy",
        human_input_mode="NEVER",
        max_consecutive_auto_reply=0,
        code_execution_config=False,
        is_termination_msg=lambda m: "TERMINATE" in (m.get("content") or ""),
    )

    group = autogen.GroupChat(
        agents=[user_proxy, product_manager, engineer, critic],
        messages=[],
        max_round=MAX_ROUNDS,
        speaker_selection_method="round_robin",
    )
    manager = autogen.GroupChatManager(groupchat=group, llm_config=llm_config)

    initial = (
        f"Feature idea: {feature_idea}\n\n"
        "Produce a concise spec. End the conversation by writing TERMINATE on its own line."
    )
    user_proxy.initiate_chat(manager, message=initial, clear_history=True)

    messages = group.messages
    spec = next(
        (m["content"] for m in reversed(messages) if m.get("name") == "product_manager" and m.get("content")),
        messages[-1]["content"] if messages else "",
    )
    summary = "\n\n".join(f"[{m.get('name', '?')}]: {m.get('content', '')[:400]}" for m in messages)
    return spec, summary


# ── MagiC worker ───────────────────────────────────────────────────────────
worker = Worker(
    name="AutoGenWorker",
    endpoint=f"http://localhost:{WORKER_PORT}",
    worker_token=os.getenv("MAGIC_WORKER_TOKEN", ""),
)


@worker.capability(
    name="product_spec_review",
    description="Run a 3-agent AutoGen GroupChat (PM → Engineer → Critic) on a feature idea. "
                "Args: feature_idea (str).",
    est_cost=0.10,
)
def product_spec_review(feature_idea: str) -> dict:
    """Kick off the GroupChat and return the consolidated spec."""
    log.info("AutoGen run — feature_idea=%r", feature_idea)
    spec, summary = _run_groupchat(feature_idea)
    return {"spec": spec, "discussion_summary": summary, "rounds_limit": MAX_ROUNDS}


if __name__ == "__main__":
    log.info("Starting AutoGenWorker → MagiC at %s", MAGIC_URL)
    worker.run(MAGIC_URL, port=WORKER_PORT)
