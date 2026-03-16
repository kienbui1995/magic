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
)

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
	rand.Read(b)
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

type Task struct {
	ID             string          `json:"id"`
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
	StepPending   = "pending"
	StepRunning   = "running"
	StepCompleted = "completed"
	StepFailed    = "failed"
	StepSkipped   = "skipped"
	StepBlocked   = "blocked"
)

type WorkflowStep struct {
	ID        string          `json:"id"`
	TaskType  string          `json:"task_type"`
	Input     json.RawMessage `json:"input,omitempty"`
	DependsOn []string        `json:"depends_on,omitempty"`
	OnFailure string          `json:"on_failure,omitempty"`
	Status    string          `json:"status,omitempty"`
	TaskID    string          `json:"task_id,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     *TaskError      `json:"error,omitempty"`
}

type Workflow struct {
	ID        string         `json:"id"`
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
