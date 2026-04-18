package secrets

import (
	"context"
	"os"
)

// EnvProvider resolves secrets via os.Getenv. It is the zero-dependency
// default and safe for concurrent use (os.Getenv itself is goroutine-safe).
type EnvProvider struct{}

// NewEnvProvider constructs the default env-backed provider.
func NewEnvProvider() *EnvProvider { return &EnvProvider{} }

// Get returns the env var matching name. An empty value is treated as
// "not set" and yields ErrNotFound so callers can distinguish missing
// secrets from intentionally empty ones.
func (e *EnvProvider) Get(_ context.Context, name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", ErrNotFound
	}
	return v, nil
}

// Name identifies this provider in logs and health output.
func (e *EnvProvider) Name() string { return "env" }
