package knowledge_test

import (
	"testing"

	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/knowledge"
	"github.com/kienbm/magic-claw/core/internal/store"
)

func TestHub_Add(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus)

	entry, err := hub.Add("API Guidelines", "Use REST conventions", []string{"api", "rest"}, "org", "org_magic", "admin")
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
	hub := knowledge.New(s, bus)

	entry, _ := hub.Add("Test", "Content", nil, "org", "org_magic", "admin")

	got, err := hub.Get(entry.ID)
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
	hub := knowledge.New(s, bus)

	hub.Add("API Guidelines", "REST conventions", []string{"api"}, "org", "org_magic", "admin")
	hub.Add("Database Guide", "Use PostgreSQL", []string{"database"}, "org", "org_magic", "admin")

	results := hub.Search("API")
	if len(results) != 1 {
		t.Errorf("Search 'API': got %d, want 1", len(results))
	}

	results = hub.Search("database")
	if len(results) != 1 {
		t.Errorf("Search 'database': got %d, want 1", len(results))
	}
}

func TestHub_Update(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus)

	entry, _ := hub.Add("Old Title", "Old content", nil, "org", "org_magic", "admin")

	err := hub.Update(entry.ID, "New Title", "New content", []string{"updated"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := hub.Get(entry.ID)
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
	hub := knowledge.New(s, bus)

	entry, _ := hub.Add("To Delete", "Content", nil, "org", "org_magic", "admin")

	err := hub.Delete(entry.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = hub.Get(entry.ID)
	if err == nil {
		t.Error("should fail after delete")
	}
}

func TestHub_List(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	hub := knowledge.New(s, bus)

	hub.Add("Entry 1", "Content 1", nil, "org", "org_magic", "admin")
	hub.Add("Entry 2", "Content 2", nil, "team", "team_marketing", "admin")

	entries := hub.List()
	if len(entries) != 2 {
		t.Errorf("List: got %d, want 2", len(entries))
	}
}
