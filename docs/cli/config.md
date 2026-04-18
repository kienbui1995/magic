# Config File Reference

`magic serve` reads an optional YAML config file. The file is **entirely optional** — every setting also has an environment variable and/or a safe default.

## Discovery

1. `--config <path>` / `-c <path>` — explicit, wins over auto-discovery.
2. `./magic.yaml` — auto-discovered from the working directory.
3. No file — defaults + env vars only.

## Precedence

For every setting the effective value is chosen with this priority (highest first):

1. **CLI flag** (e.g. `--config`)
2. **Environment variable** (e.g. `MAGIC_API_KEY`, `MAGIC_POSTGRES_URL`)
3. **Config file** value
4. **Built-in default** (e.g. `port: 8080`, `log_level: info`)

This means you can commit a checked-in `magic.yaml` with sensible defaults and override sensitive values in production via env vars, without editing the file.

## Env interpolation

Values inside the YAML file support `${VAR}` and `$VAR` expansion against the process environment, evaluated **before** the file is parsed:

```yaml
api_key: "${MAGIC_API_KEY}"
store:
  postgres_url: "${MAGIC_POSTGRES_URL}"
```

Missing variables expand to an empty string. Prefer the bracketed form when the value sits next to other characters.

## Full schema

See `magic.yaml.example` at the repo root for a fully-commented template. Key sections:

| Field                         | Env var                       | Default |
| ----------------------------- | ----------------------------- | ------- |
| `port`                        | `MAGIC_PORT`                  | `8080`  |
| `log_level`                   | `MAGIC_LOG_LEVEL`             | `info`  |
| `api_key`                     | `MAGIC_API_KEY`               | *(empty — auth off)* |
| `store.driver`                | *(auto-detected)*             | `memory` |
| `store.sqlite_path`           | `MAGIC_STORE`                 | — |
| `store.postgres_url`          | `MAGIC_POSTGRES_URL`          | — |
| `postgres_url` *(flat alias)* | `MAGIC_POSTGRES_URL`          | — |
| `redis_url`                   | `MAGIC_REDIS_URL`             | — |
| `llm.openai.api_key`          | `OPENAI_API_KEY`              | — |
| `llm.openai.base_url`         | `OPENAI_BASE_URL`             | — |
| `llm.anthropic.api_key`       | `ANTHROPIC_API_KEY`           | — |
| `llm.ollama.url`              | `OLLAMA_URL`                  | — |
| `oidc.issuer`                 | `MAGIC_OIDC_ISSUER`           | — |
| `oidc.client_id`              | `MAGIC_OIDC_CLIENT_ID`        | — |
| `oidc.audience`               | `MAGIC_OIDC_AUDIENCE`         | — |
| `otel.endpoint`               | `OTEL_EXPORTER_OTLP_ENDPOINT` | — |
| `otel.service_name`           | `OTEL_SERVICE_NAME`           | `magic` |
| `otel.sampler`                | `OTEL_TRACES_SAMPLER`         | — |
| `otel.sampler_arg`            | `OTEL_TRACES_SAMPLER_ARG`     | — |
| `rate_limits.register_per_minute` | —                         | gateway default |
| `rate_limits.task_per_minute`     | —                         | gateway default |
| `cors_origin`                 | `MAGIC_CORS_ORIGIN`           | — |
| `trusted_proxy`               | `MAGIC_TRUSTED_PROXY=true`    | `false` |

## Security notes

- Never commit a `magic.yaml` that contains plaintext credentials. Use env interpolation (`${MAGIC_API_KEY}`) and inject the env vars at runtime (Docker secrets, k8s Secret, systemd `EnvironmentFile`, …).
- Credentials (`MAGIC_API_KEY`, `MAGIC_POSTGRES_URL`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`) are resolved via the configured `secrets.Provider` so backends like Vault or AWS Secrets Manager can replace the env-var resolver without code changes. See `docs/security/secrets.md`.
- `MAGIC_API_KEY` must be at least 32 characters when set — generate one with `openssl rand -hex 32`.
