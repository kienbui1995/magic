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

func GenerateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

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
