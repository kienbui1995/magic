"""
CrewAI + MagiC integration example.

Wraps a CrewAI multi-agent crew (researcher + writer) as a single MagiC worker
capability. The existing CrewAI logic is preserved — MagiC just provides the
infrastructure layer: task routing, cost tracking, retries, policy, audit.

Usage::

    cp .env.example .env              # fill in OPENAI_API_KEY
    pip install -r requirements.txt
    # in another terminal: cd core && go build ./cmd/magic && ./magic serve
    python main.py
"""

from __future__ import annotations

import logging
import os

from crewai import Agent, Crew, Process, Task
from dotenv import load_dotenv
from magic_ai_sdk import Worker

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("crewai-worker")

MAGIC_URL = os.getenv("MAGIC_GATEWAY_URL", "http://localhost:8080")
WORKER_PORT = int(os.getenv("WORKER_PORT", "9101"))
LLM_MODEL = os.getenv("CREWAI_LLM_MODEL", "gpt-4o-mini")
# For Ollama: set OPENAI_API_BASE=http://localhost:11434/v1, OPENAI_API_KEY=ollama,
# and CREWAI_LLM_MODEL=ollama/llama3.2


def _build_crew(topic: str) -> Crew:
    """Build a 2-agent crew: researcher gathers info, writer drafts the report."""
    researcher = Agent(
        role="Senior Research Analyst",
        goal=f"Uncover the most relevant facts about {topic}",
        backstory=(
            "You are a meticulous researcher with a talent for finding reliable "
            "signals in noisy information. You present findings as bullet points."
        ),
        llm=LLM_MODEL,
        allow_delegation=False,
        verbose=False,
    )

    writer = Agent(
        role="Technical Writer",
        goal=f"Turn raw research about {topic} into a publishable briefing",
        backstory=(
            "You transform dense research notes into clear, well-structured prose "
            "suitable for engineering leaders."
        ),
        llm=LLM_MODEL,
        allow_delegation=False,
        verbose=False,
    )

    research_task = Task(
        description=f"Research the topic: {topic}. List 5-7 key facts with context.",
        expected_output="Bulleted list of facts, each 1-2 sentences.",
        agent=researcher,
    )
    write_task = Task(
        description=f"Using the research findings, write a 3-paragraph briefing on {topic}.",
        expected_output="A polished, 3-paragraph briefing in plain prose.",
        agent=writer,
        context=[research_task],
    )

    return Crew(
        agents=[researcher, writer],
        tasks=[research_task, write_task],
        process=Process.sequential,
        verbose=False,
    )


worker = Worker(
    name="CrewAIWorker",
    endpoint=f"http://localhost:{WORKER_PORT}",
    worker_token=os.getenv("MAGIC_WORKER_TOKEN", ""),
)


@worker.capability(
    name="research_and_write",
    description="Run a CrewAI crew that researches a topic and drafts a briefing. Args: topic (str).",
    est_cost=0.05,
)
def research_and_write(topic: str) -> dict:
    """Run the two-agent CrewAI crew end to end and return the final report."""
    log.info("CrewAI kickoff — topic=%s model=%s", topic, LLM_MODEL)
    crew = _build_crew(topic)
    result = crew.kickoff(inputs={"topic": topic})
    return {"report": str(result), "topic": topic, "model": LLM_MODEL}


if __name__ == "__main__":
    log.info("Starting CrewAIWorker → MagiC at %s", MAGIC_URL)
    worker.run(MAGIC_URL, port=WORKER_PORT)
