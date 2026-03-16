package evaluator_test

import (
	"encoding/json"
	"testing"

	"github.com/kienbm/magic-claw/core/internal/evaluator"
	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/protocol"
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

func TestEvaluator_NoSchema(t *testing.T) {
	bus := events.NewBus()
	ev := evaluator.New(bus)

	output := json.RawMessage(`{"anything": "goes"}`)
	result := ev.Evaluate(output, protocol.Contract{})
	if !result.Pass {
		t.Error("should pass when no schema specified")
	}
}
