package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

func newTestPostgresStore(t *testing.T) *store.PostgreSQLStore {
	t.Helper()
	url := os.Getenv("MAGIC_POSTGRES_URL")
	if url == "" {
		t.Skip("MAGIC_POSTGRES_URL not set — skipping PostgreSQL integration tests")
	}
	if err := store.RunMigrations(url); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	s, err := store.NewPostgreSQLStore(context.Background(), url)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPostgreSQLStore_WorkerCRUD(t *testing.T) {
	s := newTestPostgresStore(t)

	w := &protocol.Worker{
		ID:    "w-test-" + time.Now().Format("150405"),
		Name:  "TestWorker",
		OrgID: "org-1",
		Capabilities: []protocol.Capability{
			{Name: "summarize", Description: "Summarize text"},
		},
		Status:        protocol.StatusActive,
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
	}

	if err := s.AddWorker(w); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}
	got, err := s.GetWorker(w.ID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Name != w.Name {
		t.Errorf("Name: got %q, want %q", got.Name, w.Name)
	}
	w.Name = "UpdatedWorker"
	if err := s.UpdateWorker(w); err != nil {
		t.Fatalf("UpdateWorker: %v", err)
	}
	got2, _ := s.GetWorker(w.ID)
	if got2.Name != "UpdatedWorker" {
		t.Errorf("after update: got %q", got2.Name)
	}
	found := s.FindWorkersByCapability("summarize")
	if len(found) == 0 {
		t.Error("FindWorkersByCapability: no results")
	}
	byOrg := s.ListWorkersByOrg("org-1")
	if len(byOrg) == 0 {
		t.Error("ListWorkersByOrg: no results")
	}
	if err := s.RemoveWorker(w.ID); err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}
	if _, err := s.GetWorker(w.ID); err != store.ErrNotFound {
		t.Errorf("after remove: expected ErrNotFound, got %v", err)
	}
}

func TestPostgreSQLStore_WorkerTokens(t *testing.T) {
	s := newTestPostgresStore(t)

	tok := &protocol.WorkerToken{
		ID:        "tok-" + time.Now().Format("150405.000"),
		OrgID:     "org-1",
		Name:      "test-token",
		CreatedAt: time.Now(),
	}
	tok.TokenHash = "abc123hash"

	if err := s.AddWorkerToken(tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}
	got, err := s.GetWorkerTokenByHash("abc123hash")
	if err != nil {
		t.Fatalf("GetWorkerTokenByHash: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("ID: got %q, want %q", got.ID, tok.ID)
	}
	if got.TokenHash != "abc123hash" {
		t.Errorf("TokenHash not restored: got %q", got.TokenHash)
	}
	if !s.HasAnyWorkerTokens() {
		t.Error("HasAnyWorkerTokens: expected true")
	}
}
