# Deployment

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAGIC_PORT` | `8080` | Server port |
| `MAGIC_API_KEY` | _(none)_ | API key (min 32 chars). Generate: `openssl rand -hex 32` |
| `MAGIC_STORE` | _(none)_ | SQLite path (e.g. `./magic.db`) |
| `MAGIC_POSTGRES_URL` | _(none)_ | PostgreSQL URL — enables PostgreSQL backend |
| `MAGIC_PGVECTOR_DIM` | `1536` | Embedding dimension for semantic search |
| `MAGIC_CORS_ORIGIN` | _(none)_ | Allowed CORS origin |

## Storage Backends

MagiC auto-selects storage based on env vars:

```
MAGIC_POSTGRES_URL set  →  PostgreSQL (production)
MAGIC_STORE set         →  SQLite (single-instance)
neither                 →  In-memory (dev/testing)
```

PostgreSQL runs migrations automatically on startup — no manual schema setup needed.

## Docker

```bash
docker run -p 8080:8080 \
  -e MAGIC_API_KEY=$(openssl rand -hex 32) \
  -e MAGIC_STORE=/data/magic.db \
  -v /data:/data \
  ghcr.io/kienbui1995/magic:latest
```

## Docker Compose

```yaml
services:
  magic:
    image: ghcr.io/kienbui1995/magic:latest
    ports:
      - "8080:8080"
    environment:
      MAGIC_API_KEY: ${MAGIC_API_KEY}
      MAGIC_POSTGRES_URL: postgres://magic:secret@db:5432/magic
    depends_on:
      db:
        condition: service_healthy

  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: magic
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: magic
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "magic"]
      interval: 5s
```

## One-click deploy

[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template/magic)
[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy)
