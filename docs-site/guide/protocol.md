# MCP² Protocol

MagiC Protocol (MCP²) defines how workers communicate with the MagiC server.

## Message format

All messages are JSON:

```json
{
  "protocol": "mcp2",
  "version": "1",
  "type": "task.assign",
  "id": "msg-abc123",
  "timestamp": "2026-04-09T10:00:00Z",
  "source": "org",
  "target": "worker-xyz",
  "payload": { ... }
}
```

## Task lifecycle

**1. MagiC sends `task.assign` to worker:**

```json
{
  "type": "task.assign",
  "payload": {
    "task_id": "t-abc",
    "task_type": "summarize",
    "input": {"text": "..."},
    "contract": {"timeout_ms": 30000}
  }
}
```

**2. Worker responds with `task.complete`:**

```json
{
  "type": "task.complete",
  "payload": {
    "task_id": "t-abc",
    "output": "Summary: ...",
    "cost": 0.003
  }
}
```

**Or `task.fail`:**

```json
{
  "type": "task.fail",
  "payload": {
    "task_id": "t-abc",
    "error": {"code": "TIMEOUT", "message": "LLM timed out"}
  }
}
```

## Worker registration

```json
{
  "type": "worker.register",
  "payload": {
    "name": "MyBot",
    "endpoint": {"url": "http://myhost:9000"},
    "capabilities": [...]
  }
}
```

Workers must send heartbeats every 30 seconds:

```json
{"type": "worker.heartbeat", "payload": {"worker_id": "w-xyz", "current_load": 2}}
```
