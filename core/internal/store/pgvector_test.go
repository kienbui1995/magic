package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/kienbui1995/magic/core/internal/store"
)

func TestPGVectorStore_UpsertAndSearch(t *testing.T) {
	url := os.Getenv("MAGIC_POSTGRES_URL")
	if url == "" {
		t.Skip("MAGIC_POSTGRES_URL not set — skipping pgvector integration tests")
	}
	if err := store.RunMigrations(url); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	pgStore, err := store.NewPostgreSQLStore(context.Background(), url)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	defer pgStore.Close()

	vs := store.NewPGVectorStore(pgStore.Pool(), 3) // dim=3 for test

	// Clean up any previous test data
	vs.Delete("e-test-1") //nolint:errcheck
	vs.Delete("e-test-2") //nolint:errcheck

	// Upsert two vectors
	v1 := []float32{1, 0, 0}
	v2 := []float32{0, 1, 0}
	if err := vs.Upsert("e-test-1", v1, map[string]any{"label": "x-axis"}); err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}
	if err := vs.Upsert("e-test-2", v2, map[string]any{"label": "y-axis"}); err != nil {
		t.Fatalf("Upsert v2: %v", err)
	}

	// Search with query closest to v1
	query := []float32{0.9, 0.1, 0}
	results, err := vs.Search(query, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}
	if results[0].ID != "e-test-1" {
		t.Errorf("expected e-test-1 as top result, got %s", results[0].ID)
	}
	if results[0].Score < 0.9 {
		t.Errorf("expected high similarity score, got %f", results[0].Score)
	}

	// Delete
	if err := vs.Delete("e-test-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := vs.Delete("e-test-2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestPGVectorStore_DimensionMismatch(t *testing.T) {
	url := os.Getenv("MAGIC_POSTGRES_URL")
	if url == "" {
		t.Skip("MAGIC_POSTGRES_URL not set")
	}
	pgStore, err := store.NewPostgreSQLStore(context.Background(), url)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	defer pgStore.Close()

	vs := store.NewPGVectorStore(pgStore.Pool(), 3)

	err = vs.Upsert("bad", []float32{1, 0}, nil) // wrong dim
	if err == nil {
		t.Error("expected dimension mismatch error")
	}
}
