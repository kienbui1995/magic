package knowledge

import "github.com/kienbui1995/magic/core/internal/store"

// VectorStore is an alias for store.VectorStore.
// Defined here for ergonomics — callers import knowledge, not store, for Hub operations.
type VectorStore = store.VectorStore

// SearchResult is an alias for store.VectorSearchResult.
type SearchResult = store.VectorSearchResult
