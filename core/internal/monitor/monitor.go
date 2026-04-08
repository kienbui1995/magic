package monitor

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/kienbui1995/magic/core/internal/events"
)

// Stats represents system-wide metrics.
type Stats struct {
	TotalEvents  int64 `json:"total_events"`
	TasksRouted  int64 `json:"tasks_routed"`
	TasksDone    int64 `json:"tasks_done"`
	TasksFailed  int64 `json:"tasks_failed"`
	WorkersCount int64 `json:"workers_count"`
}

// Monitor tracks system events and outputs structured logs.
type Monitor struct {
	bus      *events.Bus
	writer   io.Writer
	writerMu sync.Mutex
	stats    Stats
}

// New creates a new Monitor that writes logs to the given writer.
func New(bus *events.Bus, writer io.Writer) *Monitor {
	return &Monitor{bus: bus, writer: writer}
}

// Start subscribes to all events and begins logging.
func (m *Monitor) Start() {
	_ = m.bus.Subscribe("*", func(e events.Event) {
		atomic.AddInt64(&m.stats.TotalEvents, 1)

		switch e.Type {
		case "task.routed":
			atomic.AddInt64(&m.stats.TasksRouted, 1)
		case "task.completed":
			atomic.AddInt64(&m.stats.TasksDone, 1)
			workerID, _ := e.Payload["worker_id"].(string)
			taskType, _ := e.Payload["task_type"].(string)
			MetricTasksTotal.WithLabelValues(taskType, "completed", workerID).Inc()
		case "task.failed":
			atomic.AddInt64(&m.stats.TasksFailed, 1)
			workerID, _ := e.Payload["worker_id"].(string)
			taskType, _ := e.Payload["task_type"].(string)
			MetricTasksTotal.WithLabelValues(taskType, "failed", workerID).Inc()
		case "worker.registered":
			atomic.AddInt64(&m.stats.WorkersCount, 1)
			orgID, _ := e.Payload["org_id"].(string)
			MetricWorkersActive.WithLabelValues(orgID).Inc()
		case "worker.deregistered":
			atomic.AddInt64(&m.stats.WorkersCount, -1)
			orgID, _ := e.Payload["org_id"].(string)
			MetricWorkersActive.WithLabelValues(orgID).Dec()
		case "knowledge.added":
			MetricKnowledgeEntriesTotal.Inc()
		case "knowledge.deleted":
			MetricKnowledgeEntriesTotal.Dec()
		case "knowledge.queried":
			qType, _ := e.Payload["type"].(string)
			if qType == "" {
				qType = "keyword"
			}
			MetricKnowledgeQueriesTotal.WithLabelValues(qType).Inc()
		case "workflow.started":
			MetricWorkflowsActive.Inc()
		case "workflow.completed":
			MetricWorkflowStepsTotal.WithLabelValues("completed").Inc()
			MetricWorkflowsActive.Dec()
		case "workflow.failed":
			MetricWorkflowStepsTotal.WithLabelValues("failed").Inc()
			MetricWorkflowsActive.Dec()
		case "cost.recorded":
			if cost, ok := e.Payload["cost"].(float64); ok {
				orgID, _ := e.Payload["org_id"].(string)
				workerID, _ := e.Payload["worker_id"].(string)
				MetricCostTotalUSD.WithLabelValues(orgID, workerID).Add(cost)
			}
		}

		writeLogEntry(m.writer, &m.writerMu, e)
	})
}

// Stats returns current system metrics atomically.
func (m *Monitor) Stats() Stats {
	return Stats{
		TotalEvents:  atomic.LoadInt64(&m.stats.TotalEvents),
		TasksRouted:  atomic.LoadInt64(&m.stats.TasksRouted),
		TasksDone:    atomic.LoadInt64(&m.stats.TasksDone),
		TasksFailed:  atomic.LoadInt64(&m.stats.TasksFailed),
		WorkersCount: atomic.LoadInt64(&m.stats.WorkersCount),
	}
}
