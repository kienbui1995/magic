# API Reference

Base URL: `http://localhost:8080`

Authentication: `Authorization: Bearer <MAGIC_API_KEY>` (if `MAGIC_API_KEY` is set)

## Workers

### Register worker
`POST /api/v1/workers/register`

Requires worker token auth (`Authorization: Bearer <worker-token>`).

**Body:**
```json
{
  "name": "MyBot",
  "endpoint": {"url": "http://myhost:9000"},
  "capabilities": [
    {
      "name": "summarize",
      "description": "Summarizes text",
      "est_cost_per_call": 0.002,
      "streaming": false
    }
  ],
  "limits": {
    "max_concurrent_tasks": 5,
    "max_cost_per_day": 10.0
  }
}
```

### List workers
`GET /api/v1/workers?limit=100&offset=0`

### Get worker
`GET /api/v1/workers/{id}`

### Deregister worker
`DELETE /api/v1/workers/{id}` — requires worker token auth

## Tasks

### Submit task
`POST /api/v1/tasks`

**Body:**
```json
{
  "type": "summarize",
  "input": {"text": "..."},
  "priority": "normal",
  "routing": {
    "strategy": "best_match",
    "required_capabilities": ["summarize"],
    "preferred_workers": [],
    "org_id": "org-123"
  },
  "contract": {
    "timeout_ms": 30000,
    "max_cost": 0.10,
    "output_schema": {}
  }
}
```

**Response:**
```json
{
  "id": "t-abc123",
  "status": "completed",
  "output": {...},
  "cost": 0.002,
  "assigned_worker": "w-xyz"
}
```

### Submit streaming task
`POST /api/v1/tasks/stream` → SSE stream

### Get task
`GET /api/v1/tasks/{id}`

### Re-subscribe to stream
`GET /api/v1/tasks/{id}/stream` → SSE stream

## Workflows

### Submit workflow
`POST /api/v1/workflows`

### Get workflow
`GET /api/v1/workflows/{id}`

### Approve step (human-in-the-loop)
`POST /api/v1/workflows/{id}/approve/{stepId}`

## Knowledge

### Add entry
`POST /api/v1/knowledge`

```json
{"title": "Guide", "content": "...", "tags": ["ai"], "scope": "org", "scope_id": "org-123"}
```

### Search
`GET /api/v1/knowledge?q=query`

### Store embedding (pgvector)
`POST /api/v1/knowledge/{id}/embedding`

```json
{"vector": [0.1, 0.2, ...], "metadata": {}}
```

### Semantic search
`POST /api/v1/knowledge/search/semantic`

```json
{"query_vector": [0.1, ...], "top_k": 5}
```

## Webhooks

### Register
`POST /api/v1/orgs/{orgID}/webhooks`

### List
`GET /api/v1/orgs/{orgID}/webhooks`

### Delete
`DELETE /api/v1/orgs/{orgID}/webhooks/{webhookID}`

## System

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |
| `GET /api/v1/metrics` | JSON stats |
| `GET /metrics` | Prometheus metrics (no auth) |
| `GET /api/v1/costs` | Cost report |
