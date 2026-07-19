package policy

import (
	"context"
	"fmt"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// Violation represents a policy rule that was triggered.
type Violation struct {
	Rule    string `json:"rule"`
	Effect  string `json:"effect"` // hard | soft
	Message string `json:"message"`
}

// Result holds the outcome of policy enforcement.
type Result struct {
	Allowed    bool        `json:"allowed"`
	Violations []Violation `json:"violations,omitempty"`
}

// Engine evaluates org policies against tasks before execution.
type Engine struct {
	store store.Store
	bus   *events.Bus
}

// New creates a new policy Engine.
func New(s store.Store, bus *events.Bus) *Engine {
	return &Engine{store: s, bus: bus}
}

// Enforce evaluates all enabled policies for the task's org.
// Returns Result with allowed=false if any hard guardrail is violated.
// Soft violations are recorded but don't block execution.
func (e *Engine) Enforce(task *protocol.Task) Result {
	orgID := task.Context.OrgID
	if orgID == "" {
		return Result{Allowed: true} // dev mode
	}

	// TODO(ctx): propagate from caller once policy API takes ctx.
	policies := e.store.ListPoliciesByOrg(context.TODO(), orgID)
	var result Result
	result.Allowed = true

	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		for _, rule := range p.Rules {
			if v := checkRule(rule, task); v != nil {
				result.Violations = append(result.Violations, *v)
				if rule.Effect == protocol.PolicyHard {
					result.Allowed = false
				}
			}
		}
	}

	for _, v := range result.Violations {
		severity := "warn"
		if v.Effect == protocol.PolicyHard {
			severity = "error"
		}
		e.bus.Publish(events.Event{
			Type:     "policy.violation",
			Source:   "policy",
			Severity: severity,
			Payload: map[string]any{
				"org_id":  orgID,
				"task_id": task.ID,
				"rule":    v.Rule,
				"effect":  v.Effect,
				"message": v.Message,
			},
		})
	}

	return result
}

// checkRule evaluates a single rule against a task.
// Returns a Violation if the rule is triggered, nil otherwise.
func checkRule(rule protocol.PolicyRule, task *protocol.Task) *Violation {
	switch rule.Name {
	case "allowed_capabilities":
		return checkAllowedCapabilities(rule, task)
	case "max_cost_per_task":
		return checkMaxCost(rule, task)
	case "max_timeout_ms":
		return checkMaxTimeout(rule, task)
	case "blocked_capabilities":
		return checkBlockedCapabilities(rule, task)
	}
	return nil
}

func checkAllowedCapabilities(rule protocol.PolicyRule, task *protocol.Task) *Violation {
	allowed := toStringSlice(rule.Value)
	if len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, c := range allowed {
		allowedSet[c] = true
	}
	for _, cap := range task.Routing.RequiredCapabilities {
		if !allowedSet[cap] {
			return &Violation{
				Rule:    rule.Name,
				Effect:  rule.Effect,
				Message: fmt.Sprintf("capability %q not in whitelist", cap),
			}
		}
	}
	return nil
}

func checkBlockedCapabilities(rule protocol.PolicyRule, task *protocol.Task) *Violation {
	blocked := toStringSlice(rule.Value)
	blockedSet := make(map[string]bool, len(blocked))
	for _, c := range blocked {
		blockedSet[c] = true
	}
	for _, cap := range task.Routing.RequiredCapabilities {
		if blockedSet[cap] {
			return &Violation{
				Rule:    rule.Name,
				Effect:  rule.Effect,
				Message: fmt.Sprintf("capability %q is blocked", cap),
			}
		}
	}
	return nil
}

func checkMaxCost(rule protocol.PolicyRule, task *protocol.Task) *Violation {
	maxCost := toFloat64(rule.Value)
	if maxCost <= 0 {
		return nil
	}
	if task.Contract.MaxCost > maxCost {
		return &Violation{
			Rule:    rule.Name,
			Effect:  rule.Effect,
			Message: fmt.Sprintf("task max_cost %.2f exceeds policy limit %.2f", task.Contract.MaxCost, maxCost),
		}
	}
	return nil
}

func checkMaxTimeout(rule protocol.PolicyRule, task *protocol.Task) *Violation {
	maxMs := toFloat64(rule.Value)
	if maxMs <= 0 {
		return nil
	}
	if float64(task.Contract.TimeoutMs) > maxMs {
		return &Violation{
			Rule:    rule.Name,
			Effect:  rule.Effect,
			Message: fmt.Sprintf("task timeout %dms exceeds policy limit %.0fms", task.Contract.TimeoutMs, maxMs),
		}
	}
	return nil
}

// --- helpers ---

func toStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	}
	return 0
}
