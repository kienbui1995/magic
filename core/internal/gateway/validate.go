package gateway

import (
	"net/http"
	"strings"
)

// validationError is a single field-level validation failure.
type validationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// validateRequest runs a set of field checks and returns the accumulated errors.
// Each check is a function that returns a (field, message) pair if the field
// is invalid, or ("", "") if the field is valid.
//
// Usage:
//
//	errs := validateRequest(
//	    required("name", req.Name),
//	    maxLen("name", req.Name, 255),
//	    oneOf("priority", req.Priority, "low", "normal", "high", "critical"),
//	)
//	if len(errs) > 0 { writeValidationError(w, errs); return }
func validateRequest(checks ...validationError) []validationError {
	out := make([]validationError, 0, len(checks))
	for _, c := range checks {
		if c.Field != "" {
			out = append(out, c)
		}
	}
	return out
}

// writeValidationError writes a 400 response with a machine-readable error body.
// The response schema is:
//
//	{
//	  "error": "validation_failed",
//	  "fields": [{"field": "name", "message": "required"}, ...]
//	}
func writeValidationError(w http.ResponseWriter, errs []validationError) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":  "validation_failed",
		"fields": errs,
	})
}

// required returns a validation error if v is empty (after trimming whitespace).
func required(field, v string) validationError {
	if strings.TrimSpace(v) == "" {
		return validationError{Field: field, Message: "required"}
	}
	return validationError{}
}

// maxLen returns a validation error if len(v) > max.
func maxLen(field, v string, max int) validationError {
	if len(v) > max {
		return validationError{Field: field, Message: "too long (max " + itoa(max) + ")"}
	}
	return validationError{}
}

// oneOf returns a validation error if v is not empty and not in allowed.
// An empty v passes (use required() separately to enforce presence).
func oneOf(field, v string, allowed ...string) validationError {
	if v == "" {
		return validationError{}
	}
	for _, a := range allowed {
		if v == a {
			return validationError{}
		}
	}
	return validationError{Field: field, Message: "must be one of: " + strings.Join(allowed, ", ")}
}

// nonEmptySlice returns a validation error if s has zero length.
func nonEmptySlice[T any](field string, s []T) validationError {
	if len(s) == 0 {
		return validationError{Field: field, Message: "must not be empty"}
	}
	return validationError{}
}

// itoa is a tiny int-to-string helper to avoid pulling in strconv for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
