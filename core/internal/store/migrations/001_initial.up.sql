CREATE TABLE IF NOT EXISTS workers (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS workflows (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS teams (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS knowledge (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

-- worker_tokens has an extra token_hash column for efficient lookup
-- (TokenHash has json:"-" so it is not in the JSONB blob)
CREATE TABLE IF NOT EXISTS worker_tokens (
    id         TEXT PRIMARY KEY,
    data       JSONB NOT NULL,
    token_hash TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id TEXT PRIMARY KEY,
    data JSONB NOT NULL
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_workers_org      ON workers ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_tasks_org        ON tasks   ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_tasks_status     ON tasks   ((data->>'status'));
CREATE INDEX IF NOT EXISTS idx_audit_org        ON audit_log((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_wh_del_status    ON webhook_deliveries((data->>'status'));
CREATE INDEX IF NOT EXISTS idx_wh_del_next      ON webhook_deliveries((data->>'next_retry'));
CREATE INDEX IF NOT EXISTS idx_worker_tokens_hash ON worker_tokens(token_hash);
