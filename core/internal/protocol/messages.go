package protocol

import (
	"encoding/json"
	"time"
)

const (
	MsgWorkerRegister           = "worker.register"
	MsgWorkerHeartbeat          = "worker.heartbeat"
	MsgWorkerDeregister         = "worker.deregister"
	MsgWorkerUpdateCapabilities = "worker.update_capabilities"

	MsgTaskAssign   = "task.assign"
	MsgTaskAccept   = "task.accept"
	MsgTaskReject   = "task.reject"
	MsgTaskProgress = "task.progress"
	MsgTaskComplete = "task.complete"
	MsgTaskFail     = "task.fail"

	MsgWorkerDelegate = "worker.delegate"
	MsgOrgBroadcast   = "org.broadcast"

	MsgWorkerOpenChannel  = "worker.open_channel"
	MsgWorkerCloseChannel = "worker.close_channel"
)

type Message struct {
	Protocol  string          `json:"protocol"`
	Version   string          `json:"version"`
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Source    string          `json:"source"`
	Target    string          `json:"target"`
	Payload   json.RawMessage `json:"payload"`
}

func NewMessage(msgType, source, target string, payload json.RawMessage) Message {
	return Message{
		Protocol:  "mcp2",
		Version:   "1.0",
		Type:      msgType,
		ID:        GenerateID("msg"),
		Timestamp: time.Now(),
		Source:    source,
		Target:    target,
		Payload:   payload,
	}
}

type RegisterPayload struct {
	Name         string            `json:"name"`
	Capabilities []Capability      `json:"capabilities"`
	Endpoint     Endpoint          `json:"endpoint"`
	Limits       WorkerLimits      `json:"limits"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
}

type TaskAssignPayload struct {
	TaskID   string          `json:"task_id"`
	TaskType string          `json:"task_type"`
	Priority string          `json:"priority"`
	Input    json.RawMessage `json:"input"`
	Contract Contract        `json:"contract"`
	Context  TaskContext     `json:"context"`
}

type TaskCompletePayload struct {
	TaskID string          `json:"task_id"`
	Output json.RawMessage `json:"output"`
	Cost   float64         `json:"cost"`
}

type TaskFailPayload struct {
	TaskID string    `json:"task_id"`
	Error  TaskError `json:"error"`
}

type TaskProgressPayload struct {
	TaskID   string          `json:"task_id"`
	Progress int             `json:"progress"`
	Output   json.RawMessage `json:"output,omitempty"`
}

type HeartbeatPayload struct {
	WorkerID    string `json:"worker_id"`
	CurrentLoad int    `json:"current_load"`
	Status      string `json:"status"`
}

type DelegatePayload struct {
	FromTaskID         string          `json:"from_task_id"`
	RequiredCapability string          `json:"required_capability"`
	Input              json.RawMessage `json:"input"`
}
