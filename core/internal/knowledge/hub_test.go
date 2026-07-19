package knowledge_test

import (
	"context"
	"testing"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/knowledge"
	"github.com/kienbui1995/magic/core/internal/store"
)

func TestHub_Add(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus, nil)

	entry, err := hub.Add(context.Background(), "API Guidelines", "Use REST conventions", []string{"api", "rest"}, "org", "org_magic", "admin")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if entry.ID == "" {
		t.Error("ID should not be empty")
	}
	if entry.Title != "API Guidelines" {
		t.Errorf("Title: got %q", entry.Title)
	}
}

func TestHub_Get(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus, nil)

	entry, _ := hub.Add(context.Background(), "Test", "Content", nil, "org", "org_magic", "admin")

	got, err := hub.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Test" {
		t.Errorf("Title: got %q", got.Title)
	}
}

func TestHub_Search(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus, nil)

	hub.Add(context.Background(), "API Guidelines", "REST conventions", []string{"api"}, "org", "org_magic", "admin")
	hub.Add(context.Background(), "Database Guide", "Use PostgreSQL", []string{"database"}, "org", "org_magic", "admin")

	results := hub.Search(context.Background(), "API")
	if len(results) != 1 {
		t.Errorf("Search 'API': got %d, want 1", len(results))
	}

	results = hub.Search(context.Background(), "database")
	if len(results) != 1 {
		t.Errorf("Search 'database': got %d, want 1", len(results))
	}
}

func TestHub_Update(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus, nil)

	entry, _ := hub.Add(context.Background(), "Old Title", "Old content", nil, "org", "org_magic", "admin")

	err := hub.Update(context.Background(), entry.ID, "New Title", "New content", []string{"updated"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := hub.Get(context.Background(), entry.ID)
	if got.Title != "New Title" {
		t.Errorf("Title: got %q", got.Title)
	}
	if got.Content != "New content" {
		t.Errorf("Content: got %q", got.Content)
	}
}

func TestHub_Delete(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus, nil)

	entry, _ := hub.Add(context.Background(), "To Delete", "Content", nil, "org", "org_magic", "admin")

	err := hub.Delete(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = hub.Get(context.Background(), entry.ID)
	if err == nil {
		t.Error("should fail after delete")
	}
}

func TestHub_List(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus, nil)

	hub.Add(context.Background(), "Entry 1", "Content 1", nil, "org", "org_magic", "admin")
	hub.Add(context.Background(), "Entry 2", "Content 2", nil, "team", "team_marketing", "admin")

	entries := hub.List(context.Background())
	if len(entries) != 2 {
		t.Errorf("List: got %d, want 2", len(entries))
	}
}

// mockVectorStore for testing
type mockVectorStore struct {
	upserted map[string][]float32
	results  []knowledge.SearchResult
}

func (m *mockVectorStore) Upsert(id string, vector []float32, meta map[string]any) error {
	if m.upserted == nil {
		m.upserted = make(map[string][]float32)
	}
	m.upserted[id] = vector
	return nil
}

func (m *mockVectorStore) Search(queryVector []float32, topK int) ([]knowledge.SearchResult, error) {
	return m.results, nil
}

func (m *mockVectorStore) Delete(id string) error {
	delete(m.upserted, id)
	return nil
}

func TestHub_SemanticSearch(t *testing.T) {
	ms := store.NewMemoryStore()
	bus := events.NewBus()
	vs := &mockVectorStore{
		results: []knowledge.SearchResult{{ID: "kb-1", Score: 0.95}},
	}
	h := knowledge.New(ms, bus, vs)

	results, err := h.SemanticSearch([]float32{0.1, 0.2, 0.3}, 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(results) != 1 || results[0].ID != "kb-1" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestHub_SemanticSearch_NoVectorStore(t *testing.T) {
	ms := store.NewMemoryStore()
	bus := events.NewBus()
	h := knowledge.New(ms, bus, nil) // no vector store

	_, err := h.SemanticSearch([]float32{0.1}, 5)
	if err == nil {
		t.Error("expected error when VectorStore is nil")
	}
}

func TestHub_AddEmbedding_NoVectorStore(t *testing.T) {
	ms := store.NewMemoryStore()
	bus := events.NewBus()
	h := knowledge.New(ms, bus, nil)

	err := h.AddEmbedding("kb-1", []float32{0.1}, nil)
	if err == nil {
		t.Error("expected error when VectorStore is nil")
	}
}
