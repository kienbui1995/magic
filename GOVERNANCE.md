# MagiC Governance

This document describes how the MagiC open-source project is governed — how decisions are made, how roles are assigned, and how the community evolves the project over time.

MagiC is licensed under [Apache 2.0](LICENSE) and welcomes contributions from anyone. Governance is intentionally lightweight for now and will formalize as the project grows.

## Mission

**Make it easy to run fleets of AI workers at any scale — open, transport-agnostic, and vendor-neutral.**

MagiC is infrastructure. It does not build AI agents; it manages them. Our north star is to be to AI agents what Kubernetes is to containers: boring, dependable, composable.

Guiding principles:

- **Open by default** — the protocol (MCP²) and core are Apache 2.0. No feature is gated behind a commercial tier in the open-source distribution.
- **Vendor-neutral** — we do not favor any LLM, vector DB, or worker framework. Adapters are pluggable.
- **Operational realism** — every feature must be operable in production: observable, testable, upgradeable, backup-able.
- **Small, sharp primitives** — prefer a clean protocol + small core over a monolith with many opinions.

## Roles

| Role | Description | How to become one |
|------|-------------|-------------------|
| **User** | Runs MagiC, reports bugs, asks questions in Discussions. | Just use the project. |
| **Contributor** | Submits pull requests, issues, or documentation. | Open a PR. |
| **Committer** | Has write access to a specific module or area. Reviews PRs in that area. | Sustained contributions + nomination by a Maintainer. |
| **Maintainer** | Has merge rights across the repo. Shapes roadmap. Enforces CoC. | See "Becoming a Maintainer" below. |
| **Steering / BDFL** | Final call on contested decisions. Currently the project lead. | Will transition to a Steering Committee once the project has 3+ active Maintainers. |

Committer-level access is granted per directory via [`.github/CODEOWNERS`](.github/CODEOWNERS). Maintainers are listed in [`MAINTAINERS.md`](MAINTAINERS.md).

## Decision Making

We use **lazy consensus** for most decisions:

1. A change is proposed (PR, issue, RFC).
2. If no one objects within a reasonable review window (typically 72 hours for non-trivial changes, 24 hours for trivial ones), the change is assumed accepted.
3. A single approving review from a relevant Maintainer is sufficient to merge.

For changes that are **non-trivial, controversial, or breaking**, we require:

- An issue or design doc under `docs/superpowers/specs/` describing the motivation, alternatives, and migration path.
- At least **two** approving reviews from different Maintainers.
- A **7-day comment window** before merge, explicitly announced in the PR body.

If lazy consensus breaks down (someone objects and agreement cannot be reached), the decision escalates in this order:

1. The PR author and reviewers attempt to resolve in the PR conversation.
2. If unresolved, the Maintainers discuss in a tracking issue or async thread.
3. If still unresolved, the project lead (BDFL) makes the final call. The decision is documented in the issue and linked from the CHANGELOG.

## Release Cadence

We follow [Semantic Versioning](https://semver.org/).

| Type | Cadence | Contents |
|------|---------|----------|
| **Minor** (`0.x.0`, `x.Y.0`) | Roughly every 6 weeks | New features, additive API changes, non-breaking protocol evolution. |
| **Patch** (`x.y.Z`) | On demand | Bug fixes, security patches, documentation fixes. Same-day for critical security fixes. |
| **Major** (`X.0.0`) | When necessary | Breaking changes. Requires a deprecation cycle (see [Upgrade Guide](docs/ops/upgrade-path.md)). |

Before 1.0.0, we may introduce breaking changes in minor releases, but we commit to documenting them in [`CHANGELOG.md`](CHANGELOG.md) with clear migration notes.

Each release:

1. A release PR updates `CHANGELOG.md` with the version number and date.
2. CI passes on `main`.
3. A Maintainer tags the release (`v0.x.y`) and GitHub Actions publishes the Go binary, Docker image, and SDK packages.
4. The release is announced in GitHub Discussions.

## Becoming a Maintainer

MagiC maintainership is earned through sustained contribution, technical depth, and alignment with the project's mission.

Criteria (non-exhaustive):

- **Sustained contributions** over 3+ months: merged PRs, reviews, triage, documentation, support in Discussions.
- **Technical depth** in at least one area (core, SDK, docs, infrastructure) and working knowledge of the overall architecture.
- **Community participation**: helpful tone, enforcing the Code of Conduct, mentoring newer contributors.
- **Alignment** with the mission and principles above.

Nomination process:

1. An existing Maintainer opens a private discussion with the other Maintainers proposing the nominee.
2. Maintainers have 7 days to raise objections.
3. If there are no blocking objections, the nominee is offered maintainership.
4. If accepted, they are added to [`MAINTAINERS.md`](MAINTAINERS.md) and to the `@maintainers` GitHub team.

There is no fixed ratio of PRs or lines of code. Judgment is holistic.

## Removing a Maintainer

Maintainers may step down at any time by opening a PR that moves their entry to the "Emeritus" section of `MAINTAINERS.md`.

Involuntary removal is reserved for:

- Serious or repeated Code of Conduct violations.
- Extended inactivity (12+ months with no contributions or review) without a sabbatical notice.
- Actions that materially harm the project or its users.

Removal requires agreement from a majority of remaining Maintainers (excluding the subject of removal). The reasoning is documented in a private discussion and, where appropriate, summarized publicly.

## Conflict Resolution

1. **Code of Conduct issues** → report to security@magic-ai-sdk.dev or any Maintainer. See [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). CoC issues are handled confidentially.
2. **Technical disagreements** → try to resolve in the PR or issue thread first. Escalate to Maintainers if stuck. Last resort is the project lead.
3. **Governance disputes** → raise in a GitHub Discussion under the "Governance" category. Maintainers will respond within 14 days.

The [Code of Conduct](CODE_OF_CONDUCT.md) (Contributor Covenant v2.1) applies in all project spaces — GitHub, Discord (when launched), mailing lists, events, and private channels related to the project.

## Security

Security vulnerabilities are handled through a separate channel to protect users before a fix is public. See [`SECURITY.md`](SECURITY.md).

Summary: email **security@magic-ai-sdk.dev** or open a private security advisory on GitHub. Do **not** open public issues for security bugs.

## Trademarks

"MagiC" and the MagiC logo are currently held by the project lead (Kien) on behalf of the project. Usage is permitted for:

- Referring to the MagiC project in documentation, articles, and talks.
- Showing the logo alongside "Works with MagiC" or similar factual statements.

Usage is **not** permitted for:

- Naming a competing product or service that could be confused with MagiC.
- Implying official endorsement without written permission.

A formal trademark policy will be published if the project transfers to a foundation.

## Changes to This Document

Changes to `GOVERNANCE.md` require a PR with a 14-day comment window and approval from at least two Maintainers (or the project lead during the single-Maintainer period).

## Acknowledgements

This governance model draws from the practices of [Kubernetes](https://github.com/kubernetes/community/blob/master/governance.md), [Envoy](https://github.com/envoyproxy/envoy/blob/main/GOVERNANCE.md), and [OpenTelemetry](https://github.com/open-telemetry/community/blob/main/community-membership.md). We thank those communities for documenting their patterns publicly.
