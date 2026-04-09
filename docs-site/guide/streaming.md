# SSE Streaming

Stream AI output token-by-token for responsive user experiences.

## Workers with streaming

Declare streaming support in your worker capability:

```python
@worker.capability(
    "chat",
    description="AI chat that streams responses",
    streaming=True,  # enables streaming
)
def chat_stream(message: str):
    # yield tokens as they're generated
    for token in llm.stream(message):
        yield token
```

Your worker must also expose a `/stream` endpoint that returns `text/event-stream`.

## Submit a streaming task

```bash
curl -N http://localhost:8080/api/v1/tasks/stream \
  -H "Content-Type: application/json" \
  -d '{"type": "chat", "input": {"message": "Explain quantum computing"}}'
```

Response (Server-Sent Events):
```
data: {"chunk": "Quantum ", "done": false}

data: {"chunk": "computing ", "done": false}

data: {"chunk": "uses...", "done": false}

data: {"task_id": "t-abc123", "done": true}
```

## Reconnection

If the connection drops, reconnect with:

```bash
curl -N http://localhost:8080/api/v1/tasks/{task_id}/stream
```

- Task `completed` → returns final output as single SSE event
- Task `failed` → returns error event
- Task still `running` → returns 202 (poll with GET /tasks/{id})

## JavaScript example

```javascript
const response = await fetch('/api/v1/tasks/stream', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ type: 'chat', input: { message: 'Hello' } }),
});

const reader = response.body.getReader();
const decoder = new TextDecoder();

while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  
  const lines = decoder.decode(value).split('\n');
  for (const line of lines) {
    if (line.startsWith('data: ')) {
      const event = JSON.parse(line.slice(6));
      if (event.done) break;
      process.stdout.write(event.chunk);
    }
  }
}
```
