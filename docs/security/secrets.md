# Secret Management

MagiC reads sensitive configuration (API keys, database URLs, LLM credentials)
through a pluggable `SecretProvider` abstraction defined in
`core/internal/secrets`. The default is a zero-dependency environment-variable
provider; enterprise deployments can plug in HashiCorp Vault or AWS Secrets
Manager without changing call sites.

## Providers

| Provider | `MAGIC_SECRETS_PROVIDER` | Status | Dependencies |
|----------|--------------------------|--------|--------------|
| Env      | `env` (default)          | Ready  | none |
| Vault    | `vault`                  | Stub   | `github.com/hashicorp/vault/api` (vendor to enable) |
| AWS      | `aws`                    | Stub   | `github.com/aws/aws-sdk-go-v2/service/secretsmanager` (vendor to enable) |

Stubs validate config at construction and return `ErrProviderUnavailable`
from `Get()` with a pointer back to this document. They intentionally do
not dial the backend — startup never blocks on a secret store.

## Environment Variables

### Selection

- `MAGIC_SECRETS_PROVIDER` — one of `env`, `vault`, `aws`. Empty = `env`.

### Vault (`vault`)

- `MAGIC_VAULT_ADDR` (required) — e.g. `https://vault.example.com:8200`
- `MAGIC_VAULT_TOKEN` (required) — prefer a token helper in production
- `MAGIC_VAULT_MOUNT` (default `secret`) — KVv2 mount point
- `MAGIC_VAULT_PATH` — base path under the mount, e.g. `magic/prod`

### AWS Secrets Manager (`aws`)

- `AWS_REGION` (required) — e.g. `ap-southeast-1`
- `MAGIC_AWS_SECRETS_PREFIX` — e.g. `magic/prod/`; prepended to the
  secret name before lookup

## Implementing a Production Provider

The `Provider` interface is intentionally small:

```go
type Provider interface {
    Get(ctx context.Context, name string) (string, error)
    Name() string
}
```

To enable Vault, vendor `github.com/hashicorp/vault/api` and replace the
stub body in `core/internal/secrets/vault.go`:

```go
func (v *VaultProvider) Get(ctx context.Context, name string) (string, error) {
    client, err := vault.NewClient(&vault.Config{Address: v.cfg.Address})
    if err != nil { return "", err }
    client.SetToken(v.cfg.Token)
    sec, err := client.KVv2(v.cfg.Mount).Get(ctx, path.Join(v.cfg.Path, name))
    if err != nil { return "", err }
    raw, ok := sec.Data["value"].(string)
    if !ok { return "", secrets.ErrNotFound }
    return raw, nil
}
```

For AWS, vendor `aws-sdk-go-v2` and implement `GetSecretValue` analogously
in `aws.go`. Re-use the existing config struct and `ErrNotFound` /
`ErrProviderUnavailable` sentinels.

## ChainProvider — Layered Lookups

`secrets.NewChainProvider(primary, fallback, ...)` walks providers in
priority order and returns the first hit. `ErrNotFound` falls through;
any other error (including `ErrProviderUnavailable`) short-circuits so
misconfiguration is never silently masked.

Recommended pattern for mixed dev/prod environments:

```go
p := secrets.NewChainProvider(
    secrets.NewEnvProvider(),   // dev overrides / CI
    vault,                      // production source of truth
)
```

## Migration Path: env → vault without downtime

1. Deploy the binary with `MAGIC_SECRETS_PROVIDER=env` (current behavior).
2. Populate Vault with the same keys. Keep env vars set as well.
3. Switch to `ChainProvider(env, vault)` — env still wins, but vault is
   primed and any missing keys fall through.
4. Flip to `MAGIC_SECRETS_PROVIDER=vault` once Vault coverage is verified.
5. Remove the env vars from the deployment and rotate secrets.

## Rotation (Future Work)

The abstraction does not yet expose a `Watch` / `OnRotate` callback.
When a rotation story is needed, add:

```go
type RotatingProvider interface {
    Provider
    Watch(ctx context.Context, name string) (<-chan string, error)
}
```

Vault's lease renewal and AWS Secrets Manager's rotation schedule both
map to this shape. Callers type-assert for the capability.

## Migrated Call Sites

The following credentials now flow through `secrets.Provider` and are
resolved at server startup in `cmd/magic/main.go` via
`config.LoadWithSecrets(ctx, path, sp)`:

| Secret name          | Purpose                         | Consumer |
|----------------------|---------------------------------|----------|
| `MAGIC_API_KEY`      | Admin API-key for gateway auth  | `gateway.authMiddleware` (read once; captured in closure) |
| `MAGIC_POSTGRES_URL` | PostgreSQL connection string    | `store.NewPostgreSQLStore` |
| `OPENAI_API_KEY`     | OpenAI LLM provider             | `llm.NewOpenAIProvider` |
| `ANTHROPIC_API_KEY`  | Anthropic LLM provider          | `llm.NewAnthropicProvider` |

With `MAGIC_SECRETS_PROVIDER=env` (default) the behavior is identical
to reading `os.Getenv` directly. To source any of these from Vault or
AWS Secrets Manager, implement the corresponding provider and set
`MAGIC_SECRETS_PROVIDER`; no code changes are required at the call
sites.

### Non-secret knobs (stay on `os.Getenv`)

These are operational knobs, not credentials, and continue to read
`os.Getenv` directly:

- `MAGIC_PORT`, `MAGIC_TRUSTED_PROXY`, `MAGIC_CORS_ORIGIN`
- `MAGIC_POSTGRES_POOL_MIN`, `MAGIC_POSTGRES_POOL_MAX`, `MAGIC_PGVECTOR_DIM`
- `MAGIC_STORE` (SQLite path), `OPENAI_BASE_URL`, `OLLAMA_URL`
- `MAGIC_RATE_LIMIT_DISABLE`, `MAGIC_REDIS_URL`, `MAGIC_OIDC_*`
- OpenTelemetry standard env vars (`OTEL_*`)

Only genuine credentials go through the provider.
