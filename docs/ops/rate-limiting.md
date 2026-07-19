# Rate Limiting

MagiC protects the gateway with per-endpoint, per-key token-bucket limits. Two
backends ship in the binary and are selected at startup by a single env var.

## Backends

| Backend    | How to enable            | Scope               | When to use              |
|------------|--------------------------|---------------------|--------------------------|
| In-memory  | (default, no config)     | Per gateway process | Single-instance deploys  |
| Redis      | Set `MAGIC_REDIS_URL`    | Shared across pods  | Multi-instance deploys   |

**In-memory** uses `golang.org/x/time/rate` with an LRU-style cap of 10,000
tracked keys per bucket. It is fast and has zero extra infra, but each gateway
replica counts independently. Running N replicas effectively gives users Nx
their intended limit — unacceptable for any serious multi-instance deployment.

**Redis** stores each token bucket as a hash under
`magic:ratelimit:{bucket}:{key}` and refills/consumes atomically via a Lua
script. All replicas share the same counters, so a user hits the real limit
regardless of which instance handled the request.

## Enabling Redis

```bash
# Standard redis URL; username/password optional.
export MAGIC_REDIS_URL="redis://redis.internal:6379/0"

# TLS / Redis Cloud / Upstash also work:
export MAGIC_REDIS_URL="rediss://user:pass@example.upstash.io:6379"
```

MagiC logs the choice at startup:

```
rate limiter: redis (addr=redis.internal:6379)
```

or, when unset:

```
rate limiter: in-memory (set MAGIC_REDIS_URL for distributed limiting)
```

No other env vars are needed; existing per-endpoint rates are unchanged.

## Fail-open policy

If Redis is unreachable or returns an error, the Redis limiter **allows the
request** and logs a warning (rate-limited to ~1 line per 5s per bucket to
avoid log floods). We explicitly prefer letting traffic through over
rejecting valid users because of infra issues — rate limits are a guardrail,
not a primary security control.

Operators should monitor Redis separately (health check, `PING`, Prometheus
redis_exporter) and alert on `magic_rate_limit_hits_total` dropping to zero
unexpectedly, which can indicate the limiter has degraded to fail-open.

## Default rate limits

These are set in `core/internal/gateway/gateway.go` and apply to both backends.

| Endpoint group                                   | Bucket name | Rate               | Burst | Key         |
|--------------------------------------------------|-------------|--------------------|-------|-------------|
| `POST /api/v1/workers/register`                  | `register`  | 10 req/IP/min      | 5     | client IP   |
| `POST /api/v1/workers/heartbeat`                 | `heartbeat` | 4 req/IP/min       | 4     | client IP   |
| `POST/DELETE /api/v1/orgs/{orgID}/tokens/*`      | `token`     | 20 req/org/min     | 10    | orgID       |
| `POST /api/v1/tasks` (and `/tasks/stream`) — IP  | `task`      | 200 req/IP/min     | 20    | client IP   |
| `POST /api/v1/tasks` (and `/tasks/stream`) — org | `orgtask`   | 200 req/org/min    | 20    | X-Org-ID    |
| `POST /api/v1/llm/chat`, prompts, memory writes  | `llm`       | 30 req/IP/min      | 5     | client IP   |

`client IP` honours `X-Forwarded-For` only when `MAGIC_TRUSTED_PROXY=true`
(see `ratelimit.go::clientIP`).

## Disabling for local dev / load tests

```bash
MAGIC_RATE_LIMIT_DISABLE=true ./magic serve
```

This short-circuits the middleware entirely; no key lookups, no Redis calls.

## Monitoring

Exposed on `/metrics` (Prometheus):

```
magic_rate_limit_hits_total{path="/api/v1/workers/register"}  counter
```

Incremented every time a request is denied (429). Sudden spikes usually mean
either a real abuse wave or an integration bug in a client worker.

## When should I upgrade to Redis?

- You run ≥2 gateway replicas → **yes, always**.
- You plan to autoscale → **yes**, or rate limits become meaningless under scale.
- Single instance, dev / staging → in-memory is fine.

The switch is a single env var and a small Redis (even 128 MB is plenty — the
bucket keys are tiny hashes and auto-expire after 10 minutes of idle).
