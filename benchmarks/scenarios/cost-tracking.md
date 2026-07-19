# Scenario: Cost tracking accuracy

Verify that MagiC's `costctrl` module records the correct aggregate cost under
concurrent load — not just at steady state, but with concurrent submitters
racing against the same org budget.

## Goal

After 10 000 tasks of known cost, the reported org spend must match the
analytical ground truth to within floating-point epsilon.

```
|reported_spend − sum(task.cost)| / sum(task.cost)  <  1e-6
```

## Setup

Echo worker reports a deterministic cost (`$0.001` per call) via the
`complete` message payload. Run **5 concurrent load generators** so cost
writes are interleaved.

```bash
# 5 terminals, each:
python3 ../scripts/load.py --rate 50 --total 2000 --out costN.csv
```

Org starts with a soft budget of `$100`; each run pushes `$2` of spend so
total is `$10`, well within the limit. A second run intentionally exceeds the
limit to verify `budget.exceeded` fires exactly once.

## Procedure

1. Reset org spend: `POST /api/v1/orgs/{id}/spend/reset` (dev-only endpoint).
2. Run 5 concurrent load runs.
3. Query spend: `GET /api/v1/orgs/{id}/spend`.
4. Compare against `5 × 2000 × 0.001 = $10.000`.
5. Repeat with budget $5 and confirm `budget.exceeded` fires at/after $5.

## Metrics

| Metric | Definition |
|--------|------------|
| `cost_delta_pct` | `|reported − expected| / expected` (must be < 1e-6) |
| `budget_event_count` | number of `budget.exceeded` events (must be 1 in the overspend run) |
| `cost_write_p99_ms` | latency of the `cost.recorded` handler observed via event bus |

## Expected Shape (not a promise)

`cost_delta_pct` should be effectively zero — this is a correctness check
disguised as a benchmark. If drift appears, suspect non-atomic update in the
`costctrl` store path.
