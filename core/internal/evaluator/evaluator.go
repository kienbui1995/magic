package evaluator

import (
	"encoding/json"
	"fmt"

	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/protocol"
)

type Result struct {
	Pass   bool     `json:"pass"`
	Errors []string `json:"errors,omitempty"`
}

type Evaluator struct {
	bus *events.Bus
}

func New(bus *events.Bus) *Evaluator {
	return &Evaluator{bus: bus}
}

func (e *Evaluator) Evaluate(output json.RawMessage, contract protocol.Contract) Result {
	var result Result
	result.Pass = true

	if len(contract.OutputSchema) > 0 {
		schemaErrors := validateSchema(output, contract.OutputSchema)
		if len(schemaErrors) > 0 {
			result.Pass = false
			result.Errors = append(result.Errors, schemaErrors...)
		}
	}

	if !result.Pass {
		e.bus.Publish(events.Event{
			Type:     "evaluation.failed",
			Source:   "evaluator",
			Severity: "warn",
			Payload:  map[string]any{"errors": result.Errors},
		})
	}

	return result
}

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
