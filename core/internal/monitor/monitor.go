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
		case "task.failed":
			atomic.AddInt64(&m.stats.TasksFailed, 1)
		case "worker.registered":
			atomic.AddInt64(&m.stats.WorkersCount, 1)
		case "worker.deregistered":
			atomic.AddInt64(&m.stats.WorkersCount, -1)
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
