# Observability

## Prometheus metrics

MagiC exports Prometheus metrics at `GET /metrics` (no authentication required):

```bash
curl http://localhost:8080/metrics | grep magic_
```

### Available metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `magic_tasks_total` | Counter | `type`, `status`, `worker` | Tasks processed |
| `magic_task_duration_seconds` | Histogram | `type`, `worker` | Task processing time |
| `magic_workers_active` | Gauge | `org` | Active workers |
| `magic_cost_total_usd` | Counter | `org`, `worker` | Total cost in USD |
| `magic_workflow_steps_total` | Counter | `status` | Workflow steps |
| `magic_knowledge_queries_total` | Counter | `type` | Knowledge hub queries |
| `magic_rate_limit_hits_total` | Counter | `endpoint` | Rate limit rejections |
| `magic_webhook_deliveries_total` | Counter | `status` | Webhook delivery attempts |
| `magic_streams_active` | Gauge | — | Active SSE connections |

### Grafana dashboard

Scrape config for `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: magic
    static_configs:
      - targets: ['localhost:8080']
```

## Structured logging

All events are logged as JSON to stdout:

```json
{"level":"info","time":"2026-04-09T10:00:00Z","type":"task.completed","task_id":"t-abc","worker_id":"w-xyz","cost":0.003}
```

## JSON stats endpoint

Human-readable stats (no Prometheus needed):

```bash
curl http://localhost:8080/api/v1/metrics
```

```json
{
  "total_events": 1523,
  "tasks_routed": 847,
  "tasks_done": 831,
  "tasks_failed": 16,
  "workers_count": 4
}
```
