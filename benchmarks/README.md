# MagiC Benchmarks

Performance benchmarks for the MagiC AI agent orchestration framework.

> **Note:** LLM latency is NOT measured here. These benchmarks cover orchestration
> overhead only: worker registration, task routing, event dispatch. The numbers
> represent what MagiC adds on top of your existing agents.

## Scope

The suite targets the dimensions that matter for enterprise comparison against
Temporal / Dapr Workflows / Ray Serve:

| Dimension | What we measure | Where |
|-----------|-----------------|-------|
| **Throughput** | Tasks completed per second, 1/10/100 workers | `scenarios/throughput.md` |
| **Latency** | p50/p95/p99 dispatch latency under sustained load | `scenarios/latency.md` |
| **Fan-out** | Parallel vs sequential workflow step execution | `scenarios/fanout.md` |
| **Durability** | DLQ recovery + retry success under induced failures | `scenarios/durability.md` |
| **Cost accuracy** | Cost accounting correctness under load | `scenarios/cost-tracking.md` |
| **Scalability** | Route time at 1 → 1000 registered workers | `core/benchmarks/routing_test.go` |

## Hardware Recipe (reproducibility)

Results published in `results/` must be produced on — or clearly labelled
deviations from — this baseline rig:

- CPU: 4 physical cores, x86_64
- RAM: 8 GB
- Disk: local NVMe SSD
- OS: Linux kernel 6.x, cgroups v2
- Go: **1.25**
- Postgres: **16** (with `pgvector`), local socket
- Network: loopback only (no cross-host NIC)
- MagiC version: tagged release (see file name `results/vX.Y.Z-*.md`)

Run each scenario **three times** and publish the median. Note any deviation
(CPU model, cloud instance) in the result header.

## Output Format

Load-test scenarios emit two artefacts:

1. **CSV** — one row per task: `timestamp,task_id,submit_ms,complete_ms,status`
2. **Markdown summary** — aggregates in `results/vX.Y.Z-<scenario>.md`
   including methodology, p50/p95/p99, throughput, success rate, observations.

## Versioning

Benchmarks are pinned to the MagiC release they ran against. File naming:

```
results/v0.8.0-baseline.md
results/v0.9.0-baseline.md
```

Never overwrite historic results; append new runs as new files so regressions
are visible over time.

## Location

Benchmark files live inside the `core` module at `../core/benchmarks/` because
Go's `internal` package visibility rules require benchmark code to be within
the same module as the packages it tests.

## How to Run

```bash
# Run all benchmarks (5 seconds per benchmark, recommended for stable numbers)
cd ../core
go test -bench=. -benchtime=5s -benchmem ./benchmarks/...

# Quick run (1 second per benchmark)
go test -bench=. -benchtime=1s -benchmem ./benchmarks/...

# Run a specific benchmark
go test -bench=BenchmarkTaskRouting_1000Workers -benchtime=5s -benchmem ./benchmarks/...

# CPU profiling
go test -bench=BenchmarkTaskRouting_1000Workers -cpuprofile=cpu.prof ./benchmarks/...
go tool pprof cpu.prof

# Memory profiling
go test -bench=BenchmarkWorkerLookup -memprofile=mem.prof ./benchmarks/...
go tool pprof mem.prof
```

## What Each Benchmark Measures

### routing_test.go — Task Routing

| Benchmark | Measures |
|-----------|---------|
| `BenchmarkTaskRouting_10Workers` | Route 1 task across 10 active workers (best_match strategy) |
| `BenchmarkTaskRouting_100Workers` | Route 1 task across 100 active workers |
| `BenchmarkTaskRouting_1000Workers` | Route 1 task across 1000 active workers (scalability ceiling) |
| `BenchmarkTaskRoutingParallel_100Workers` | Concurrent routing with 100 workers (GOMAXPROCS goroutines) |
| `BenchmarkTaskRoutingParallel_1000Workers` | Concurrent routing with 1000 workers |

The routing pipeline per call: `ListWorkers` → `filterByCapability` → `scoreBestMatch` → `store.UpdateWorker` → `bus.Publish`.

### registry_test.go — Worker Registry

| Benchmark | Measures |
|-----------|---------|
| `BenchmarkWorkerRegistration` | Sequential: register 1 worker (ID generation + store write + event publish) |
| `BenchmarkWorkerRegistration_Parallel` | Concurrent registration from GOMAXPROCS goroutines |
| `BenchmarkHeartbeat_100Workers` | One round of 100 concurrent heartbeats (store read + write per worker) |
| `BenchmarkHeartbeat_1000Workers` | One round of 1000 concurrent heartbeats |
| `BenchmarkWorkerLookup` | Find all workers with a given capability from a 1000-worker registry |

### events_test.go — Event Bus

| Benchmark | Measures |
|-----------|---------|
| `BenchmarkEventBus_Publish` | Publish 1 event to 10 subscribers (channel enqueue) |
| `BenchmarkEventBus_PublishParallel` | Concurrent publish from GOMAXPROCS goroutines (channel contention) |
| `BenchmarkEventBus_FanOut` | Publish 1 event to 100 subscribers (fan-out overhead) |

The event bus is async: `Publish` enqueues to a buffered channel and returns immediately. The ns/op numbers here represent the enqueue cost, not handler execution time.

## Baseline Numbers (TBD)

These will be filled in by CI after the first run on the reference machine.

| Benchmark | Expected ns/op | Expected allocs/op |
|-----------|---------------|-------------------|
| `BenchmarkTaskRouting_10Workers` | TBD | TBD |
| `BenchmarkTaskRouting_100Workers` | TBD | TBD |
| `BenchmarkTaskRouting_1000Workers` | TBD | TBD |
| `BenchmarkTaskRoutingParallel_100Workers` | TBD | TBD |
| `BenchmarkTaskRoutingParallel_1000Workers` | TBD | TBD |
| `BenchmarkWorkerRegistration` | TBD | TBD |
| `BenchmarkWorkerRegistration_Parallel` | TBD | TBD |
| `BenchmarkHeartbeat_100Workers` | TBD | TBD |
| `BenchmarkHeartbeat_1000Workers` | TBD | TBD |
| `BenchmarkWorkerLookup` | TBD | TBD |
| `BenchmarkEventBus_Publish` | TBD | TBD |
| `BenchmarkEventBus_PublishParallel` | TBD | TBD |
| `BenchmarkEventBus_FanOut` | TBD | TBD |

## Comparing Against Python Frameworks

MagiC is written in Go. Python-based frameworks (AutoGen, CrewAI, LangGraph) have
higher orchestration overhead due to the GIL, dynamic dispatch, and async event loops.

To benchmark a Python framework for comparison, measure the same operations:
- Time to register N agents
- Time to route/assign 1 task to the right agent
- Time to dispatch 1 event to N subscribers

MagiC benchmarks intentionally exclude LLM call latency (typically 500ms–5s)
because that cost is identical regardless of the orchestration framework.

## Design Notes

- Each benchmark creates a fresh stack (registry + router + event bus) in setup.
- `b.ResetTimer()` is called after setup so setup cost is excluded from ns/op.
- Workers use `MaxConcurrentTasks: 0` (unlimited) to avoid load-limit filtering
  during high-iteration runs.
- The event bus uses a large buffer (`NewBusWithConfig(64, 1<<20)`) to prevent
  buffer saturation from masking routing/registration latency. The separate event
  bus benchmarks directly measure publish throughput.
