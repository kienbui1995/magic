package store

import (
	"context"
	"errors"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrTokenAlreadyBound is returned when a token is already bound to a different worker.
var ErrTokenAlreadyBound = errors.New("token already bound to another worker")

// ErrTaskTerminal is returned by CancelTask when the task is already in a
// terminal state (completed, failed, or cancelled) and cannot be cancelled.
var ErrTaskTerminal = errors.New("task already in terminal state")

// AuditFilter defines query parameters for audit log.
type AuditFilter struct {
	OrgID     string
	WorkerID  string
	Action    string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// Store defines the persistence interface for all MagiC entities.
//
// Every method accepts context.Context as its first parameter. Implementations
// must honour cancellation and deadlines where the underlying backend allows
// (PostgreSQL, SQLite). The in-memory implementation accepts ctx for interface
// conformance but is CPU-bound so cancellation has no meaningful effect.
type Store interface {
	AddWorker(ctx context.Context, w *protocol.Worker) error
	GetWorker(ctx context.Context, id string) (*protocol.Worker, error)
	UpdateWorker(ctx context.Context, w *protocol.Worker) error
	RemoveWorker(ctx context.Context, id string) error
	ListWorkers(ctx context.Context) []*protocol.Worker
	FindWorkersByCapability(ctx context.Context, capability string) []*protocol.Worker

	AddTask(ctx context.Context, t *protocol.Task) error
	GetTask(ctx context.Context, id string) (*protocol.Task, error)
	UpdateTask(ctx context.Context, t *protocol.Task) error
	// CancelTask atomically transitions a task to the cancelled state.
	// It returns ErrNotFound if the task does not exist, and ErrTaskTerminal
	// if the task is already in a terminal state (completed/failed/cancelled).
	// The returned *protocol.Task reflects the updated state.
	CancelTask(ctx context.Context, id string) (*protocol.Task, error)
	ListTasks(ctx context.Context) []*protocol.Task

	// Workflows
	AddWorkflow(ctx context.Context, w *protocol.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*protocol.Workflow, error)
	UpdateWorkflow(ctx context.Context, w *protocol.Workflow) error
	ListWorkflows(ctx context.Context) []*protocol.Workflow

	// Teams
	AddTeam(ctx context.Context, t *protocol.Team) error
	GetTeam(ctx context.Context, id string) (*protocol.Team, error)
	UpdateTeam(ctx context.Context, t *protocol.Team) error
	RemoveTeam(ctx context.Context, id string) error
	ListTeams(ctx context.Context) []*protocol.Team

	// Knowledge
	AddKnowledge(ctx context.Context, k *protocol.KnowledgeEntry) error
	GetKnowledge(ctx context.Context, id string) (*protocol.KnowledgeEntry, error)
	UpdateKnowledge(ctx context.Context, k *protocol.KnowledgeEntry) error
	DeleteKnowledge(ctx context.Context, id string) error
	ListKnowledge(ctx context.Context) []*protocol.KnowledgeEntry
	SearchKnowledge(ctx context.Context, query string) []*protocol.KnowledgeEntry

	// Worker tokens
	AddWorkerToken(ctx context.Context, t *protocol.WorkerToken) error
	GetWorkerToken(ctx context.Context, id string) (*protocol.WorkerToken, error)
	GetWorkerTokenByHash(ctx context.Context, hash string) (*protocol.WorkerToken, error)
	UpdateWorkerToken(ctx context.Context, t *protocol.WorkerToken) error
	ListWorkerTokensByOrg(ctx context.Context, orgID string) []*protocol.WorkerToken
	ListWorkerTokensByWorker(ctx context.Context, workerID string) []*protocol.WorkerToken
	HasAnyWorkerTokens(ctx context.Context) bool

	// Audit log
	AppendAudit(ctx context.Context, e *protocol.AuditEntry) error
	QueryAudit(ctx context.Context, filter AuditFilter) []*protocol.AuditEntry

	// Org-scoped queries
	ListWorkersByOrg(ctx context.Context, orgID string) []*protocol.Worker
	ListTasksByOrg(ctx context.Context, orgID string) []*protocol.Task
	FindWorkersByCapabilityAndOrg(ctx context.Context, capability, orgID string) []*protocol.Worker

	// Webhooks
	AddWebhook(ctx context.Context, w *protocol.Webhook) error
	GetWebhook(ctx context.Context, id string) (*protocol.Webhook, error)
	UpdateWebhook(ctx context.Context, w *protocol.Webhook) error
	DeleteWebhook(ctx context.Context, id string) error
	ListWebhooksByOrg(ctx context.Context, orgID string) []*protocol.Webhook
	FindWebhooksByEvent(ctx context.Context, eventType string) []*protocol.Webhook

	// Webhook deliveries
	AddWebhookDelivery(ctx context.Context, d *protocol.WebhookDelivery) error
	UpdateWebhookDelivery(ctx context.Context, d *protocol.WebhookDelivery) error
	ListPendingWebhookDeliveries(ctx context.Context) []*protocol.WebhookDelivery

	// Role bindings (RBAC)
	AddRoleBinding(ctx context.Context, rb *protocol.RoleBinding) error
	GetRoleBinding(ctx context.Context, id string) (*protocol.RoleBinding, error)
	RemoveRoleBinding(ctx context.Context, id string) error
	ListRoleBindingsByOrg(ctx context.Context, orgID string) []*protocol.RoleBinding
	FindRoleBinding(ctx context.Context, orgID, subject string) (*protocol.RoleBinding, error)

	// Policies
	AddPolicy(ctx context.Context, p *protocol.Policy) error
	GetPolicy(ctx context.Context, id string) (*protocol.Policy, error)
	UpdatePolicy(ctx context.Context, p *protocol.Policy) error
	RemovePolicy(ctx context.Context, id string) error
	ListPoliciesByOrg(ctx context.Context, orgID string) []*protocol.Policy

	// Dead Letter Queue
	AddDLQEntry(ctx context.Context, e *protocol.DLQEntry) error
	ListDLQ(ctx context.Context) []*protocol.DLQEntry

	// Prompts
	AddPrompt(ctx context.Context, p *protocol.PromptTemplate) error
	ListPrompts(ctx context.Context) []*protocol.PromptTemplate

	// Agent Memory
	AddMemoryTurn(ctx context.Context, sessionID string, turn *protocol.MemoryTurn) error
	GetMemoryTurns(ctx context.Context, sessionID string) []*protocol.MemoryTurn
}
