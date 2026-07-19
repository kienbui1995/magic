package registry_test

import (
	"context"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/store"
)

// addToken is a helper that creates a WorkerToken in the store and returns the raw token string.
func addToken(t *testing.T, s store.Store, orgID string) (rawToken string, tok *protocol.WorkerToken) {
	t.Helper()
	raw, hash := protocol.GenerateToken()
	tok = &protocol.WorkerToken{
		ID:        protocol.GenerateID("token"),
		OrgID:     orgID,
		TokenHash: hash,
		Name:      "test-token",
		CreatedAt: time.Now(),
	}
	if err := s.AddWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}
	return raw, tok
}

func TestRegistry_Register(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	payload := protocol.RegisterPayload{
		Name: "TestBot",
		Capabilities: []protocol.Capability{
			{Name: "greeting", Description: "Says hello"},
		},
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
		Limits:   protocol.WorkerLimits{MaxConcurrentTasks: 5},
	}

	worker, err := reg.Register(payload)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if worker.ID == "" {
		t.Error("worker ID should not be empty")
	}
	if worker.Status != protocol.StatusActive {
		t.Errorf("status: got %q, want active", worker.Status)
	}

	got, err := s.GetWorker(context.Background(), worker.ID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Name != "TestBot" {
		t.Errorf("Name: got %q", got.Name)
	}
}

func TestRegistry_Heartbeat(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	payload := protocol.RegisterPayload{
		Name:     "TestBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
	}
	worker, _ := reg.Register(payload)

	hb := protocol.HeartbeatPayload{
		WorkerID:    worker.ID,
		CurrentLoad: 2,
		Status:      protocol.StatusActive,
	}

	err := reg.Heartbeat(hb)
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	got, _ := s.GetWorker(context.Background(), worker.ID)
	if got.CurrentLoad != 2 {
		t.Errorf("CurrentLoad: got %d, want 2", got.CurrentLoad)
	}
}

func TestRegistry_Deregister(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	payload := protocol.RegisterPayload{
		Name:     "TestBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
	}
	worker, _ := reg.Register(payload)

	err := reg.Deregister(worker.ID)
	if err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	_, err = s.GetWorker(context.Background(), worker.ID)
	if err == nil {
		t.Error("worker should be removed")
	}
}

func TestRegistry_HeartbeatCannotOverridePaused(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	payload := protocol.RegisterPayload{
		Name:     "TestBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9000"},
	}
	worker, _ := reg.Register(payload)

	// Simulate cost controller pausing the worker
	w, _ := s.GetWorker(context.Background(), worker.ID)
	w.Status = protocol.StatusPaused
	s.UpdateWorker(context.Background(), w)

	// Heartbeat tries to set status back to active
	err := reg.Heartbeat(protocol.HeartbeatPayload{
		WorkerID:    worker.ID,
		CurrentLoad: 0,
		Status:      protocol.StatusActive,
	})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	got, _ := s.GetWorker(context.Background(), worker.ID)
	if got.Status != protocol.StatusPaused {
		t.Errorf("Status: got %q, want paused (heartbeat should not override)", got.Status)
	}
}

func TestRegistry_FindByCapability(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	reg.Register(protocol.RegisterPayload{
		Name:         "ContentBot",
		Capabilities: []protocol.Capability{{Name: "content_writing"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	reg.Register(protocol.RegisterPayload{
		Name:         "DataBot",
		Capabilities: []protocol.Capability{{Name: "data_analysis"}},
		Endpoint:     protocol.Endpoint{Type: "http", URL: "http://localhost:9002"},
	})

	writers := reg.FindByCapability("content_writing")
	if len(writers) != 1 {
		t.Errorf("content_writing: got %d, want 1", len(writers))
	}
	if writers[0].Name != "ContentBot" {
		t.Errorf("Name: got %q", writers[0].Name)
	}
}

// --- Token authentication tests ---

func TestRegister_ValidToken(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	rawToken, _ := addToken(t, s, "org_acme")

	worker, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: rawToken,
		Name:        "ContentBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if worker.ID == "" {
		t.Error("worker ID should not be empty")
	}
	if worker.OrgID != "org_acme" {
		t.Errorf("OrgID: got %q, want org_acme", worker.OrgID)
	}
}

func TestRegister_NoToken_DevMode(t *testing.T) {
	// No tokens in store → dev mode → registration allowed without token
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	worker, err := reg.Register(protocol.RegisterPayload{
		Name:     "AnonBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err != nil {
		t.Fatalf("Register in dev mode: %v", err)
	}
	if worker.ID == "" {
		t.Error("worker ID should not be empty")
	}
	if worker.OrgID != "" {
		t.Errorf("OrgID should be empty in dev mode, got %q", worker.OrgID)
	}
}

func TestRegister_NoToken_SecurityMode(t *testing.T) {
	// Tokens exist in store but no token in payload → error
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	addToken(t, s, "org_acme") // activates security mode

	_, err := reg.Register(protocol.RegisterPayload{
		Name:     "AnonBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err == nil {
		t.Fatal("expected error when no token provided in security mode, got nil")
	}
}

func TestRegister_InvalidToken(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	addToken(t, s, "org_acme") // activates security mode

	_, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: "mct_thisiswrongtokenvalue0000000000000000000000000000000000000000",
		Name:        "BadBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
}

func TestRegister_RevokedToken(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	rawToken, tok := addToken(t, s, "org_acme")

	// Revoke the token
	now := time.Now()
	tok.RevokedAt = &now
	if err := s.UpdateWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("UpdateWorkerToken: %v", err)
	}

	_, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: rawToken,
		Name:        "RevokedBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err == nil {
		t.Fatal("expected error for revoked token, got nil")
	}
}

func TestRegister_ExpiredToken(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	raw, hash := protocol.GenerateToken()
	past := time.Now().Add(-time.Hour)
	tok := &protocol.WorkerToken{
		ID:        protocol.GenerateID("token"),
		OrgID:     "org_acme",
		TokenHash: hash,
		Name:      "expired-token",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: &past,
	}
	if err := s.AddWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	_, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: raw,
		Name:        "ExpiredBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestRegister_AlreadyBoundToken(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	rawToken, tok := addToken(t, s, "org_acme")

	// Bind the token to an existing worker ID
	tok.WorkerID = protocol.GenerateID("worker")
	if err := s.UpdateWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("UpdateWorkerToken: %v", err)
	}

	_, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: rawToken,
		Name:        "NewBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err == nil {
		t.Fatal("expected error for already-bound token, got nil")
	}
}

func TestRegister_SetsOrgID(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	rawToken, _ := addToken(t, s, "org_beta")

	worker, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: rawToken,
		Name:        "BetaBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Verify OrgID is set correctly both on the returned worker and in the store
	if worker.OrgID != "org_beta" {
		t.Errorf("returned worker OrgID: got %q, want org_beta", worker.OrgID)
	}
	stored, err := s.GetWorker(context.Background(), worker.ID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if stored.OrgID != "org_beta" {
		t.Errorf("stored worker OrgID: got %q, want org_beta", stored.OrgID)
	}
}

func TestHeartbeat_ValidToken(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	rawToken, _ := addToken(t, s, "org_acme")

	// Register a worker using the token
	worker, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: rawToken,
		Name:        "HBBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	err = reg.Heartbeat(protocol.HeartbeatPayload{
		WorkerToken: rawToken,
		WorkerID:    worker.ID,
		CurrentLoad: 1,
		Status:      protocol.StatusActive,
	})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	got, _ := s.GetWorker(context.Background(), worker.ID)
	if got.CurrentLoad != 1 {
		t.Errorf("CurrentLoad: got %d, want 1", got.CurrentLoad)
	}
}

func TestHeartbeat_WrongWorkerID(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	rawToken, _ := addToken(t, s, "org_acme")

	// Register worker — binds the token to this worker
	worker, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: rawToken,
		Name:        "HBBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Heartbeat with a different worker ID — should fail
	otherWorkerID := protocol.GenerateID("worker")
	_ = worker // suppress unused warning
	err = reg.Heartbeat(protocol.HeartbeatPayload{
		WorkerToken: rawToken,
		WorkerID:    otherWorkerID,
		CurrentLoad: 0,
		Status:      protocol.StatusActive,
	})
	if err == nil {
		t.Fatal("expected error when token bound to different worker, got nil")
	}
}

func TestHeartbeat_RevokedToken_SecurityMode(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	rawToken, tok := addToken(t, s, "org_acme")

	// Register first to bind the token
	worker, err := reg.Register(protocol.RegisterPayload{
		WorkerToken: rawToken,
		Name:        "HBBot",
		Endpoint:    protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Revoke the token after registration
	now := time.Now()
	tok.RevokedAt = &now
	tok.WorkerID = worker.ID
	if err := s.UpdateWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("UpdateWorkerToken: %v", err)
	}

	err = reg.Heartbeat(protocol.HeartbeatPayload{
		WorkerToken: rawToken,
		WorkerID:    worker.ID,
		CurrentLoad: 0,
		Status:      protocol.StatusActive,
	})
	if err == nil {
		t.Fatal("expected error for revoked token heartbeat, got nil")
	}
}

func TestHeartbeat_DevMode(t *testing.T) {
	// No tokens in store → dev mode → heartbeat without token works
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)

	// Register in dev mode
	worker, err := reg.Register(protocol.RegisterPayload{
		Name:     "DevBot",
		Endpoint: protocol.Endpoint{Type: "http", URL: "http://localhost:9001"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	err = reg.Heartbeat(protocol.HeartbeatPayload{
		WorkerID:    worker.ID,
		CurrentLoad: 3,
		Status:      protocol.StatusActive,
	})
	if err != nil {
		t.Fatalf("Heartbeat in dev mode: %v", err)
	}

	got, _ := s.GetWorker(context.Background(), worker.ID)
	if got.CurrentLoad != 3 {
		t.Errorf("CurrentLoad: got %d, want 3", got.CurrentLoad)
	}
}
