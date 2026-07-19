# Scenario: Throughput

Measure the maximum sustained rate at which MagiC can route and dispatch tasks
end-to-end through the gateway.

## Goal

Produce `tasks_completed_per_second` for 1, 10, and 100 concurrent echo workers
on the reference rig (see `../README.md`).

## Setup

1. Start the bench stack:
   ```bash
   docker compose -f ../scripts/docker-compose.bench.yml up -d
   ```
2. Start N echo workers (one per terminal, or with `--replicas N` via docker
   compose):
   ```bash
   python3 ../scripts/worker.py --port 9100
   ```
3. Register each worker against the gateway (the worker script auto-registers
   on boot).

## Procedure

Submit **10 000** tasks as fast as the client can push. The load generator uses
`asyncio` with bounded concurrency (50 inflight by default):

```bash
python3 ../scripts/load.py \
    --rate 0 \
    --total 10000 \
    --concurrency 200 \
    --out ../results/throughput-N<workers>.csv
```

`--rate 0` means "no rate limit, push as fast as possible". The throughput
ceiling is observed by watching completed tasks/sec once the submit phase
stabilises.

## Metrics

| Metric | Definition |
|--------|------------|
| `throughput_tasks_per_sec` | tasks with `status=ok` divided by wall clock elapsed |
| `submit_p99_ms` | 99th percentile submit→ack latency |
| `complete_p99_ms` | 99th percentile submit→complete latency |
| `success_rate` | ok / total |

## Expected Shape (not a promise)

- 1 worker: bounded by worker concurrency, flat-lines around worker limit.
- 10 workers: near-linear scale until gateway becomes CPU-bound.
- 100 workers: router `best_match` scoring dominates; scale factor < linear.

Record the knee of the curve in the result summary.
