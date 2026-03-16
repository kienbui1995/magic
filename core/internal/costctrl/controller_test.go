package costctrl_test

import (
	"testing"
	"time"

	"github.com/kienbm/magic-claw/core/internal/costctrl"
	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/protocol"
	"github.com/kienbm/magic-claw/core/internal/store"
)

func TestCostController_RecordCost(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	cc := costctrl.New(s, bus)
	w := &protocol.Worker{ID: "worker_001", Name: "Bot", Status: protocol.StatusActive}
	s.AddWorker(w)
	cc.RecordCost("worker_001", "task_001", 0.15)
	report := cc.WorkerReport("worker_001")
	if report.TotalCost != 0.15 {
		t.Errorf("TotalCost: got %f", report.TotalCost)
	}
	if report.TaskCount != 1 {
		t.Errorf("TaskCount: got %d", report.TaskCount)
	}
}

func TestCostController_BudgetAlert(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	cc := costctrl.New(s, bus)
	var alerts []events.Event
	bus.Subscribe("budget.threshold", func(e events.Event) { alerts = append(alerts, e) })
	w := &protocol.Worker{ID: "worker_001", Name: "Bot", Status: protocol.StatusActive,
		Limits: protocol.WorkerLimits{MaxCostPerDay: 1.0}}
	s.AddWorker(w)
	cc.RecordCost("worker_001", "task_001", 0.85)
	time.Sleep(50 * time.Millisecond)
	if len(alerts) == 0 {
		t.Error("should have received budget alert at 80%")
	}
}

func TestCostController_AutoPause(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	cc := costctrl.New(s, bus)
	w := &protocol.Worker{ID: "worker_001", Name: "Bot", Status: protocol.StatusActive,
		Limits: protocol.WorkerLimits{MaxCostPerDay: 1.0}}
	s.AddWorker(w)
	cc.RecordCost("worker_001", "task_001", 1.10)
	time.Sleep(50 * time.Millisecond)
	got, _ := s.GetWorker("worker_001")
	if got.Status != protocol.StatusPaused {
		t.Errorf("Status: got %q, want paused", got.Status)
	}
}

func TestCostController_OrgReport(t *testing.T) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	cc := costctrl.New(s, bus)
	cc.RecordCost("w1", "t1", 0.10)
	cc.RecordCost("w2", "t2", 0.20)
	cc.RecordCost("w1", "t3", 0.05)
	report := cc.OrgReport()
	// Use tolerance for floating point comparison
	expected := 0.35
	if report.TotalCost < expected-0.001 || report.TotalCost > expected+0.001 {
		t.Errorf("TotalCost: got %f, want ~%f", report.TotalCost, expected)
	}
	if report.TaskCount != 3 {
		t.Errorf("TaskCount: got %d", report.TaskCount)
	}
}
