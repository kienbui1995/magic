package evaluator_test

import (
	"encoding/json"
	"testing"

	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
)

func TestEvaluator_SchemaValidation_Pass(t *testing.T) {
	bus := events.NewBus()
	ev := evaluator.New(bus)

	schema := json.RawMessage(`{
		"type": "object",
		"required": ["title", "body"],
		"properties": {
			"title": {"type": "string"},
			"body": {"type": "string"}
		}
	}`)
	output := json.RawMessage(`{"title": "Hello", "body": "World"}`)

	result := ev.Evaluate(output, protocol.Contract{OutputSchema: schema})
	if !result.Pass {
		t.Errorf("should pass, got errors: %v", result.Errors)
	}
}

func TestEvaluator_SchemaValidation_MissingRequired(t *testing.T) {
	bus := events.NewBus()
	ev := evaluator.New(bus)

	schema := json.RawMessage(`{
		"type": "object",
		"required": ["title", "body"],
		"properties": {
			"title": {"type": "string"},
			"body": {"type": "string"}
		}
	}`)
	output := json.RawMessage(`{"title": "Hello"}`)

	result := ev.Evaluate(output, protocol.Contract{OutputSchema: schema})
	if result.Pass {
		t.Error("should fail — missing required field 'body'")
	}
	if len(result.Errors) == 0 {
		t.Error("should have at least one error")
	}
}

func TestEvaluator_SchemaValidation_WrongType(t *testing.T) {
	bus := events.NewBus()
	ev := evaluator.New(bus)

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"count": {"type": "number"}
		}
	}`)
	output := json.RawMessage(`{"count": "not a number"}`)

	result := ev.Evaluate(output, protocol.Contract{OutputSchema: schema})
	if result.Pass {
		t.Error("should fail — wrong type for 'count'")
	}
}

// --- Custom plugin test ---

// minLengthPlugin rejects output shorter than N bytes.
type minLengthPlugin struct{ min int }

func (p minLengthPlugin) Name() string { return "min_length" }
func (p minLengthPlugin) Evaluate(output json.RawMessage, _ *protocol.Task) evaluator.Result {
	if len(output) < p.min {
		return evaluator.Result{Pass: false, Errors: []string{"output too short"}}
	}
	return evaluator.Result{Pass: true}
}

func TestEvaluator_CustomPlugin(t *testing.T) {
	bus := events.NewBus()
	ev := evaluator.New(bus)
	ev.RegisterPlugin(minLengthPlugin{min: 50})

	// Short output — custom plugin rejects
	short := json.RawMessage(`{"ok":true}`)
	r := ev.Evaluate(short, protocol.Contract{})
	if r.Pass {
		t.Error("custom plugin should reject short output")
	}

	// Long output — passes both built-in and custom
	long := json.RawMessage(`{"title":"Hello World","body":"This is a sufficiently long output for the test"}`)
	r = ev.Evaluate(long, protocol.Contract{})
	if !r.Pass {
		t.Errorf("should pass, got errors: %v", r.Errors)
	}
}

func TestEvaluator_NoSchema(t *testing.T) {
	bus := events.NewBus()
	ev := evaluator.New(bus)

	output := json.RawMessage(`{"anything": "goes"}`)
	result := ev.Evaluate(output, protocol.Contract{})
	if !result.Pass {
		t.Error("should pass when no schema specified")
	}
}
