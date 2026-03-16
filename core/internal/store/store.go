package store

import (
	"fmt"

	"github.com/kienbm/magic-claw/core/internal/protocol"
)

var ErrNotFound = fmt.Errorf("not found")

type Store interface {
	AddWorker(w *protocol.Worker) error
	GetWorker(id string) (*protocol.Worker, error)
	UpdateWorker(w *protocol.Worker) error
	RemoveWorker(id string) error
	ListWorkers() []*protocol.Worker
	FindWorkersByCapability(capability string) []*protocol.Worker

	AddTask(t *protocol.Task) error
	GetTask(id string) (*protocol.Task, error)
	UpdateTask(t *protocol.Task) error
	ListTasks() []*protocol.Task

	// Workflows
	AddWorkflow(w *protocol.Workflow) error
	GetWorkflow(id string) (*protocol.Workflow, error)
	UpdateWorkflow(w *protocol.Workflow) error
	ListWorkflows() []*protocol.Workflow

	// Teams
	AddTeam(t *protocol.Team) error
	GetTeam(id string) (*protocol.Team, error)
	UpdateTeam(t *protocol.Team) error
	RemoveTeam(id string) error
	ListTeams() []*protocol.Team
}
