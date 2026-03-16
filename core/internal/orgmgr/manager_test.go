package orgmgr_test

import (
	"testing"

	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/orgmgr"
	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/store"
)

func TestOrgManager_CreateTeam(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	mgr := orgmgr.New(s, bus)

	team, err := mgr.CreateTeam("Marketing", "org_magic", 10.0)
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.ID == "" {
		t.Error("team ID should not be empty")
	}
	if team.Name != "Marketing" {
		t.Errorf("Name: got %q", team.Name)
	}
	if team.DailyBudget != 10.0 {
		t.Errorf("DailyBudget: got %f", team.DailyBudget)
	}
}

func TestOrgManager_AssignWorker(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	mgr := orgmgr.New(s, bus)

	team, _ := mgr.CreateTeam("Marketing", "org_magic", 10.0)
	w := &protocol.Worker{ID: "worker_001", Name: "Bot", Status: protocol.StatusActive}
	s.AddWorker(w)

	err := mgr.AssignWorker(team.ID, "worker_001")
	if err != nil {
		t.Fatalf("AssignWorker: %v", err)
	}

	got, _ := s.GetTeam(team.ID)
	if len(got.Workers) != 1 || got.Workers[0] != "worker_001" {
		t.Errorf("Workers: got %v", got.Workers)
	}

	gotW, _ := s.GetWorker("worker_001")
	if gotW.TeamID != team.ID {
		t.Errorf("TeamID: got %q", gotW.TeamID)
	}
}

func TestOrgManager_RemoveWorker(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	mgr := orgmgr.New(s, bus)

	team, _ := mgr.CreateTeam("Marketing", "org_magic", 10.0)
	w := &protocol.Worker{ID: "worker_001", Name: "Bot", Status: protocol.StatusActive}
	s.AddWorker(w)
	mgr.AssignWorker(team.ID, "worker_001")

	err := mgr.RemoveWorker(team.ID, "worker_001")
	if err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}

	got, _ := s.GetTeam(team.ID)
	if len(got.Workers) != 0 {
		t.Errorf("Workers: got %v, want empty", got.Workers)
	}

	gotW, _ := s.GetWorker("worker_001")
	if gotW.TeamID != "" {
		t.Errorf("TeamID: got %q, want empty", gotW.TeamID)
	}
}

func TestOrgManager_ListTeams(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	mgr := orgmgr.New(s, bus)

	mgr.CreateTeam("Marketing", "org_magic", 10.0)
	mgr.CreateTeam("Sales", "org_magic", 15.0)

	teams := mgr.ListTeams()
	if len(teams) != 2 {
		t.Errorf("ListTeams: got %d, want 2", len(teams))
	}
}

func TestOrgManager_DeleteTeam(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	mgr := orgmgr.New(s, bus)

	team, _ := mgr.CreateTeam("Marketing", "org_magic", 10.0)

	err := mgr.DeleteTeam(team.ID)
	if err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}

	teams := mgr.ListTeams()
	if len(teams) != 0 {
		t.Errorf("ListTeams after delete: got %d", len(teams))
	}
}
