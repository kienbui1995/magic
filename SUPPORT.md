# Getting Support

MagiC is an open-source project maintained on a best-effort basis. This page describes where to go for each type of question.

## Quick Guide

| I want to... | Channel |
|--------------|---------|
| Report a bug | [GitHub Issues](https://github.com/kienbui1995/magic/issues/new?template=bug_report.yml) |
| Request a feature | [GitHub Issues](https://github.com/kienbui1995/magic/issues/new?template=feature_request.yml) |
| Ask a how-to or design question | [GitHub Discussions](https://github.com/kienbui1995/magic/discussions) |
| Share something I built | [GitHub Discussions → Show and Tell](https://github.com/kienbui1995/magic/discussions) |
| Report a security vulnerability | **Do not open a public issue.** See [`SECURITY.md`](SECURITY.md). |
| Report a Code of Conduct concern | See [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). |
| Get commercial support | See "Enterprise Support" below. |

## Before You Open an Issue

Please check, in this order:

1. **Existing issues** — your question may already be answered: <https://github.com/kienbui1995/magic/issues?q=is%3Aissue>
2. **Documentation** — the [README](README.md), [CLAUDE.md](CLAUDE.md), `docs/`, and the docs site cover common setup and API questions.
3. **CHANGELOG** — check [`CHANGELOG.md`](CHANGELOG.md) to see whether the behaviour you see is expected for your version.
4. **Source code** — the Go core is under `core/` and is reasonably small; grep is fast.

If you still need help, open an issue or discussion with:

- Version of MagiC (`magic version` if available, otherwise git commit / Docker tag).
- Go and Python versions, if relevant.
- OS and deployment method (binary, Docker, Helm, Railway, etc.).
- Minimal reproduction — smallest config and command sequence that shows the problem.
- Relevant logs (redact any secrets).

## Response Times (Best-Effort)

MagiC has no paid support SLA by default. The table below is a **best-effort** target during the single-maintainer period.

| Channel | Target first response |
|---------|-----------------------|
| Security advisories | 48 hours (committed — see [`SECURITY.md`](SECURITY.md)) |
| Bug reports | 3 business days |
| Feature requests | 1 week |
| Discussions | 1 week |

We may be slower during holidays, weekends, or major releases. If something is truly urgent, say so in the title and we will prioritize as able.

## Channels

### GitHub Issues

Use for concrete, reproducible bugs and for feature requests with a clear use case. Issue templates will guide you.

### GitHub Discussions

Use for anything that is not a defect in the code:

- "How do I do X with MagiC?"
- "Is this the right design for my use case?"
- "I built a worker for Y, check it out."
- "What is the roadmap for Z?"

### Security

Email **security@magic-ai-sdk.dev** or open a [GitHub Security Advisory](https://github.com/kienbui1995/magic/security/advisories/new). See [`SECURITY.md`](SECURITY.md) for scope and disclosure timeline.

### Chat (Planned)

A public chat (Discord or similar) is on the roadmap but not yet launched. When it ships, this page will be updated with an invite link. Until then, please use Discussions — it keeps answers searchable.

### Social Updates

Release announcements and project updates are posted under the GitHub Releases feed and [Discussions → Announcements](https://github.com/kienbui1995/magic/discussions).

## Enterprise Support

Commercial support, SLAs, private audits, and architectural engagements are available on request. Typical scope:

- Defined response-time SLA (business-day or 24/7).
- Named engineer(s) for incident response.
- Private security audits and patch backports.
- Architecture review and deployment assistance (on-prem, air-gapped, multi-region).
- Custom development (new adapters, connectors, integrations).

To enquire, email the project lead at the address listed in [`MAINTAINERS.md`](MAINTAINERS.md), or contact: **TODO — enterprise@magic-ai-sdk.dev (placeholder, confirm before publishing).**

This offering is separate from the open-source project. The Apache 2.0 license applies regardless of whether you have a commercial agreement.

## Contributing

If you want to help others get support, answering questions in Discussions is one of the most valuable contributions possible. See [`CONTRIBUTING.md`](CONTRIBUTING.md).
