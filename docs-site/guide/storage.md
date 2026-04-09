# Storage Backends

MagiC auto-selects the storage backend at startup based on environment variables.

## Selection logic

```
MAGIC_POSTGRES_URL set  →  PostgreSQL (recommended for production)
MAGIC_STORE set         →  SQLite (good for single-instance deployments)
neither                 →  In-memory (dev and testing only — data lost on restart)
```

## In-memory (default)

No configuration needed. All data is lost on restart. Use for local development and testing only.

```bash
./magic serve
```

## SQLite

Persistent single-file database. Good for small deployments and single-instance setups.

```bash
MAGIC_STORE=./magic.db ./magic serve
```

## PostgreSQL

Recommended for production. Supports connection pooling, pgvector semantic search, and horizontal scaling.

```bash
MAGIC_POSTGRES_URL=postgres://user:pass@localhost/magic ./magic serve
```

### Connection pool settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAGIC_POSTGRES_POOL_MIN` | `2` | Minimum connections kept alive |
| `MAGIC_POSTGRES_POOL_MAX` | `20` | Maximum concurrent connections |

### pgvector semantic search

Enable semantic search on the knowledge hub by configuring the embedding dimension:

```bash
MAGIC_PGVECTOR_DIM=1536 ./magic serve  # default, matches OpenAI text-embedding-ada-002
```

The `pgvector` extension must be installed in your PostgreSQL instance. It's included in `pgvector/pgvector:pg16` Docker image.

### Auto-migrations

PostgreSQL runs all schema migrations automatically on startup using `golang-migrate`. No manual `CREATE TABLE` statements needed.

## Backup

```bash
# SQLite
cp magic.db magic.db.bak

# PostgreSQL
pg_dump -U magic magic > backup.sql
```
