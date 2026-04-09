# Configuration

All configuration via environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `MAGIC_PORT` | `8080` | Server port |
| `MAGIC_API_KEY` | _(none)_ | API key for authentication. Minimum 32 characters. Generate: `openssl rand -hex 32` |
| `MAGIC_STORE` | _(none)_ | SQLite file path. Example: `./magic.db` |
| `MAGIC_POSTGRES_URL` | _(none)_ | PostgreSQL connection URL. Enables PostgreSQL backend with auto-migrations. Example: `postgres://user:pass@localhost/magic` |
| `MAGIC_POSTGRES_POOL_MIN` | `2` | Minimum PostgreSQL connection pool size |
| `MAGIC_POSTGRES_POOL_MAX` | `20` | Maximum PostgreSQL connection pool size |
| `MAGIC_PGVECTOR_DIM` | `1536` | Vector embedding dimension for semantic search. Must match your embedding model. |
| `MAGIC_CORS_ORIGIN` | _(none)_ | Allowed CORS origin. Example: `https://yourdomain.com` |
| `MAGIC_RATE_LIMIT_DISABLE` | `false` | Disable rate limiting (dev/testing only) |

## Storage selection

```
MAGIC_POSTGRES_URL set  →  PostgreSQL (recommended for production)
MAGIC_STORE set         →  SQLite (good for single-instance deployments)
neither                 →  In-memory (dev and testing only — data lost on restart)
```
