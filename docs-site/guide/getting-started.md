# Quick Start

Get MagiC running in under 5 minutes.

## 1. Start the server

::: code-group

```bash [pip install]
pip install magic-ai-sdk
```

```bash [From source]
git clone https://github.com/kienbui1995/magic.git
cd magic/core && go build -o ../bin/magic ./cmd/magic
./bin/magic serve
```

```bash [Docker]
docker run -p 8080:8080 ghcr.io/kienbui1995/magic:latest
```

:::

## 2. Build a worker

```python
from magic_ai_sdk import Worker

worker = Worker(name="HelloBot", endpoint="http://localhost:9000")

@worker.capability("greeting", description="Says hello to anyone")
def greet(name: str) -> str:
    return f"Hello, {name}! I'm managed by MagiC."

worker.register("http://localhost:8080")
worker.serve()  # serves on :9000
```

```bash
python worker.py
# ✓ HelloBot registered (worker_abc123)
# ✓ Serving on 0.0.0.0:9000
```

## 3. Submit a task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "greeting",
    "input": {"name": "World"}
  }'
```

```json
{
  "id": "t-abc123",
  "status": "completed",
  "output": "Hello, World! I'm managed by MagiC.",
  "cost": 0.001
}
```

## Next steps

- [Core Concepts](/guide/concepts) — understand workers, tasks, and workflows
- [Python SDK](/guide/python-sdk) — full SDK reference
- [Deployment](/guide/deployment) — run in production
