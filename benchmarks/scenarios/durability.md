# Scenario: Durability — DLQ and retry success rate

Inject worker failures and verify that MagiC retries, eventually succeeds, or
routes to the Dead Letter Queue with no silent task loss.

## Goal

Under a 10% worker failure rate, measure:

- `retry_success_rate` — fraction of failed attempts that later succeed
- `dlq_rate` — fraction of tasks that land in DLQ (exhausted retries)
- `lost_rate` — fraction with no terminal event (**must be 0**)

## Setup

Worker is started with fault injection:

```bash
python3 ../scripts/worker.py --port 9100 --fail-rate 0.1
```

At each dispatch, the worker rolls a dice and returns HTTP 500 with probability
`fail-rate`. MagiC's dispatcher retries up to `maxRetries=2`, then moves to DLQ.

## Procedure

Submit 5 000 tasks at 50 rps. Let the run drain for 30 s after the last submit
so retries can complete.

```bash
python3 ../scripts/load.py \
    --rate 50 --total 5000 \
    --drain 30 \
    --out ../results/durability.csv
```

After the run, query DLQ:

```bash
curl -s http://localhost:8080/api/v1/dlq | jq '.tasks | length'
```

## Metrics

| Metric | Definition |
|--------|------------|
| `retry_success_rate` | (tasks with ≥1 attempt_failed + final ok) / tasks with ≥1 attempt_failed |
| `dlq_rate` | DLQ size / 5000 |
| `lost_rate` | 1 − (ok + dlq) / 5000 — **MUST be 0** |

## Expected Shape (not a promise)

With 10% per-attempt failure and 3 total attempts, DLQ rate should be around
0.1³ = 0.001 (0.1%). Anything higher than 0.5% suggests retry logic regression.
`lost_rate` above zero is a correctness bug, not a performance regression.
