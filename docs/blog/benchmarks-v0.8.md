# MagiC Performance Benchmarks — v0.8 Baseline

*April 18, 2026 — Preliminary results, reproduce before quoting.*

When we positioned MagiC as "Kubernetes for AI agents", the first question from
every enterprise evaluation team was the same: **how does it compare to
Temporal, Dapr Workflows, and Ray Serve?** This post publishes the first
baseline of our benchmark suite so that comparison can start happening in the
open, on shared methodology, rather than in vendor-supplied PowerPoint.

The numbers below are **preliminary**. They were produced on a synthetic run
against placeholder hardware; they describe the *shape* of the output, not a
measured result. The value of publishing now is that the **methodology,
scripts, and scenarios are frozen** and anyone can reproduce — and contradict —
our numbers. The goal for v0.9 is to replace every cell in the table below
with a real measurement that links to its `results/vX.Y.Z-*.md` file.

---

## Methodology

All benchmarks live in [`benchmarks/`](../../benchmarks/) with one scenario
per file:

- `throughput.md` — peak tasks/sec with 1 / 10 / 100 workers
- `latency.md` — p50 / p95 / p99 at a sustained 100 rps
- `fanout.md` — 100-step workflow, parallel vs sequential
- `durability.md` — retry success rate under induced worker failure
- `cost-tracking.md` — spend accounting accuracy under concurrent load

The reference rig is deliberately modest so results are reproducible on a
laptop:

- 4 physical cores, x86_64
- 8 GB RAM, NVMe SSD
- Linux 6.x, Go 1.25, Postgres 16 (loopback socket)
- Loopback networking only — we are measuring MagiC, not the NIC

Every run:

1. Spins up a clean stack via `benchmarks/scripts/docker-compose.bench.yml`
   (tmpfs-backed Postgres for deterministic cold starts).
2. Registers N echo workers that sleep 10 ms per call to simulate lightweight
   real-world work without drowning dispatch overhead.
3. Drives load from `benchmarks/scripts/load.py` (asyncio + httpx, token-bucket
   rate limiter, coordinated-omission-safe timing).
4. Records per-task CSV and a markdown summary into `benchmarks/results/`.

Each scenario is run three times; we publish the median.

---

## Preliminary results (synthetic placeholders — v0.8.0)

> These numbers are illustrative only. They are taken from the template in
> `benchmarks/results/v0.8.0-baseline.md` and exist to show the output
> structure and order of magnitude we expect. Do not cite externally until
> replaced with measured values.

| Scenario | Metric | Synthetic value |
|----------|--------|-----------------|
| Throughput (10 workers) | tasks/sec | **2,500** |
| Latency @ 100 rps | p50 / p95 / p99 ms | **12 / 28 / 45** |
| Workflow fan-out (100 steps, parallel) | wall-clock | **3.2 s** |
| Workflow fan-out (100 steps, sequential) | wall-clock | **~105 s** |
| Durability (10% fault injection) | DLQ rate / lost rate | **~0.1% / 0%** |
| Cost tracking drift | \|reported − expected\| / expected | **< 1e-6** |
| Router latency @ 1000 workers | ns/op (Go microbench) | **~400,000** |

## Comparison with other orchestration frameworks

This is the comparison the community has asked for. We are explicitly **not
populating it yet** — we want these cells filled by third parties running the
same `benchmarks/scripts/load.py` against each system, not by us eyeballing
blog posts.

| Framework | Throughput (10 workers) | p99 @ 100 rps | Fan-out 100 (parallel) |
|-----------|-------------------------|---------------|-------------------------|
| **MagiC v0.8** | pending | pending | pending |
| Temporal | TBD — awaiting community submission | TBD | TBD |
| Dapr Workflows | TBD — awaiting community submission | TBD | TBD |
| Ray Serve | TBD — awaiting community submission | TBD | TBD |

If you run MagiC alongside any of the above on the reference rig (or a
well-documented deviation), please open a PR adding a
`benchmarks/results/comparisons/<framework>-vX.Y.md` file. We will merge
honest numbers even when MagiC loses — the only thing we will reject is
undocumented setups.

---

## Reproducibility

```bash
# Go micro-benchmarks (no external deps)
make bench-go

# End-to-end load test (needs running gateway + echo workers)
docker compose -f benchmarks/scripts/docker-compose.bench.yml up -d
make bench-load
```

The `make bench` target runs the Go side only; the load tests are separate
because they need a live stack and can take minutes to stabilise.

---

## Caveats

- **Numbers vary by environment.** The reference rig is a laptop-class CPU.
  Cloud VMs with noisy neighbours will look worse; bare metal with PCIe
  Postgres will look better.
- **LLM latency is excluded.** MagiC is infrastructure; the workers call
  whatever LLM they choose. We benchmark orchestration overhead, which is
  what the framework actually controls.
- **GC pauses dominate the tail.** Go's default GC produces occasional
  50 ms+ pauses under sustained allocation. We report the raw distribution
  rather than trimming outliers; consumers can re-aggregate however they
  prefer.
- **Warm-up is excluded.** The first 30 seconds of each run are discarded so
  connection pool warm-up and JIT-style cache effects do not bias the mean.

---

## Call to action

The benchmarks are framework-independent: `load.py` talks HTTP, and any
orchestration system exposing a similar submit+poll shape can be driven the
same way. We would genuinely like:

1. **Contradictions.** If your numbers are worse than ours, file an issue —
   that is a regression we need to fix.
2. **Comparisons.** Run the same scripts against Temporal / Dapr / Ray on
   matched hardware and PR the results.
3. **New scenarios.** Multi-tenant isolation, cold start after crash, and
   cross-region dispatch are all missing from v0.8. Specs welcome.

The repo is at `github.com/kienbui1995/magic`. Benchmark specs, scripts, and
this blog post all live under source control, so numbers you publish today
will still be comparable a year from now.

*— The MagiC team*
