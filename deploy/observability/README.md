# MagiC Observability Stack

Standalone Grafana + Prometheus + MagiC + PostgreSQL for local testing and reference deployments.

## Quick start

```bash
# From repo root:
docker compose -f deploy/docker-compose.observability.yml up -d

# Wait ~30s for everything to become healthy, then:
open http://localhost:3000    # Grafana (admin / admin)
open http://localhost:9090    # Prometheus
open http://localhost:8080/metrics   # raw MagiC metrics
```

Change the admin password:

```bash
GRAFANA_ADMIN_PASSWORD='strong-pass' \
POSTGRES_PASSWORD='strong-db-pass' \
MAGIC_API_KEY='strong-api-key' \
docker compose -f deploy/docker-compose.observability.yml up -d
```

## Port map

| Port  | Service          | Notes |
|-------|------------------|-------|
| 3000  | Grafana          | admin / `$GRAFANA_ADMIN_PASSWORD` (default `admin`) |
| 9090  | Prometheus       | TSDB + alert rules |
| 8080  | MagiC Gateway    | `GET /metrics` (Prometheus), `GET /health`, `/api/v1/*` |
| 5432  | PostgreSQL       | not exposed externally; internal network only |
| 9093  | Alertmanager     | optional, commented |
| 16686 | Jaeger UI        | optional, commented (awaits OTel tracing) |

## What ships out of the box

**Dashboards** (auto-provisioned into the `MagiC` folder):

- **MagiC Framework Overview** (`magic-overview.json`) — task rate, error rate, latency quantiles, active workers, worker load, cost/hour, webhook success rate, queue depth, rate-limit hits, SSE stream count.
- **MagiC Costs & Budgets** (`magic-costs.json`) — 24h/7d spend, avg cost per task, spend rate, top cost workers, top cost orgs, cost leaderboard.

Both are wired to the provisioned **Prometheus** datasource and expose `$org` / `$worker` template variables.

**Alerts** (`deploy/prometheus/alerts.yaml`, group `magic.rules`):

| Alert | Severity | Trigger |
|---|---|---|
| `MagicHighErrorRate` | warning | Task failure rate > 5% for 5m |
| `MagicHighLatency` | warning | Task p99 > 30s for 10m |
| `MagicWebhookDeliveryFailures` | warning | Webhook failed/dead rate > 10% for 10m |
| `MagicBudgetExceeded` | critical | Any `budget.exceeded` event delivered (auto-pause fired) |
| `MagicWorkerOffline` | warning | `magic_worker_heartbeat_lag_seconds > 300` for 2m |
| `MagicNoWorkersAvailable` | critical | Task failures while `magic_workers_active == 0` |
| `MagicDLQGrowing` | warning | > 100 dead webhook deliveries / hour |
| `MagicRateLimitPressure` | info | Any endpoint rejecting > 1 req/s for 10m |

Severities follow the convention:

- **critical** — page immediately (data loss, production outage, budget blown).
- **warning** — human response within an hour (latency, elevated error rate).
- **info** — awareness / ticket (capacity pressure).

All annotations include a `runbook_url` placeholder — replace with your real runbook location.

## SLO suggestions

| SLI | Target | PromQL |
|-----|--------|--------|
| Task success rate | 99% rolling 30d | `1 - sum(increase(magic_tasks_total{status="failed"}[30d])) / sum(increase(magic_tasks_total[30d]))` |
| Task latency (p99) | < 10s | `histogram_quantile(0.99, sum by (le) (rate(magic_task_duration_seconds_bucket[5m])))` |
| Webhook delivery | 99.5% | `1 - sum(rate(magic_webhook_deliveries_total{status=~"failed|dead"}[30d])) / sum(rate(magic_webhook_deliveries_total[30d]))` |
| Gateway availability | 99.9% | from your probe / black-box exporter, not in-band |

Error budget examples (30d):

- 99.0% success → 7h 12m budget
- 99.5% → 3h 36m
- 99.9% → 43m

## Importing dashboards manually

If you're using your own Grafana:

```bash
# From your Grafana UI:
# Dashboards → New → Import → Upload JSON
# Pick deploy/grafana/dashboards/magic-overview.json
# Pick deploy/grafana/dashboards/magic-costs.json
# Select your Prometheus datasource when prompted.
```

Or copy the provisioning bits:

```bash
cp -r deploy/grafana/provisioning/* /etc/grafana/provisioning/
cp deploy/grafana/dashboards/*.json /var/lib/grafana/dashboards/
systemctl restart grafana-server
```

## Wiring alerts to Slack / PagerDuty

Uncomment the `alertmanager` service in `docker-compose.observability.yml`,
then create `deploy/alertmanager/alertmanager.yml`, for example:

```yaml
route:
  receiver: slack
  group_by: [alertname, component]
  routes:
    - matchers: [severity="critical"]
      receiver: pagerduty

receivers:
  - name: slack
    slack_configs:
      - api_url: "https://hooks.slack.com/services/XXX/YYY/ZZZ"
        channel: "#magic-alerts"
  - name: pagerduty
    pagerduty_configs:
      - service_key: "YOUR_PD_INTEGRATION_KEY"
```

Then restart the stack.

## Metrics currently exposed by MagiC

Taken from `core/internal/monitor/metrics.go` — don't guess metric names, grep that file if unsure.

| Metric | Type | Labels |
|--------|------|--------|
| `magic_tasks_total` | counter | `type`, `status`, `worker` |
| `magic_task_duration_seconds` | histogram | `type`, `worker` (not yet populated by code — see note) |
| `magic_workers_active` | gauge | `org` |
| `magic_worker_heartbeat_lag_seconds` | gauge | `worker` (not yet populated — see note) |
| `magic_cost_total_usd` | counter | `org`, `worker` |
| `magic_workflow_steps_total` | counter | `status` |
| `magic_workflows_active` | gauge | — |
| `magic_knowledge_queries_total` | counter | `type` (`keyword` / `semantic`) |
| `magic_knowledge_entries_total` | gauge | — |
| `magic_rate_limit_hits_total` | counter | `endpoint` |
| `magic_webhook_deliveries_total` | counter | `status` (`delivered` / `failed` / `dead`) |
| `magic_webhook_delivery_duration_seconds` | histogram | — |
| `magic_streams_active` | gauge | — |
| `magic_stream_duration_seconds` | histogram | — |
| `magic_events_dropped_total` | counter | — (not yet populated — see note) |

**Note:** `magic_task_duration_seconds`, `magic_worker_heartbeat_lag_seconds`, and `magic_events_dropped_total` are declared but not currently populated by the code. Related dashboard panels / alert rules are left in place as forward-looking; they stay silent (no series) until the corresponding `.Observe()` / `.Set()` / `.Inc()` calls are wired up. File an issue against `core/internal/monitor/` if you need them active.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Grafana shows "No data" | Prometheus can't reach MagiC | `docker compose logs prometheus` — look for scrape errors |
| Dashboards missing | Provisioning path wrong | Check `deploy/grafana/provisioning/dashboards/magic.yaml` path matches container mount |
| Alerts never fire | Rule file not loaded | `curl http://localhost:9090/api/v1/rules` to confirm rules are loaded |
| `MagicHighLatency` silent | Task duration histogram empty | Expected today — see note above |
