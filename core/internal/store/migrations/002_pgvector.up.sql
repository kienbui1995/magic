CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS knowledge_embeddings (
    id     TEXT PRIMARY KEY,
    vector vector(1536) NOT NULL,
    meta   JSONB
);

CREATE INDEX IF NOT EXISTS idx_knowledge_embeddings_vec
    ON knowledge_embeddings
    USING ivfflat (vector vector_cosine_ops)
    WITH (lists = 100);
