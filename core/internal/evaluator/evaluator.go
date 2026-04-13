package evaluator

import (
	"encoding/json"
	"fmt"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
)

// Result holds the outcome of an evaluation.
type Result struct {
	Pass   bool     `json:"pass"`
	Errors []string `json:"errors,omitempty"`
}

// EvalPlugin defines the interface for evaluation plugins.
// Implementations validate task output and return pass/fail with error details.
type EvalPlugin interface {
	Name() string
	Evaluate(output json.RawMessage, task *protocol.Task) Result
}

// Evaluator runs all registered plugins against task output.
type Evaluator struct {
	bus     *events.Bus
	plugins []EvalPlugin
}

// New creates a new Evaluator with the built-in schema validator.
func New(bus *events.Bus) *Evaluator {
	e := &Evaluator{bus: bus}
	e.RegisterPlugin(SchemaValidator{})
	return e
}

// RegisterPlugin adds a custom evaluation plugin.
func (e *Evaluator) RegisterPlugin(p EvalPlugin) {
	e.plugins = append(e.plugins, p)
}

// Evaluate runs all plugins and aggregates results.
func (e *Evaluator) Evaluate(output json.RawMessage, contract protocol.Contract) Result {
	task := &protocol.Task{Contract: contract}
	return e.EvaluateTask(output, task)
}

// EvaluateTask runs all plugins with full task context.
func (e *Evaluator) EvaluateTask(output json.RawMessage, task *protocol.Task) Result {
	var combined Result
	combined.Pass = true

	for _, p := range e.plugins {
		r := p.Evaluate(output, task)
		if !r.Pass {
			combined.Pass = false
			combined.Errors = append(combined.Errors, r.Errors...)
		}
	}

	if !combined.Pass {
		e.bus.Publish(events.Event{
			Type:     "evaluation.failed",
			Source:   "evaluator",
			Severity: "warn",
			Payload:  map[string]any{"errors": combined.Errors},
		})
	}

	return combined
}

// --- Built-in plugin: SchemaValidator ---

// SchemaValidator validates task output against the contract's OutputSchema.
type SchemaValidator struct{}

func (SchemaValidator) Name() string { return "schema_validator" }

func (SchemaValidator) Evaluate(output json.RawMessage, task *protocol.Task) Result {
	if len(task.Contract.OutputSchema) == 0 {
		return Result{Pass: true}
	}
	errs := validateSchema(output, task.Contract.OutputSchema)
	if len(errs) > 0 {
		return Result{Pass: false, Errors: errs}
	}
	return Result{Pass: true}
}

// --- Schema validation helpers (unchanged) ---

func validateSchema(data json.RawMessage, schema json.RawMessage) []string {
	var schemaMap map[string]any
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		return []string{fmt.Sprintf("invalid schema: %v", err)}
	}
	var dataVal any
	if err := json.Unmarshal(data, &dataVal); err != nil {
		return []string{fmt.Sprintf("invalid JSON output: %v", err)}
	}
	return validateValue(dataVal, schemaMap)
}

func validateValue(val any, schema map[string]any) []string {
	var errors []string

	if expectedType, ok := schema["type"].(string); ok {
		if !checkType(val, expectedType) {
			return []string{fmt.Sprintf("expected type %q, got %T", expectedType, val)}
		}
	}

	if obj, ok := val.(map[string]any); ok {
		if reqRaw, ok := schema["required"].([]any); ok {
			for _, r := range reqRaw {
				field := fmt.Sprint(r)
				if _, exists := obj[field]; !exists {
					errors = append(errors, fmt.Sprintf("missing required field %q", field))
				}
			}
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			for fieldName, propSchema := range props {
				fieldVal, exists := obj[fieldName]
				if !exists {
					continue
				}
				if propMap, ok := propSchema.(map[string]any); ok {
					fieldErrors := validateValue(fieldVal, propMap)
					for _, e := range fieldErrors {
						errors = append(errors, fmt.Sprintf("field %q: %s", fieldName, e))
					}
				}
			}
		}
	}

	return errors
}

func checkType(val any, expected string) bool {
	switch expected {
	case "object":
		_, ok := val.(map[string]any)
		return ok
	case "array":
		_, ok := val.([]any)
		return ok
	case "string":
		_, ok := val.(string)
		return ok
	case "number":
		_, ok := val.(float64)
		return ok
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "null":
		return val == nil
	}
	return true
}
