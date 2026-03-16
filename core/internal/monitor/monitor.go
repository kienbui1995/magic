package monitor

import (
	"io"
	"sync/atomic"

	"github.com/kienbm/magic-claw/core/internal/events"
)

type Stats struct {
	TotalEvents  int64 `json:"total_events"`
	TasksRouted  int64 `json:"tasks_routed"`
	TasksDone    int64 `json:"tasks_done"`
	TasksFailed  int64 `json:"tasks_failed"`
	WorkersCount int64 `json:"workers_count"`
}

type Monitor struct {
	bus    *events.Bus
	writer io.Writer
	stats  Stats
}

func New(bus *events.Bus, writer io.Writer) *Monitor {
	return &Monitor{bus: bus, writer: writer}
}

func (m *Monitor) Start() {
	m.bus.Subscribe("*", func(e events.Event) {
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

		writeLogEntry(m.writer, e)
	})
}

func (m *Monitor) Stats() Stats {
	return Stats{
		TotalEvents:  atomic.LoadInt64(&m.stats.TotalEvents),
		TasksRouted:  atomic.LoadInt64(&m.stats.TasksRouted),
		TasksDone:    atomic.LoadInt64(&m.stats.TasksDone),
		TasksFailed:  atomic.LoadInt64(&m.stats.TasksFailed),
		WorkersCount: atomic.LoadInt64(&m.stats.WorkersCount),
	}
}
