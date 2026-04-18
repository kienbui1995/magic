# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, email **security@magic-ai-sdk.dev** (or open a private security advisory on GitHub).

Include:
- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Suggested fix (if any)

We will acknowledge receipt within 48 hours and aim to release a fix within 7 days for critical issues.

## Scope

- MagiC core server (Go)
- Python SDK (`magic-ai-sdk`)
- Official Docker images
- API authentication and authorization

## Out of Scope

- Third-party workers or plugins
- Issues in dependencies (report upstream)

## Supply Chain Verification

All release binaries and container images are signed with Sigstore cosign
(keyless OIDC) and carry SLSA Level 3 build provenance. For exact
verification commands (cosign `verify-blob`, `verify`, `slsa-verifier
verify-artifact`, `verify-image`), see
[`docs/security/signing-and-provenance.md`](docs/security/signing-and-provenance.md).

Hardening summary:

- All GitHub Actions are pinned to immutable commit SHAs (no floating tags).
- Release binaries: `.cosign.bundle` published alongside each asset.
- Container images (`ghcr.io/kienbui1995/magic`): signed; signatures in the
  public Rekor transparency log.
- SLSA v1.0 Level 3 provenance attestations are published with every release
  via `slsa-framework/slsa-github-generator`.
- Container images are scanned with Trivy (CRITICAL/HIGH) before publish.
- CodeQL + gosec SAST + govulncheck run on every PR and push to `main`.
- OpenSSF Scorecard runs weekly and on `main` pushes.
