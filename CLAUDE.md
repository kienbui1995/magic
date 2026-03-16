# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**MagiC** (capital C = Company / Crew / Claw) is an open-source framework for managing fleets of AI workers. Think "Kubernetes for AI agents" — it doesn't build agents, it manages any agents built with any tool (CrewAI, LangChain, custom bots, etc.) through an open protocol.

- **Core:** Go (9 modules)
- **SDK:** Python (`pip install magic`) + Go SDK
- **Protocol:** MagiC Protocol (MCP²) — transport-agnostic JSON messages over HTTP/WebSocket/gRPC
- **License:** Apache 2.0

## Current Status

The project is in **pre-implementation phase**. Design spec and implementation plan are complete, no Go/Python source code exists yet.

- Design spec: `docs/superpowers/specs/2026-03-16-magic-framework-design.md`
- Implementation plan (Phase 1 — Foundation): `docs/superpowers/plans/2026-03-16-magic-plan-1-foundation.md`

## Architecture

### Planned Project Structure

```
magic/
├── core/                    # Go — MagiC framework server
│   ├── cmd/magic/           # CLI entrypoint (magic serve, magic worker list)
│   └── internal/
│       ├── protocol/        # MCP² message types & entity definitions
│       ├── store/           # Storage interface + in-memory impl
│       ├── events/          # Event bus (pub/sub)
│       ├── gateway/         # HTTP server, middleware, handlers
│       ├── registry/        # Worker registration & discovery
│       ├── router/          # Task routing engine (best_match, round_robin, cheapest)
│       ├── monitor/         # Structured logging & metrics
│       ├── orchestrator/    # Multi-step workflow DAG execution
│       ├── evaluator/       # Output quality assessment
│       ├── costctrl/        # Budget & cost tracking
│       ├── orgmgr/          # Organization, teams, RBAC
│       └── knowledge/       # Shared knowledge hub
├── sdk/python/              # Python SDK (magic_claw package)
└── examples/hello-worker/   # Minimal example worker
```

### Module Tiers (build order)

| Tier | Modules | Purpose |
|------|---------|---------|
| **Core** | Gateway, Registry, Router, Monitor | Minimum viable framework |
| **Differentiator** | Orchestrator, Evaluator, Cost Controller, Org Manager | Key value props |
| **Bonus** | Knowledge Hub | Shared context across workers |

### Core Entities (10)

Organization → Teams → Workers → Tools. Plus: Task, Workflow, Channel, Memory, Event, Plugin.

### Protocol (MCP²)

14 message types across 5 categories: Worker Lifecycle (4), Task Lifecycle (6), Collaboration (2), Direct Channel (2), Environment Access (2). All messages are JSON with fields: `protocol`, `version`, `type`, `id`, `timestamp`, `source`, `target`, `payload`.

## Planned Build Commands

Per the implementation plan, once code exists:

```bash
# Go core
cd core && go build ./cmd/magic/
cd core && go test ./...
cd core && go test ./internal/protocol/ -run TestMessageValidation  # single package/test

# Python SDK
cd sdk/python && pip install -e ".[dev]"
cd sdk/python && pytest

# Top-level Makefile (planned)
make build      # build Go binary
make test       # run all tests
make dev        # run dev server
make lint       # golangci-lint + ruff
```

## Review Server (existing)

A simple HTTP server for reviewing specs/plans with feedback:

```bash
python server.py   # serves at http://localhost:8899
```

Serves `review.html`, `spec-review.html`, `plan-review.html` with feedback APIs at `/api/feedback`, `/api/spec-feedback`, `/api/plan-feedback`.

## Key Design Decisions

- **Go for core** — performance, goroutines for concurrency, small binary, follows infra precedent (K8s, Docker)
- **Python SDK first** — AI/ML ecosystem compatibility
- **In-memory store for dev** — SQLite planned for persistence, storage interface allows swapping
- **Event bus** — all modules communicate through pub/sub events, not direct calls
- **Transport-agnostic protocol** — MCP² defines message format only; workers choose HTTP, WebSocket, or gRPC
- **LLM calls via LiteLLM Proxy** — never call model APIs directly (see global CLAUDE.md)
