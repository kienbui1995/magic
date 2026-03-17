# Using CrewAI Agents as MagiC Workers

This guide shows how to wrap your existing CrewAI agents as MagiC workers, so they can be managed, routed, and monitored by the MagiC framework.

## Why?

- **Keep your CrewAI agents** — no rewrite needed
- **Add cost control** — MagiC tracks spending per worker
- **Add routing** — MagiC routes tasks to the best available agent
- **Add orchestration** — combine CrewAI agents with other workers in DAG workflows
- **Add monitoring** — structured logging and metrics for all agents

## Prerequisites

```bash
pip install magic-ai-sdk crewai
```

## Step 1: Wrap a CrewAI Agent

```python
from magic_ai_sdk import Worker
from crewai import Agent, Task, Crew

# Create your CrewAI agent as usual
researcher = Agent(
    role="Senior Research Analyst",
    goal="Research and analyze topics thoroughly",
    backstory="You are an expert researcher with deep analytical skills.",
    verbose=False,
)

# Create a MagiC worker that wraps the agent
worker = Worker(name="CrewAI-Researcher", endpoint="http://localhost:9010")

@worker.capability("research", description="Research a topic using CrewAI agent")
def research(topic: str, depth: str = "detailed") -> dict:
    task = Task(
        description=f"Research the following topic: {topic}. Provide a {depth} analysis.",
        expected_output="A comprehensive research report",
        agent=researcher,
    )
    crew = Crew(agents=[researcher], tasks=[task], verbose=False)
    result = crew.kickoff()
    return {
        "report": str(result),
        "topic": topic,
        "agent": "CrewAI-Researcher",
    }

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve(port=9010)
```

## Step 2: Wrap a CrewAI Crew (Multi-Agent)

```python
from magic_ai_sdk import Worker
from crewai import Agent, Task, Crew

writer = Agent(role="Writer", goal="Write engaging content", backstory="Expert writer")
editor = Agent(role="Editor", goal="Edit and improve content", backstory="Expert editor")

worker = Worker(name="CrewAI-ContentTeam", endpoint="http://localhost:9011")

@worker.capability("content_pipeline", description="Write and edit content using CrewAI crew")
def content_pipeline(topic: str, tone: str = "professional") -> dict:
    write_task = Task(
        description=f"Write a {tone} article about {topic}",
        expected_output="A well-written article",
        agent=writer,
    )
    edit_task = Task(
        description="Edit and improve the article for clarity and grammar",
        expected_output="A polished, publication-ready article",
        agent=editor,
    )
    crew = Crew(agents=[writer, editor], tasks=[write_task, edit_task], verbose=False)
    result = crew.kickoff()
    return {"article": str(result), "topic": topic}

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve(port=9011)
```

## Step 3: Combine with Other Workers

Now your CrewAI agents can participate in MagiC workflows alongside any other worker:

```bash
# Submit a workflow: CrewAI researcher -> Summarizer -> Translator
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Research Pipeline",
    "steps": [
      {"id": "research", "task_type": "research", "input": {"topic": "AI agents in 2026"}},
      {"id": "summarize", "task_type": "summarize", "depends_on": ["research"]},
      {"id": "translate", "task_type": "translate", "depends_on": ["summarize"], "input": {"target_lang": "vi"}}
    ]
  }'
```

This workflow:
1. **CrewAI researcher** does deep research
2. **SummarizerBot** (simple Python) extracts key points
3. **TranslatorBot** (simple Python) translates to Vietnamese

Different frameworks, different languages — all managed by MagiC.

## Architecture

```
MagiC Server (Go)
├── CrewAI-Researcher (Python + CrewAI)    ← wrapped as MagiC worker
├── CrewAI-ContentTeam (Python + CrewAI)   ← wrapped as MagiC worker
├── SummarizerBot (Python)                 ← native MagiC worker
├── TranslatorBot (Python)                 ← native MagiC worker
└── Any other worker (Node.js, Go, etc.)
```

## Tips

- **One capability per wrapper** keeps things simple and composable
- **Set `verbose=False`** on CrewAI agents to avoid cluttering worker logs
- **Use `max_cost_per_day`** in worker limits to control CrewAI API spending
- **Health check**: MagiC monitors heartbeats, so your CrewAI workers stay tracked
