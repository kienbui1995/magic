# OpenTelemetry Tracing

MagiC emits OTLP-compatible traces. Any OTel collector can ingest them —
Jaeger, Grafana Tempo, Datadog Agent, Honeycomb, New Relic, AWS X-Ray
(via ADOT), etc.

When `OTEL_EXPORTER_OTLP_ENDPOINT` is unset MagiC installs a no-op tracer:
spans cost ~nothing and no network I/O happens. This is the safe default
for dev.

## Environment variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Collector URL, e.g. `http://localhost:4318` | unset (no-op) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` or `grpc` | `http/protobuf` |
| `OTEL_SERVICE_NAME` | Service name attached to every span | `magic` |
| `OTEL_SERVICE_VERSION` | Version tag | unset |
| `OTEL_TRACES_SAMPLER` | `always_on`, `always_off`, `traceidratio`, `parentbased_traceidratio`, `parentbased_always_on/off` | `always_on` |
| `OTEL_TRACES_SAMPLER_ARG` | Ratio for ratio-based samplers (0.0–1.0) | `1.0` |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource key-values, e.g. `env=prod,region=ap-se-1` | unset |
| `MAGIC_OTEL_STDOUT` | `1` to also dump spans to stdout for debugging | off |

## Quickstart — Jaeger (local)

```bash
docker compose -f deploy/docker-compose.observability.yml up -d
# MagiC exports to jaeger:4318 automatically (see compose file).
# Open http://localhost:16686 and search for service "magic".
```

Submit a task, then view the trace in Jaeger. You will see:

- `POST /api/v1/tasks` — root HTTP span (from `otelhttp` middleware)
- `dispatcher.Dispatch` — child span with `task.id`, `worker.id` attributes
- Downstream worker spans — automatically linked via W3C `traceparent`
  injected by the dispatcher.

## Vendor recipes

### Datadog Agent

Run the Datadog Agent with OTLP enabled and point MagiC at it:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://dd-agent:4318 \
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
OTEL_SERVICE_NAME=magic \
OTEL_RESOURCE_ATTRIBUTES="deployment.environment=prod" \
./magic serve
```

### Honeycomb

Honeycomb accepts OTLP directly. Supply API key as a header via the standard
`OTEL_EXPORTER_OTLP_HEADERS` env var:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://api.honeycomb.io \
OTEL_EXPORTER_OTLP_HEADERS="x-honeycomb-team=YOUR_API_KEY" \
OTEL_SERVICE_NAME=magic \
./magic serve
```

### Grafana Tempo

Tempo ships with an OTLP receiver. Point at the receiver port:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://tempo:4318 \
./magic serve
```

### New Relic

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.nr-data.net \
OTEL_EXPORTER_OTLP_HEADERS="api-key=YOUR_LICENSE_KEY" \
./magic serve
```

## Sampling strategy

- **Dev / low traffic**: `always_on` — see every request.
- **Staging**: `parentbased_traceidratio` with `OTEL_TRACES_SAMPLER_ARG=0.5`
  so sampled incoming requests stay sampled throughout the pipeline.
- **Prod / high traffic**: `parentbased_traceidratio` with `0.05`–`0.1`
  typically balances cost vs signal. For head-based sampling this means
  5–10% of traces are retained end-to-end.
- **Debugging a specific tenant**: keep the service on a low ratio but
  configure the collector (e.g. OTel Collector tail sampler) to retain
  100% of spans matching `org.id == "tenant-X"`.

## Tuning the batch span processor

Defaults in `core/internal/tracing/init.go`:

- Batch timeout: 5 s
- Max export batch size: 512 spans
- Max queue size: 2048 spans

If you see `OTel SDK: span queue full` warnings, raise queue size or
shorten the batch timeout. If exports are slow / collector flaky, keep
the queue generous — the processor drops spans silently when full, it
never blocks hot paths.

## Verification checklist

```bash
# 1. Tracer installed?
curl -s http://localhost:8080/health
# 2. Send a request.
curl -s -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type":"echo","input":{"msg":"hi"}}' \
  http://localhost:8080/api/v1/tasks
# 3. Open http://localhost:16686 → Service: magic → Find Traces.
#    You should see "POST /api/v1/tasks" with child span "dispatcher.Dispatch".
```

## Worker-to-gateway continuity

Workers that use the MagiC Python SDK inherit trace context automatically
via the `traceparent` header on the outbound `task.assign` HTTP call.
Legacy workers that only read `X-Trace-ID` keep working — MagiC always
sets both headers on outbound dispatches.
