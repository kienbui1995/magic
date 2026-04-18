# Scenario: Latency under sustained load

Characterise dispatch latency distribution when MagiC is operating steadily
below its throughput ceiling.

## Goal

Produce p50 / p95 / p99 / p99.9 for submit→complete latency at a **fixed**
rate of 100 requests per second, held for 10 minutes.

## Setup

Same bench stack as `throughput.md`, with **10 workers** (enough headroom that
queue depth stays near zero).

## Procedure

```bash
python3 ../scripts/load.py \
    --rate 100 \
    --duration 600 \
    --concurrency 50 \
    --out ../results/latency-100rps.csv
```

The load generator enforces the rate with a token bucket, so spikes do not
artificially inflate the tail.

## Metrics

| Metric | Definition |
|--------|------------|
| `latency_p50_ms` | median submit→complete |
| `latency_p95_ms` | 95th percentile |
| `latency_p99_ms` | 99th percentile |
| `latency_p999_ms` | 99.9th percentile |
| `error_rate` | fail / total |

## Anti-patterns to guard against

- **Coordinated omission**: the load generator records request start time at
  scheduled tick, not at actual submit, so slow responses do not hide missing
  latency samples.
- **Warm-up**: the first 30 seconds are excluded from the aggregate; they
  cover connection pool warm-up and JIT-style amortised cache fills.

## Expected Shape (not a promise)

At 100 rps with 10 workers the p99 should sit inside a small number of tens of
milliseconds; p99.9 can spike with Go GC pauses. Record the GC pause histogram
if possible (`GODEBUG=gctrace=1`).
