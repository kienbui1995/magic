package store

import (
	"errors"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrTokenAlreadyBound is returned when a token is already bound to a different worker.
var ErrTokenAlreadyBound = errors.New("token already bound to another worker")

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

	// Knowledge
	AddKnowledge(k *protocol.KnowledgeEntry) error
	GetKnowledge(id string) (*protocol.KnowledgeEntry, error)
	UpdateKnowledge(k *protocol.KnowledgeEntry) error
	DeleteKnowledge(id string) error
	ListKnowledge() []*protocol.KnowledgeEntry
	SearchKnowledge(query string) []*protocol.KnowledgeEntry

	// Worker tokens
	AddWorkerToken(t *protocol.WorkerToken) error
	GetWorkerToken(id string) (*protocol.WorkerToken, error)
	GetWorkerTokenByHash(hash string) (*protocol.WorkerToken, error)
	UpdateWorkerToken(t *protocol.WorkerToken) error
	ListWorkerTokensByOrg(orgID string) []*protocol.WorkerToken
	ListWorkerTokensByWorker(workerID string) []*protocol.WorkerToken
	HasAnyWorkerTokens() bool

	// Audit log
	AppendAudit(e *protocol.AuditEntry) error
	QueryAudit(filter AuditFilter) []*protocol.AuditEntry

	// Org-scoped queries
	ListWorkersByOrg(orgID string) []*protocol.Worker
	ListTasksByOrg(orgID string) []*protocol.Task
	FindWorkersByCapabilityAndOrg(capability, orgID string) []*protocol.Worker

	// Webhooks
	AddWebhook(w *protocol.Webhook) error
	GetWebhook(id string) (*protocol.Webhook, error)
	UpdateWebhook(w *protocol.Webhook) error
	DeleteWebhook(id string) error
	ListWebhooksByOrg(orgID string) []*protocol.Webhook
	FindWebhooksByEvent(eventType string) []*protocol.Webhook

	// Webhook deliveries
	AddWebhookDelivery(d *protocol.WebhookDelivery) error
	UpdateWebhookDelivery(d *protocol.WebhookDelivery) error
	ListPendingWebhookDeliveries() []*protocol.WebhookDelivery

	// Role bindings (RBAC)
	AddRoleBinding(rb *protocol.RoleBinding) error
	GetRoleBinding(id string) (*protocol.RoleBinding, error)
	RemoveRoleBinding(id string) error
	ListRoleBindingsByOrg(orgID string) []*protocol.RoleBinding
	FindRoleBinding(orgID, subject string) (*protocol.RoleBinding, error)

	// Policies
	AddPolicy(p *protocol.Policy) error
	GetPolicy(id string) (*protocol.Policy, error)
	UpdatePolicy(p *protocol.Policy) error
	RemovePolicy(id string) error
	ListPoliciesByOrg(orgID string) []*protocol.Policy
}
