# Core Concepts

## Workers

A worker is any HTTP server that:
1. Registers with MagiC declaring its capabilities
2. Accepts `task.assign` messages via POST
3. Responds with `task.complete` or `task.fail`

```python
from magic_ai_sdk import Worker

worker = Worker(
    name="SummaryBot",
    endpoint="http://myserver:9001",  # where MagiC can reach this worker
)

@worker.capability(
    "summarize",
    description="Summarizes long text into bullet points",
    input_schema={"text": "string", "max_bullets": "integer"},
)
def summarize(text: str, max_bullets: int = 5) -> str:
    # your AI logic here
    return summarized_text
```

## Tasks

A task is a unit of work submitted to MagiC:

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -d '{
    "type": "summarize",
    "input": {"text": "...", "max_bullets": 3},
    "routing": {"strategy": "best_match"},
    "contract": {"timeout_ms": 30000, "max_cost": 0.10}
  }'
```

MagiC automatically:
- Finds the best available worker for `summarize`
- Dispatches the task via HTTP POST
- Tracks cost and validates output
- Returns the result

## Workflows (DAG)

Multi-step workflows run steps in parallel where possible:

```json
{
  "name": "content-pipeline",
  "steps": [
    {"id": "research", "task_type": "web_research", "input": {"query": "AI trends"}},
    {"id": "outline", "task_type": "outline_writer", "depends_on": ["research"]},
    {"id": "draft", "task_type": "content_writer", "depends_on": ["outline"]},
    {"id": "seo", "task_type": "seo_optimizer", "depends_on": ["draft"], "on_failure": "skip"}
  ]
}
```

Step outputs flow automatically to dependent steps via the `_deps` field in input.

## Organizations & Teams

Group workers into teams with shared policies:

```bash
# Create a team
curl -X POST http://localhost:8080/api/v1/teams \
  -d '{"name": "content-team", "workers": ["w-1", "w-2"], "daily_budget": 10.0}'
```

## Routing Strategies

| Strategy | Description |
|----------|-------------|
| `best_match` | Highest capability match score |
| `cheapest` | Lowest `est_cost_per_call` |
| `round_robin` | Distribute evenly |
| `specific` | Target a specific worker ID |
