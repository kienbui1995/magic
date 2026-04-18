# Maintainers

This document lists the current maintainers of MagiC and the modules they own.

For how to become a maintainer, see [`GOVERNANCE.md`](GOVERNANCE.md#becoming-a-maintainer).

## Active Maintainers

| Name | GitHub | Role | Areas of Expertise | Timezone |
|------|--------|------|--------------------|----------|
| Kien Bui | [@kienbui1995](https://github.com/kienbui1995) | Project Lead / BDFL | Core architecture, protocol, Go server, release engineering | Asia/Ho_Chi_Minh (UTC+7) |

## Module Ownership

Code owners for each major area of the repository. For the authoritative machine-readable version, see [`.github/CODEOWNERS`](.github/CODEOWNERS).

| Area | Path | Owner(s) |
|------|------|----------|
| Gateway (HTTP, middleware, auth) | `core/internal/gateway/` | @kienbui1995 |
| Protocol (MCP² types and messages) | `core/internal/protocol/` | @kienbui1995 |
| Storage (Memory / SQLite / PostgreSQL) | `core/internal/store/` | @kienbui1995 |
| Registry, Router, Dispatcher | `core/internal/{registry,router,dispatcher}/` | @kienbui1995 |
| Orchestrator (workflow DAG) | `core/internal/orchestrator/` | @kienbui1995 |
| Evaluator | `core/internal/evaluator/` | @kienbui1995 |
| Cost Controller | `core/internal/costctrl/` | @kienbui1995 |
| Org Manager / RBAC / Policy | `core/internal/{orgmgr,rbac,policy}/` | @kienbui1995 |
| Knowledge Hub | `core/internal/knowledge/` | @kienbui1995 |
| LLM Gateway / Prompt Registry / Agent Memory | `core/internal/{llm,prompt,memory}/` | @kienbui1995 |
| Webhooks | `core/internal/webhook/` | @kienbui1995 |
| Audit | `core/internal/audit/` | @kienbui1995 |
| Monitor / Metrics / Tracing | `core/internal/{monitor,tracing}/` | @kienbui1995 |
| Python SDK | `sdk/python/` | @kienbui1995 |
| Go SDK | `sdk/go/` | @kienbui1995 |
| TypeScript SDK | `sdk/typescript/` | @kienbui1995 |
| Documentation site | `docs-site/`, `docs/` | @kienbui1995 |
| Deploy manifests (Helm, Compose, Railway) | `deploy/` | @kienbui1995 |
| CI and release workflows | `.github/workflows/` | @kienbui1995 |
| Examples | `examples/` | @kienbui1995 |

The project currently has a single maintainer. Module ownership will broaden as the community grows and new maintainers are added per the [Governance](GOVERNANCE.md#becoming-a-maintainer) process.

## Want to Become a Maintainer?

We welcome additional maintainers who share the project's mission and have demonstrated sustained contribution. See the criteria and nomination process in [`GOVERNANCE.md`](GOVERNANCE.md#becoming-a-maintainer).

In short:

- Ship meaningful PRs and reviews over 3+ months.
- Help in issues and Discussions.
- Care about operability, docs, and community health — not just code.
- Open a `good first issue` or pick something from the roadmap to get started.

## Emeritus Maintainers

Maintainers who have stepped back from active work but whose past contributions shaped the project.

_None yet._

## Contact

For project-wide questions, open a [GitHub Discussion](https://github.com/kienbui1995/magic/discussions). For security issues, see [`SECURITY.md`](SECURITY.md). For Code of Conduct concerns, see [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).
