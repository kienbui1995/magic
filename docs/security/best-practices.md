# Security Best Practices

MagiC is infrastructure for managing AI workloads. Treating the supply chain, secrets, and release pipeline with rigor is a first-class concern. This document describes the practices enforced in this repository and the ones expected of contributors.

## Supply chain security

We run layered scanning on every pull request and on a weekly schedule.

- **Dependabot** (`.github/dependabot.yml`) — opens weekly PRs for Go modules (`core/`, `sdk/go/`), Python (`sdk/python/`), npm (`sdk/typescript/`, root VitePress), Docker base images, and GitHub Actions. PRs are grouped and auto-labeled `dependencies`.
- **govulncheck** (`ci.yml` job) — scans Go modules for known CVEs from the Go vulnerability database on every PR. Runs against `core` and `sdk/go` so a shared dependency cannot slip in.
- **gosec** (`ci.yml` job) — static application security testing for Go. Uploads SARIF to GitHub code scanning so findings show up in the Security tab.
- **CodeQL** (`codeql.yml`) — semantic code analysis for Go and JavaScript. Detects injection, data flow, and other deep issues beyond pattern-based SAST.
- **OpenSSF Scorecard** (`scorecard.yml`) — weekly benchmark of repository security posture (branch protection, pinned actions, token permissions, signed releases). Score is published on the README badge.

Each job runs as an independent workflow job so a single failure does not take the whole pipeline down.

## Secret handling

- **Never commit secrets.** `.env`, `.env.*`, and files matching `*credentials*` are in `.gitignore`. The CI job that publishes to PyPI uses OIDC (no long-lived token). GHCR uses the ephemeral `GITHUB_TOKEN`.
- **GitHub repository secrets** are the only place long-lived credentials live. `NPM_TOKEN` is scoped to the typescript package only. Rotate yearly.
- **Environment validation** — production backends must validate all required env vars at startup (Pydantic Settings for Python, typed config struct for Go) so misconfiguration fails fast.
- If you believe a secret has leaked, rotate immediately and open an issue using the template `security-disclosure`. Do not file public issues for active credential leaks — follow `SECURITY.md`.

## Commits and releases

- **Signed commits recommended** — maintainers sign all merges to `main` with GPG or SSH. Contributors are encouraged but not required to sign. We plan to make signing mandatory for `main` once the contributor community is fully onboarded.
- **Conventional Commits** — commit messages follow `feat(scope): …`, `fix(scope): …`, etc. Release notes are generated from the commit log.
- **Release artifacts** — binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 are built in the release workflow, checksummed (SHA-256), and published to GitHub Releases. Container images are pushed to GHCR for the same arch pair.
- **Future work — artifact signing.** We plan to add Sigstore cosign signing for both container images and binaries, plus SLSA build provenance attestation, so downstream consumers can verify the build came from our CI.

## Vulnerability disclosure

Security issues must not be filed as public GitHub issues. Follow the contact and disclosure policy in [`SECURITY.md`](../../SECURITY.md). We aim to acknowledge within 48 hours and patch critical issues within 7 days.
