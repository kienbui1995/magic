package orgmgr

import (
	"context"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

type Manager struct {
	store store.Store
	bus   *events.Bus
}

func New(s store.Store, bus *events.Bus) *Manager {
	return &Manager{store: s, bus: bus}
}

func (m *Manager) CreateTeam(ctx context.Context, name, orgID string, dailyBudget float64) (*protocol.Team, error) {
	team := &protocol.Team{
		ID:          protocol.GenerateID("team"),
		Name:        name,
		OrgID:       orgID,
		DailyBudget: dailyBudget,
	}
	if err := m.store.AddTeam(ctx, team); err != nil {
		return nil, err
	}
	m.bus.Publish(events.Event{
		Type:   "team.created",
		Source: "orgmgr",
		Payload: map[string]any{"team_id": team.ID, "team_name": team.Name},
	})
	return team, nil
}

func (m *Manager) DeleteTeam(ctx context.Context, teamID string) error {
	if err := m.store.RemoveTeam(ctx, teamID); err != nil {
		return err
	}
	m.bus.Publish(events.Event{
		Type:   "team.deleted",
		Source: "orgmgr",
		Payload: map[string]any{"team_id": teamID},
	})
	return nil
}

func (m *Manager) ListTeams(ctx context.Context) []*protocol.Team {
	return m.store.ListTeams(ctx)
}

func (m *Manager) GetTeam(ctx context.Context, id string) (*protocol.Team, error) {
	return m.store.GetTeam(ctx, id)
}

func (m *Manager) AssignWorker(ctx context.Context, teamID, workerID string) error {
	team, err := m.store.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	worker, err := m.store.GetWorker(ctx, workerID)
	if err != nil {
		return err
	}
	team.Workers = append(team.Workers, workerID)
	if err := m.store.UpdateTeam(ctx, team); err != nil {
		return err
	}
	worker.TeamID = teamID
	if err := m.store.UpdateWorker(ctx, worker); err != nil {
		return err
	}
	m.bus.Publish(events.Event{
		Type:   "team.worker_assigned",
		Source: "orgmgr",
		Payload: map[string]any{"team_id": teamID, "worker_id": workerID},
	})
	return nil
}

func (m *Manager) RemoveWorker(ctx context.Context, teamID, workerID string) error {
	team, err := m.store.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	var updated []string
	for _, id := range team.Workers {
		if id != workerID {
			updated = append(updated, id)
		}
	}
	team.Workers = updated
	if err := m.store.UpdateTeam(ctx, team); err != nil {
		return err
	}
	worker, err := m.store.GetWorker(ctx, workerID)
	if err != nil {
		return err
	}
	worker.TeamID = ""
	if err := m.store.UpdateWorker(ctx, worker); err != nil {
		return err
	}
	m.bus.Publish(events.Event{
		Type:   "team.worker_removed",
		Source: "orgmgr",
		Payload: map[string]any{"team_id": teamID, "worker_id": workerID},
	})
	return nil
}
