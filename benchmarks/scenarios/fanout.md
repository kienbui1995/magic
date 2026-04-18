# Scenario: Workflow fan-out (parallel vs sequential)

Compare wall-clock time for a 100-step workflow executed (a) sequentially vs
(b) fully parallel. This is the flagship comparison against Temporal activity
fan-out and Dapr workflow children.

## Goal

Two numbers per MagiC release:

- `workflow_seq_100_ms` — 100 echo steps with `depends_on` chained linearly.
- `workflow_par_100_ms` — 100 echo steps with no dependencies.

## Setup

Bench stack with **20 workers** (parallel case needs enough workers so scheduler
is not the bottleneck). Each echo step adds 10 ms artificial latency inside the
worker so dispatch overhead is visible without being drowned by sleep.

## Procedure

Submit workflow JSON via `POST /api/v1/workflows`. Two fixtures live in this
directory:

- `fanout-seq-100.json` — 100 steps, each `depends_on: [previous]`
- `fanout-par-100.json` — 100 steps, all independent

```bash
curl -X POST http://localhost:8080/api/v1/workflows \
    -H 'Authorization: Bearer $TOKEN' \
    -d @fanout-par-100.json
```

Wait for `workflow.completed` via SSE and record the total elapsed.

## Metrics

| Metric | Definition |
|--------|------------|
| `workflow_seq_100_ms` | wall-clock: submit → workflow.completed (sequential) |
| `workflow_par_100_ms` | wall-clock: submit → workflow.completed (parallel) |
| `parallel_efficiency` | `seq_ms / (par_ms * 100)` — 1.0 means perfect scaling |

## Expected Shape (not a promise)

Sequential should be ~ (100 × per-step overhead + 100 × 10 ms sleep).
Parallel should approach (1 × per-step overhead + 1 × 10 ms sleep) plus
dispatch fan-out cost. If `parallel_efficiency` < 0.8, investigate router
contention or DB write amplification.
