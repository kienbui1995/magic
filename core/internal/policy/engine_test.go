package policy_test

import (
	"context"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/policy"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

func setup(t *testing.T) (*policy.Engine, store.Store) {
	s := store.NewMemoryStore()
	bus := events.NewBus()
	return policy.New(s, bus), s
}

func TestEngine_DevMode_NoPolicies(t *testing.T) {
	e, _ := setup(t)
	task := &protocol.Task{ID: "t1", Routing: protocol.RoutingConfig{RequiredCapabilities: []string{"anything"}}}
	r := e.Enforce(task)
	if !r.Allowed {
		t.Error("dev mode (no orgID) should allow all")
	}
}

func TestEngine_HardGuardrail_BlockedCapability(t *testing.T) {
	e, s := setup(t)
	s.AddPolicy(context.Background(), &protocol.Policy{
		ID: "p1", OrgID: "org1", Name: "security", Enabled: true,
		Rules: []protocol.PolicyRule{
			{Name: "blocked_capabilities", Effect: protocol.PolicyHard, Value: []any{"dangerous_tool"}},
		},
		CreatedAt: time.Now(),
	})

	task := &protocol.Task{
		ID:      "t1",
		Context: protocol.TaskContext{OrgID: "org1"},
		Routing: protocol.RoutingConfig{RequiredCapabilities: []string{"dangerous_tool"}},
	}
	r := e.Enforce(task)
	if r.Allowed {
		t.Error("hard guardrail should block dangerous_tool")
	}
	if len(r.Violations) == 0 {
		t.Error("should have violations")
	}
}

func TestEngine_SoftGuardrail_CostWarning(t *testing.T) {
	e, s := setup(t)
	s.AddPolicy(context.Background(), &protocol.Policy{
		ID: "p1", OrgID: "org1", Name: "cost-limit", Enabled: true,
		Rules: []protocol.PolicyRule{
			{Name: "max_cost_per_task", Effect: protocol.PolicySoft, Value: float64(1.0)},
		},
		CreatedAt: time.Now(),
	})

	task := &protocol.Task{
		ID:       "t1",
		Context:  protocol.TaskContext{OrgID: "org1"},
		Contract: protocol.Contract{MaxCost: 5.0},
	}
	r := e.Enforce(task)
	if !r.Allowed {
		t.Error("soft guardrail should allow but warn")
	}
	if len(r.Violations) == 0 {
		t.Error("should have a soft violation")
	}
	if r.Violations[0].Effect != protocol.PolicySoft {
		t.Errorf("expected soft effect, got %q", r.Violations[0].Effect)
	}
}

func TestEngine_AllowedCapabilities_Whitelist(t *testing.T) {
	e, s := setup(t)
	s.AddPolicy(context.Background(), &protocol.Policy{
		ID: "p1", OrgID: "org1", Name: "whitelist", Enabled: true,
		Rules: []protocol.PolicyRule{
			{Name: "allowed_capabilities", Effect: protocol.PolicyHard, Value: []any{"writing", "analysis"}},
		},
		CreatedAt: time.Now(),
	})

	// Allowed capability
	task := &protocol.Task{
		ID:      "t1",
		Context: protocol.TaskContext{OrgID: "org1"},
		Routing: protocol.RoutingConfig{RequiredCapabilities: []string{"writing"}},
	}
	r := e.Enforce(task)
	if !r.Allowed {
		t.Error("writing should be allowed")
	}

	// Not in whitelist
	task2 := &protocol.Task{
		ID:      "t2",
		Context: protocol.TaskContext{OrgID: "org1"},
		Routing: protocol.RoutingConfig{RequiredCapabilities: []string{"hacking"}},
	}
	r2 := e.Enforce(task2)
	if r2.Allowed {
		t.Error("hacking should be blocked by whitelist")
	}
}

func TestEngine_MaxTimeout(t *testing.T) {
	e, s := setup(t)
	s.AddPolicy(context.Background(), &protocol.Policy{
		ID: "p1", OrgID: "org1", Name: "timeout", Enabled: true,
		Rules: []protocol.PolicyRule{
			{Name: "max_timeout_ms", Effect: protocol.PolicyHard, Value: float64(30000)},
		},
		CreatedAt: time.Now(),
	})

	task := &protocol.Task{
		ID:       "t1",
		Context:  protocol.TaskContext{OrgID: "org1"},
		Contract: protocol.Contract{TimeoutMs: 60000},
	}
	r := e.Enforce(task)
	if r.Allowed {
		t.Error("timeout 60s should exceed 30s limit")
	}
}

func TestEngine_DisabledPolicy_Ignored(t *testing.T) {
	e, s := setup(t)
	s.AddPolicy(context.Background(), &protocol.Policy{
		ID: "p1", OrgID: "org1", Name: "disabled", Enabled: false,
		Rules: []protocol.PolicyRule{
			{Name: "blocked_capabilities", Effect: protocol.PolicyHard, Value: []any{"everything"}},
		},
		CreatedAt: time.Now(),
	})

	task := &protocol.Task{
		ID:      "t1",
		Context: protocol.TaskContext{OrgID: "org1"},
		Routing: protocol.RoutingConfig{RequiredCapabilities: []string{"everything"}},
	}
	r := e.Enforce(task)
	if !r.Allowed {
		t.Error("disabled policy should be ignored")
	}
}
