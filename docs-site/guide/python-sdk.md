# Python SDK

```bash
pip install magic-ai-sdk
```

## Worker

```python
from magic_ai_sdk import Worker

worker = Worker(
    name="MyBot",
    endpoint="http://myhost:9000",  # where MagiC can reach this worker
)
```

## Capabilities

```python
@worker.capability(
    "summarize",
    description="Summarizes text",
    input_schema={
        "type": "object",
        "properties": {
            "text": {"type": "string"},
            "max_words": {"type": "integer", "default": 100}
        },
        "required": ["text"]
    },
    est_cost_per_call=0.002,
)
def summarize(text: str, max_words: int = 100) -> str:
    return your_llm.summarize(text, max_words)
```

## Register and serve

```python
# Register with MagiC server
worker.register(
    server_url="http://localhost:8080",
    api_key="your-api-key",  # optional
)

# Start serving (blocks)
worker.serve(host="0.0.0.0", port=9000)
```

Or combine:

```python
worker.run(
    server_url="http://localhost:8080",
    host="0.0.0.0",
    port=9000,
)
```

## Client

Submit tasks and query results from Python:

```python
from magic_ai_sdk import MagiCClient

client = MagiCClient("http://localhost:8080", api_key="your-key")

# Submit task and wait for result
result = client.submit_and_wait(
    task_type="summarize",
    input={"text": "Long article..."},
    timeout=30,
)
print(result.output)

# List workers
workers = client.list_workers()
```
