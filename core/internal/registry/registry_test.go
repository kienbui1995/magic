package registry_test

import (
	"testing"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/store"
)

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

	got, err := s.GetWorker(worker.ID)
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

	got, _ := s.GetWorker(worker.ID)
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

	_, err = s.GetWorker(worker.ID)
	if err == nil {
		t.Error("worker should be removed")
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
