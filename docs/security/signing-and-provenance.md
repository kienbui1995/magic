# Release Signing & Build Provenance

MagiC release artifacts are cryptographically signed and ship with SLSA Level 3
build provenance. This page lists the exact commands to verify them.

## What gets signed

| Artifact | Signer | Format |
|----------|--------|--------|
| `magic-{linux,darwin}-{amd64,arm64}` binaries | Sigstore cosign (keyless, OIDC) | `.cosign.bundle` next to each file |
| `checksums.sha256` | Sigstore cosign (keyless, OIDC) | `checksums.sha256.cosign.bundle` |
| `ghcr.io/kienbui1995/magic@<digest>` container image | Sigstore cosign (keyless, OIDC) | Signature in Rekor transparency log |
| Binaries (all) | SLSA GitHub Generator v2 | `multiple.intoto.jsonl` release asset |
| Container image | SLSA GitHub Generator v2 | OCI provenance attestation in GHCR |

Keyless signing means there is no long-lived private key. Each signature is
tied to the GitHub Actions OIDC identity of the release workflow and logged
to the public Rekor transparency log.

## Prerequisites

```bash
# cosign >= 2.2
brew install cosign          # or: go install github.com/sigstore/cosign/v2/cmd/cosign@latest

# slsa-verifier >= 2.6
go install github.com/slsa-framework/slsa-verifier/v2/cli/slsa-verifier@latest
```

## Verify a binary signature (cosign)

```bash
VERSION=v0.1.0
FILE=magic-linux-amd64

curl -LO https://github.com/kienbui1995/magic/releases/download/${VERSION}/${FILE}
curl -LO https://github.com/kienbui1995/magic/releases/download/${VERSION}/${FILE}.cosign.bundle

cosign verify-blob \
  --bundle "${FILE}.cosign.bundle" \
  --certificate-identity-regexp "^https://github.com/kienbui1995/magic/.github/workflows/release.yml@refs/tags/v" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "${FILE}"
```

Expected output: `Verified OK`.

## Verify the container image signature (cosign)

```bash
IMAGE=ghcr.io/kienbui1995/magic:v0.1.0

cosign verify \
  --certificate-identity-regexp "^https://github.com/kienbui1995/magic/.github/workflows/release.yml@refs/tags/v" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "${IMAGE}"
```

## Verify SLSA provenance (binary)

```bash
VERSION=v0.1.0
FILE=magic-linux-amd64

curl -LO https://github.com/kienbui1995/magic/releases/download/${VERSION}/${FILE}
curl -LO https://github.com/kienbui1995/magic/releases/download/${VERSION}/multiple.intoto.jsonl

slsa-verifier verify-artifact \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/kienbui1995/magic \
  --source-tag ${VERSION} \
  "${FILE}"
```

## Verify SLSA provenance (container)

```bash
IMAGE=ghcr.io/kienbui1995/magic
VERSION=v0.1.0

DIGEST=$(docker buildx imagetools inspect ${IMAGE}:${VERSION} --format '{{json .Manifest}}' | jq -r .digest)

slsa-verifier verify-image \
  --source-uri github.com/kienbui1995/magic \
  --source-tag ${VERSION} \
  "${IMAGE}@${DIGEST}"
```

## Key rotation policy

Keyless signing is tied to the GitHub workflow identity (OIDC). There is no
long-lived key to rotate. If the release workflow is compromised:

1. Revoke the GitHub Actions identity: disable `release.yml` on main.
2. Publish an advisory listing affected release tags.
3. Cut a new release from a clean state; old bundles remain in Rekor but the
   advisory tells consumers which ranges to reject.

## Trust-on-first-use — pin the identity

When automating verification in a CI of your own, pin to the exact workflow
path + tag pattern (as in the commands above). Do not rely on the repository
name alone — the certificate identity is what cryptographically ties a
signature to the workflow that produced it.
