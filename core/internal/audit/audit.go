package audit

import (
	"context"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// Logger records security-relevant actions to the audit store.
type Logger struct {
	store store.Store
	bus   *events.Bus
}

// New creates a new audit Logger.
func New(s store.Store, bus *events.Bus) *Logger {
	return &Logger{store: s, bus: bus}
}

// Record writes an audit entry to store and publishes an event on the bus.
func (l *Logger) Record(orgID, workerID, action, resource, requestID, outcome string, detail map[string]any) {
	entry := &protocol.AuditEntry{
		ID:        protocol.GenerateID("audit"),
		Timestamp: time.Now(),
		OrgID:     orgID,
		WorkerID:  workerID,
		Action:    action,
		Resource:  resource,
		RequestID: requestID,
		Outcome:   outcome,
		Detail:    detail,
	}

	// TODO(ctx): propagate from caller once audit API takes ctx.
	_ = l.store.AppendAudit(context.TODO(), entry)

	l.bus.Publish(events.Event{
		Type:     "audit." + action,
		Source:   "audit",
		Severity: severityForOutcome(outcome),
		Payload: map[string]any{
			"audit_id":   entry.ID,
			"org_id":     orgID,
			"worker_id":  workerID,
			"action":     action,
			"resource":   resource,
			"request_id": requestID,
			"outcome":    outcome,
		},
	})
}

// Query returns audit entries matching the filter.
func (l *Logger) Query(filter store.AuditFilter) []*protocol.AuditEntry {
	// TODO(ctx): propagate from caller once audit API takes ctx.
	return l.store.QueryAudit(context.TODO(), filter)
}

// SubscribeToEvents subscribes to existing bus events and records them as audit entries.
func (l *Logger) SubscribeToEvents() {
	l.bus.Subscribe("worker.registered", func(e events.Event) {
		l.Record(
			strVal(e.Payload, "org_id"),
			strVal(e.Payload, "worker_id"),
			"worker.registered",
			"worker/"+strVal(e.Payload, "worker_id"),
			strVal(e.Payload, "request_id"),
			"success",
			e.Payload,
		)
	})

	l.bus.Subscribe("worker.deregistered", func(e events.Event) {
		l.Record(
			strVal(e.Payload, "org_id"),
			strVal(e.Payload, "worker_id"),
			"worker.deregistered",
			"worker/"+strVal(e.Payload, "worker_id"),
			strVal(e.Payload, "request_id"),
			"success",
			e.Payload,
		)
	})

	l.bus.Subscribe("task.routed", func(e events.Event) {
		l.Record(
			strVal(e.Payload, "org_id"),
			strVal(e.Payload, "worker_id"),
			"task.routed",
			"task/"+strVal(e.Payload, "task_id"),
			strVal(e.Payload, "request_id"),
			"success",
			e.Payload,
		)
	})

	l.bus.Subscribe("task.completed", func(e events.Event) {
		l.Record(
			strVal(e.Payload, "org_id"),
			strVal(e.Payload, "worker_id"),
			"task.completed",
			"task/"+strVal(e.Payload, "task_id"),
			strVal(e.Payload, "request_id"),
			"success",
			e.Payload,
		)
	})

	l.bus.Subscribe("task.failed", func(e events.Event) {
		l.Record(
			strVal(e.Payload, "org_id"),
			strVal(e.Payload, "worker_id"),
			"task.failed",
			"task/"+strVal(e.Payload, "task_id"),
			strVal(e.Payload, "request_id"),
			"error",
			e.Payload,
		)
	})

	l.bus.Subscribe("worker.heartbeat", func(e events.Event) {
		l.Record(
			strVal(e.Payload, "org_id"),
			strVal(e.Payload, "worker_id"),
			"worker.heartbeat",
			"worker/"+strVal(e.Payload, "worker_id"),
			strVal(e.Payload, "request_id"),
			"success",
			e.Payload,
		)
	})
}

// severityForOutcome maps an outcome string to a severity level.
func severityForOutcome(outcome string) string {
	switch outcome {
	case "denied":
		return "warn"
	case "error":
		return "error"
	default:
		return "info"
	}
}

// strVal safely extracts a string value from a map.
func strVal(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	v, ok := payload[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
