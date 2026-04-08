package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGVectorStore implements VectorStore using PostgreSQL pgvector extension.
// The schema is created by migration 002_pgvector.up.sql.
// Table: knowledge_embeddings(id TEXT, vector vector(N), meta JSONB)
type PGVectorStore struct {
	pool *pgxpool.Pool
	dim  int // embedding dimension, default 1536
}

// NewPGVectorStore creates a new PGVectorStore.
// dim must match the dimension set at table creation time (migration 002_pgvector).
// Default: 1536 (text-embedding-3-small). Changing dim requires recreating the table.
func NewPGVectorStore(pool *pgxpool.Pool, dim int) *PGVectorStore {
	if dim <= 0 {
		dim = 1536
	}
	return &PGVectorStore{pool: pool, dim: dim}
}

// Upsert stores or replaces an embedding for a knowledge entry.
func (s *PGVectorStore) Upsert(id string, vector []float32, meta map[string]any) error {
	if len(vector) != s.dim {
		return fmt.Errorf("vector dimension mismatch: got %d, want %d", len(vector), s.dim)
	}
	vecStr := encodeVector(vector)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO knowledge_embeddings (id, vector, meta)
         VALUES ($1, $2::vector, $3::jsonb)
         ON CONFLICT (id) DO UPDATE
             SET vector = EXCLUDED.vector, meta = EXCLUDED.meta`,
		id, vecStr, metaJSON)
	return err
}

// Search returns the top-K knowledge entries most similar to queryVector.
// Score is 1 - cosine_distance: 1.0 = identical, 0.0 = orthogonal.
func (s *PGVectorStore) Search(queryVector []float32, topK int) ([]VectorSearchResult, error) {
	if len(queryVector) != s.dim {
		return nil, fmt.Errorf("query vector dimension mismatch: got %d, want %d", len(queryVector), s.dim)
	}
	if topK <= 0 {
		topK = 10
	}
	vecStr := encodeVector(queryVector)
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, meta, 1 - (vector <=> $1::vector) AS score
         FROM knowledge_embeddings
         ORDER BY vector <=> $1::vector
         LIMIT $2`,
		vecStr, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []VectorSearchResult
	for rows.Next() {
		var id string
		var metaJSON []byte
		var score float32
		if err := rows.Scan(&id, &metaJSON, &score); err != nil {
			continue
		}
		var meta map[string]any
		_ = json.Unmarshal(metaJSON, &meta)
		results = append(results, VectorSearchResult{ID: id, Score: score, Metadata: meta})
	}
	return results, nil
}

// Delete removes the embedding for a knowledge entry ID.
func (s *PGVectorStore) Delete(id string) error {
	result, err := s.pool.Exec(context.Background(),
		"DELETE FROM knowledge_embeddings WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// encodeVector formats a []float32 slice as pgvector literal: "[x,y,z,...]"
func encodeVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Compile-time interface check.
var _ VectorStore = (*PGVectorStore)(nil)
