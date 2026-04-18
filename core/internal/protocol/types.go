package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Worker statuses
const (
	StatusActive  = "active"
	StatusPaused  = "paused"
	StatusOffline = "offline"
)

// Task statuses
const (
	TaskPending    = "pending"
	TaskAssigned   = "assigned"
	TaskAccepted   = "accepted"
	TaskInProgress = "in_progress"
	TaskCompleted  = "completed"
	TaskFailed     = "failed"
	TaskCancelled  = "cancelled"
)

// IsTaskTerminal reports whether the given task status is a terminal state
// (no further transitions are expected).
func IsTaskTerminal(status string) bool {
	switch status {
	case TaskCompleted, TaskFailed, TaskCancelled:
		return true
	}
	return false
}

// Task priorities
const (
	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

// GenerateID creates a unique identifier with the given prefix.
func GenerateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// Capability represents a skill or function that a worker can perform.
type Capability struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	InputSchema    json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema   json.RawMessage `json:"output_schema,omitempty"`
	EstCostPerCall float64         `json:"est_cost_per_call,omitempty"`
	AvgResponseMs  int64           `json:"avg_response_ms,omitempty"`
	Streaming      bool            `json:"streaming,omitempty"` // worker supports SSE streaming for this capability
}

type Endpoint struct {
	Type string        `json:"type"`
	URL  string        `json:"url"`
	Auth *EndpointAuth `json:"auth,omitempty"`
}

type EndpointAuth struct {
	Type   string `json:"type"`
	Header string `json:"header"`
}

type WorkerLimits struct {
	MaxConcurrentTasks int     `json:"max_concurrent_tasks"`
	RateLimit          string  `json:"rate_limit,omitempty"`
	MaxCostPerDay      float64 `json:"max_cost_per_day,omitempty"`
}

type Worker struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	OrgID          string            `json:"org_id,omitempty"`
	TeamID         string            `json:"team_id,omitempty"`
	Capabilities   []Capability      `json:"capabilities"`
	Endpoint       Endpoint          `json:"endpoint"`
	Limits         WorkerLimits      `json:"limits"`
	Status         string            `json:"status"`
	CurrentLoad    int               `json:"current_load"`
	TotalCostToday float64           `json:"total_cost_today"`
	RegisteredAt   time.Time         `json:"registered_at"`
	LastHeartbeat  time.Time         `json:"last_heartbeat"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	SessionMode    string            `json:"session_mode,omitempty"` // "stateless" (default) or "sessionful"
}

// WorkerToken represents an authentication credential issued to a worker.
type WorkerToken struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	WorkerID  string     `json:"worker_id"`
	TokenHash string     `json:"-"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// IsValid checks if the token is not expired and not revoked.
func (t *WorkerToken) IsValid() bool {
	if t.RevokedAt != nil {
		return false
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return false
	}
	return true
}

// AuditEntry records a security-relevant action.
type AuditEntry struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	OrgID     string         `json:"org_id"`
	WorkerID  string         `json:"worker_id,omitempty"`
	Action    string         `json:"action"`
	Resource  string         `json:"resource"`
	Detail    map[string]any `json:"detail,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Outcome   string         `json:"outcome"`
}

type Contract struct {
	OutputSchema    json.RawMessage    `json:"output_schema,omitempty"`
	QualityCriteria []QualityCriterion `json:"quality_criteria,omitempty"`
	TimeoutMs       int64              `json:"timeout_ms"`
	MaxCost         float64            `json:"max_cost"`
	RetryPolicy     *RetryPolicy       `json:"retry_policy,omitempty"`
}

type QualityCriterion struct {
	Metric    string  `json:"metric"`
	Threshold float64 `json:"threshold"`
}

type RetryPolicy struct {
	MaxRetries int   `json:"max_retries"`
	BackoffMs  int64 `json:"backoff_ms,omitempty"`
}

type RoutingConfig struct {
	Strategy             string   `json:"strategy"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	PreferredWorkers     []string `json:"preferred_workers,omitempty"`
	ExcludedWorkers      []string `json:"excluded_workers,omitempty"`
}

type TaskContext struct {
	OrgID      string `json:"org_id,omitempty"`
	TeamID     string `json:"team_id,omitempty"`
	Requester  string `json:"requester,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
}

type TaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// DLQEntry represents a task that permanently failed after all retries.
type DLQEntry struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	TaskType  string    `json:"task_type"`
	WorkerID  string    `json:"worker_id"`
	Error     string    `json:"error"`
	Retries   int       `json:"retries"`
	CreatedAt time.Time `json:"created_at"`
}

// PromptTemplate is a versioned prompt stored in the registry.
type PromptTemplate struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Version   int               `json:"version"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// MemoryTurn is a conversation turn stored in agent memory.
type MemoryTurn struct {
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Task struct {
	ID             string          `json:"id"`
	TraceID        string          `json:"trace_id,omitempty"`
	Type           string          `json:"type"`
	Priority       string          `json:"priority"`
	Status         string          `json:"status"`
	Input          json.RawMessage `json:"input"`
	Output         json.RawMessage `json:"output,omitempty"`
	Contract       Contract        `json:"contract"`
	Routing        RoutingConfig   `json:"routing"`
	AssignedWorker string          `json:"assigned_worker,omitempty"`
	WorkflowID     string          `json:"workflow_id,omitempty"`
	Context        TaskContext     `json:"context"`
	Cost           float64         `json:"cost"`
	Progress       int             `json:"progress"`
	CreatedAt      time.Time       `json:"created_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	Error          *TaskError      `json:"error,omitempty"`
}

type Team struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	OrgID            string   `json:"org_id"`
	Workers          []string `json:"workers"`
	DailyBudget      float64  `json:"daily_budget"`
	ApprovalRequired bool     `json:"approval_required"`
}

// Workflow statuses
const (
	WorkflowPending   = "pending"
	WorkflowRunning   = "running"
	WorkflowCompleted = "completed"
	WorkflowFailed    = "failed"
	WorkflowAborted   = "aborted"
)

// Step statuses
const (
	StepPending       = "pending"
	StepRunning       = "running"
	StepCompleted     = "completed"
	StepFailed        = "failed"
	StepSkipped       = "skipped"
	StepBlocked       = "blocked"
	StepAwaitApproval = "awaiting_approval"
)

type WorkflowStep struct {
	ID               string          `json:"id"`
	TaskType         string          `json:"task_type"`
	Input            json.RawMessage `json:"input,omitempty"`
	DependsOn        []string        `json:"depends_on,omitempty"`
	OnFailure        string          `json:"on_failure,omitempty"`
	ApprovalRequired bool            `json:"approval_required,omitempty"`
	Status           string          `json:"status,omitempty"`
	TaskID           string          `json:"task_id,omitempty"`
	Output           json.RawMessage `json:"output,omitempty"`
	Error            *TaskError      `json:"error,omitempty"`
}

type Workflow struct {
	ID        string         `json:"id"`
	TraceID   string         `json:"trace_id,omitempty"`
	Name      string         `json:"name"`
	Steps     []WorkflowStep `json:"steps"`
	Status    string         `json:"status"`
	Context   TaskContext    `json:"context"`
	CreatedAt time.Time      `json:"created_at"`
	DoneAt    *time.Time     `json:"done_at,omitempty"`
}

// DeepCopyWorker returns a deep copy of a Worker, including slices and maps.
func DeepCopyWorker(w *Worker) *Worker {
	cp := *w
	if w.Capabilities != nil {
		cp.Capabilities = make([]Capability, len(w.Capabilities))
		copy(cp.Capabilities, w.Capabilities)
	}
	if w.Metadata != nil {
		cp.Metadata = make(map[string]any, len(w.Metadata))
		for k, v := range w.Metadata {
			cp.Metadata[k] = v
		}
	}
	if w.Tags != nil {
		cp.Tags = make(map[string]string, len(w.Tags))
		for k, v := range w.Tags {
			cp.Tags[k] = v
		}
	}
	return &cp
}

// DeepCopyTask returns a deep copy of a Task, including all nested slices and pointers.
func DeepCopyTask(t *Task) *Task {
	cp := *t
	if t.Input != nil {
		cp.Input = make(json.RawMessage, len(t.Input))
		copy(cp.Input, t.Input)
	}
	if t.Output != nil {
		cp.Output = make(json.RawMessage, len(t.Output))
		copy(cp.Output, t.Output)
	}
	if t.Contract.QualityCriteria != nil {
		cp.Contract.QualityCriteria = make([]QualityCriterion, len(t.Contract.QualityCriteria))
		copy(cp.Contract.QualityCriteria, t.Contract.QualityCriteria)
	}
	if t.Contract.OutputSchema != nil {
		cp.Contract.OutputSchema = make(json.RawMessage, len(t.Contract.OutputSchema))
		copy(cp.Contract.OutputSchema, t.Contract.OutputSchema)
	}
	if t.Contract.RetryPolicy != nil {
		rp := *t.Contract.RetryPolicy
		cp.Contract.RetryPolicy = &rp
	}
	if t.Routing.RequiredCapabilities != nil {
		cp.Routing.RequiredCapabilities = make([]string, len(t.Routing.RequiredCapabilities))
		copy(cp.Routing.RequiredCapabilities, t.Routing.RequiredCapabilities)
	}
	if t.Routing.PreferredWorkers != nil {
		cp.Routing.PreferredWorkers = make([]string, len(t.Routing.PreferredWorkers))
		copy(cp.Routing.PreferredWorkers, t.Routing.PreferredWorkers)
	}
	if t.Routing.ExcludedWorkers != nil {
		cp.Routing.ExcludedWorkers = make([]string, len(t.Routing.ExcludedWorkers))
		copy(cp.Routing.ExcludedWorkers, t.Routing.ExcludedWorkers)
	}
	if t.Error != nil {
		te := *t.Error
		cp.Error = &te
	}
	if t.CompletedAt != nil {
		ca := *t.CompletedAt
		cp.CompletedAt = &ca
	}
	return &cp
}

// deepCopyStep returns a deep copy of a WorkflowStep.
func deepCopyStep(s WorkflowStep) WorkflowStep {
	if s.DependsOn != nil {
		deps := make([]string, len(s.DependsOn))
		copy(deps, s.DependsOn)
		s.DependsOn = deps
	}
	if s.Input != nil {
		in := make(json.RawMessage, len(s.Input))
		copy(in, s.Input)
		s.Input = in
	}
	if s.Output != nil {
		out := make(json.RawMessage, len(s.Output))
		copy(out, s.Output)
		s.Output = out
	}
	if s.Error != nil {
		te := *s.Error
		s.Error = &te
	}
	return s
}

// DeepCopyWorkflow returns a deep copy of a Workflow, including all steps and their nested fields.
func DeepCopyWorkflow(wf *Workflow) *Workflow {
	cp := *wf
	if wf.Steps != nil {
		cp.Steps = make([]WorkflowStep, len(wf.Steps))
		for i, s := range wf.Steps {
			cp.Steps[i] = deepCopyStep(s)
		}
	}
	if wf.DoneAt != nil {
		d := *wf.DoneAt
		cp.DoneAt = &d
	}
	return &cp
}

// DeepCopyTeam returns a deep copy of a Team.
func DeepCopyTeam(t *Team) *Team {
	cp := *t
	if t.Workers != nil {
		cp.Workers = make([]string, len(t.Workers))
		copy(cp.Workers, t.Workers)
	}
	return &cp
}

// DeepCopyKnowledge returns a deep copy of a KnowledgeEntry.
func DeepCopyKnowledge(k *KnowledgeEntry) *KnowledgeEntry {
	cp := *k
	if k.Tags != nil {
		cp.Tags = make([]string, len(k.Tags))
		copy(cp.Tags, k.Tags)
	}
	return &cp
}

type KnowledgeEntry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	Scope     string    `json:"scope"`     // org | team | worker
	ScopeID   string    `json:"scope_id"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Webhook represents a registered webhook endpoint for receiving MagiC events.
type Webhook struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`                   // e.g. ["task.complete", "worker.register"]
	Secret    string    `json:"secret,omitempty"`         // write-only: HMAC-SHA256 key, never returned in GET
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// WebhookDelivery tracks one attempted delivery of an event to a webhook URL.
type WebhookDelivery struct {
	ID        string     `json:"id"`
	WebhookID string     `json:"webhook_id"`
	EventType string     `json:"event_type"`
	Payload   string     `json:"payload"`          // JSON-encoded event body
	Status    string     `json:"status"`           // pending|delivered|failed|dead
	Attempts  int        `json:"attempts"`
	NextRetry *time.Time `json:"next_retry,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Webhook delivery statuses
const (
	DeliveryPending   = "pending"
	DeliveryDelivered = "delivered"
	DeliveryFailed    = "failed"
	DeliveryDead      = "dead" // max retries exhausted
)

// DeepCopyWebhook returns a deep copy of a Webhook, including the Events slice.
func DeepCopyWebhook(w *Webhook) *Webhook {
	cp := *w
	if w.Events != nil {
		cp.Events = make([]string, len(w.Events))
		copy(cp.Events, w.Events)
	}
	return &cp
}

// --- RBAC ---

// Role constants for access control.
const (
	RoleOwner  = "owner"  // full access: manage org, policies, tokens, workers
	RoleAdmin  = "admin"  // read/write: submit tasks, manage workers, view costs
	RoleViewer = "viewer" // read-only: list workers, tasks, costs
)

// RoleBinding maps a subject to a role within an org.
type RoleBinding struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Subject   string    `json:"subject"`   // API key hash, user ID, or token ID
	Role      string    `json:"role"`      // owner | admin | viewer
	CreatedAt time.Time `json:"created_at"`
}

// DeepCopyRoleBinding returns a deep copy of a RoleBinding.
func DeepCopyRoleBinding(rb *RoleBinding) *RoleBinding {
	cp := *rb
	return &cp
}

// --- Policy Engine ---

// PolicyEffect determines how a rule violation is handled.
const (
	PolicyHard = "hard" // reject request immediately
	PolicySoft = "soft" // allow but audit + warn
)

// PolicyRule defines a single constraint within a policy.
type PolicyRule struct {
	Name   string  `json:"name"`             // e.g. "allowed_capabilities", "max_cost_per_task"
	Effect string  `json:"effect"`           // hard | soft
	Value  any     `json:"value"`            // []string for whitelist, float64 for limits
}

// Policy defines a set of rules scoped to an org.
type Policy struct {
	ID        string       `json:"id"`
	OrgID     string       `json:"org_id"`
	Name      string       `json:"name"`
	Rules     []PolicyRule `json:"rules"`
	Enabled   bool         `json:"enabled"`
	CreatedAt time.Time    `json:"created_at"`
}

// DeepCopyPolicy returns a deep copy of a Policy.
func DeepCopyPolicy(p *Policy) *Policy {
	cp := *p
	if p.Rules != nil {
		cp.Rules = make([]PolicyRule, len(p.Rules))
		copy(cp.Rules, p.Rules)
	}
	return &cp
}
