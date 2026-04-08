package store

// VectorStore defines the interface for vector similarity search.
// It is separate from Store to keep vector concerns focused.
// PGVectorStore implements this interface.
// When nil, semantic search in Hub returns an error.
type VectorStore interface {
	// Upsert stores or replaces an embedding for a knowledge entry ID.
	Upsert(id string, vector []float32, meta map[string]any) error
	// Search returns the top-K most similar entries to queryVector by cosine similarity.
	Search(queryVector []float32, topK int) ([]VectorSearchResult, error)
	// Delete removes the embedding for a knowledge entry ID.
	Delete(id string) error
}

// VectorSearchResult is returned by VectorStore.Search.
type VectorSearchResult struct {
	ID       string         `json:"id"`
	Score    float32        `json:"score"`    // 0.0–1.0 cosine similarity
	Metadata map[string]any `json:"metadata,omitempty"`
}
