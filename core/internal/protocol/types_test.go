package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kienbm/magic-claw/core/internal/protocol"
)

func TestWorkerSerialization(t *testing.T) {
	w := protocol.Worker{
		ID:   "worker_001",
		Name: "TestBot",
		Capabilities: []protocol.Capability{
			{
				Name:        "greeting",
				Description: "Says hello",
			},
		},
		Endpoint: protocol.Endpoint{
			Type: "http",
			URL:  "http://localhost:9000/mcp2",
		},
		Status: protocol.StatusActive,
	}

	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal worker: %v", err)
	}

	var w2 protocol.Worker
	if err := json.Unmarshal(data, &w2); err != nil {
		t.Fatalf("unmarshal worker: %v", err)
	}

	if w2.ID != w.ID {
		t.Errorf("ID: got %q, want %q", w2.ID, w.ID)
	}
	if w2.Name != w.Name {
		t.Errorf("Name: got %q, want %q", w2.Name, w.Name)
	}
	if len(w2.Capabilities) != 1 {
		t.Fatalf("Capabilities: got %d, want 1", len(w2.Capabilities))
	}
	if w2.Status != protocol.StatusActive {
		t.Errorf("Status: got %q, want %q", w2.Status, protocol.StatusActive)
	}
}

func TestTaskSerialization(t *testing.T) {
	task := protocol.Task{
		ID:       "task_001",
		Type:     "greeting",
		Priority: protocol.PriorityNormal,
		Status:   protocol.TaskPending,
		Input:    json.RawMessage(`{"name": "Kien"}`),
		Contract: protocol.Contract{
			TimeoutMs: 30000,
			MaxCost:   0.50,
		},
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}

	var task2 protocol.Task
	if err := json.Unmarshal(data, &task2); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}

	if task2.ID != "task_001" {
		t.Errorf("ID: got %q, want %q", task2.ID, "task_001")
	}
	if task2.Contract.MaxCost != 0.50 {
		t.Errorf("MaxCost: got %f, want 0.50", task2.Contract.MaxCost)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := protocol.GenerateID("worker")
	id2 := protocol.GenerateID("worker")
	if id1 == id2 {
		t.Error("GenerateID should return unique IDs")
	}
	if len(id1) < 10 {
		t.Errorf("ID too short: %q", id1)
	}
}

func TestMessageSerialization(t *testing.T) {
	msg := protocol.Message{
		Protocol:  "mcp2",
		Version:   "1.0",
		Type:      protocol.MsgWorkerRegister,
		ID:        "msg_001",
		Timestamp: time.Now(),
		Source:    "worker_001",
		Target:    "org_magic",
		Payload:   json.RawMessage(`{"name": "TestBot"}`),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var msg2 protocol.Message
	if err := json.Unmarshal(data, &msg2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if msg2.Type != protocol.MsgWorkerRegister {
		t.Errorf("Type: got %q, want %q", msg2.Type, protocol.MsgWorkerRegister)
	}
}

func TestNewMessage(t *testing.T) {
	msg := protocol.NewMessage(protocol.MsgTaskAssign, "org", "worker_001", json.RawMessage(`{}`))
	if msg.Protocol != "mcp2" {
		t.Errorf("Protocol: got %q, want mcp2", msg.Protocol)
	}
	if msg.ID == "" {
		t.Error("ID should not be empty")
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestWorkflowSerialization(t *testing.T) {
	wf := protocol.Workflow{
		ID:   "wf_001",
		Name: "Product Launch",
		Steps: []protocol.WorkflowStep{
			{ID: "research", TaskType: "market_research", Input: json.RawMessage(`{"topic": "AI"}`)},
			{ID: "content", TaskType: "content_writing", DependsOn: []string{"research"}, OnFailure: "retry"},
		},
		Status: protocol.WorkflowPending,
	}
	data, err := json.Marshal(wf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wf2 protocol.Workflow
	if err := json.Unmarshal(data, &wf2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wf2.Name != "Product Launch" {
		t.Errorf("Name: got %q", wf2.Name)
	}
	if len(wf2.Steps) != 2 {
		t.Fatalf("Steps: got %d", len(wf2.Steps))
	}
	if wf2.Steps[1].DependsOn[0] != "research" {
		t.Errorf("DependsOn: got %v", wf2.Steps[1].DependsOn)
	}
}
