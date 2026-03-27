---
title: "MagiC v0.4: Embedded Server Mode + Real Benchmark Numbers"
tags: [opensource, go, python, ai]
cover_image: https://raw.githubusercontent.com/kienbui1995/magic/main/landing-full.png
---

We shipped [MagiC v0.4](https://github.com/kienbui1995/magic) today. Two things worth talking about: embedded server mode and the benchmark suite we built to validate our performance claims.

## Background: What is MagiC?

MagiC is an open-source framework for managing fleets of AI workers. Think **Kubernetes for AI agents** — it doesn't build agents, it manages any agents built with any framework (CrewAI, LangChain, custom bots) through an open HTTP protocol.

```
         You (CEO)
          |
     MagiC Server  ←— Go, 15MB binary
    /    |    |    \
ContentBot  SEOBot  LeadBot  CodeBot
(Python)   (Node)  (Python)  (Go)
```

The core is Go. Workers are anything that speaks HTTP.

---

## What's new in v0.4

### 1. Embedded server mode

Before v0.4, using MagiC from Python required you to separately install and run the Go server. Now you can do this:

```python
from magic_ai_sdk import MagiC

with MagiC() as client:
    # Server is running. client is a MagiCClient.
    client.submit_task({"type": "summarize", "input": {"url": "..."}})
# Server stops automatically.
```

`MagiC()` downloads the correct binary for your platform on first use (Linux/macOS, amd64/arm64), caches it at `~/.magic/bin/`, and starts it in the background. Second run is instant — no download.

This is how it works under the hood:

```python
class MagiC:
    def __enter__(self) -> MagiCClient:
        binary = _get_binary(self._version)   # download if not cached
        self._proc = subprocess.Popen(
            [str(binary), "serve"],
            env={"MAGIC_PORT": str(self._port), **os.environ},
        )
        self._wait_ready()  # polls /health until 200 OK
        return MagiCClient(base_url=f"http://localhost:{self._port}")

    def __exit__(self, *_):
        self._proc.terminate()
```

The binary is pulled from GitHub Releases, which the release CI builds automatically for 4 targets on every `v*` tag:

```yaml
- run: |
    GOOS=linux  GOARCH=amd64 go build -ldflags="-s -w" -o dist/magic-linux-amd64  ./cmd/magic
    GOOS=linux  GOARCH=arm64 go build -ldflags="-s -w" -o dist/magic-linux-arm64  ./cmd/magic
    GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/magic-darwin-amd64 ./cmd/magic
    GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/magic-darwin-arm64 ./cmd/magic
```

Result: `pip install magic-ai-sdk` is all a Python developer needs. No Go toolchain required.

---

### 2. Benchmark suite

A frequent question is: "Why Go? Won't Python be fine?" We now have actual numbers.

The benchmark suite lives in `core/benchmarks/` — 13 benchmarks covering routing, registration, heartbeat, and the event bus. Run them yourself:

```bash
git clone https://github.com/kienbui1995/magic
cd magic/core
go test -bench=. -benchtime=5s -benchmem ./benchmarks/...
```

Results on an i7-12700 (20 logical cores):

| Benchmark | Latency | Throughput |
|-----------|---------|-----------|
| Route task — 10 workers | 2.8 µs | 920K tasks/s |
| Route task — 100 workers | 20 µs | 50K tasks/s |
| Route task — 1000 workers | 240 µs | 4K tasks/s |
| Worker registration | 1.9 µs | 1.3M/s |
| Heartbeat — 100 workers | 84 µs | — |
| Heartbeat — 1000 workers | 1.0 ms | — |
| Event bus (10 subscribers) | 163 ns | 12.5M/s |
| Event bus (parallel) | 77 ns | 26M/s |

A few things worth noting:

**Routing scales O(n) with worker count**, which is expected — we scan all workers to filter by capability. With 1000 workers the overhead is 240µs. An LLM call is 1–30 seconds. The orchestrator is never the bottleneck.

**Heartbeat at 1000 workers costs ~1ms per check cycle.** In practice, heartbeats happen every 30 seconds per worker, so the server processes ~33 heartbeats/second at 1000 workers. Completely trivial.

**The event bus at 26M events/second** means no async communication between modules will ever be a bottleneck.

---

### Why these numbers matter for fleet management

Every AI agent framework I've seen (AutoGen, CrewAI, Agno, LangGraph) runs agents as Python objects in a single process. There's no heartbeat because there's no persistent worker registry. There's no routing because there's no fleet.

Agno's "529x faster" benchmark measures how quickly Python objects instantiate. That's a valid metric for their architecture. MagiC measures something different: how many concurrent agents can the orchestrator manage, and how fast can it route tasks to them.

Python's GIL means routing decisions for N concurrent agents share one thread. In Go, each heartbeat runs in its own goroutine (~2KB RAM, true parallelism). The numbers reflect that.

---

## What's coming in v0.5

We're implementing zero-trust worker authentication:

- Each worker gets its own `mct_<256-bit token>` credential (not a shared API key)
- Token bound to a specific worker on first registration — can't be reused or impersonated
- Org isolation: tasks from Org A can only route to workers in Org A
- Immutable audit log: every register/heartbeat/route/complete recorded

No competitor has this at the framework level. AutoGen agents have no identity. CrewAI's `shared_memory=True` default has caused PII leaks between sessions. LangGraph gets closest with interrupt primitives but still no agent-level auth.

We'll publish the implementation and a write-up when it ships.

---

## Try it

```bash
pip install magic-ai-sdk
```

```python
from magic_ai_sdk import MagiC, Worker, capability

# Define a worker
class SummarizerBot(Worker):
    @capability(name="summarize")
    def summarize(self, task):
        # your LLM call here
        return {"summary": "..."}

# Start server + register worker
with MagiC() as client:
    bot = SummarizerBot(name="SummarizerBot", endpoint="http://localhost:9001")
    bot.start(magic_url=client.base_url)

    result = client.submit_task({
        "type": "summarize",
        "routing": {"required_capabilities": ["summarize"]}
    })
    print(result)
```

GitHub: [kienbui1995/magic](https://github.com/kienbui1995/magic)

Feedback welcome — especially on the benchmark methodology. We want to publish fair comparisons, not marketing numbers.
