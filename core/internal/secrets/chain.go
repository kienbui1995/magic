package secrets

import (
	"context"
	"errors"
	"strings"
)

// ChainProvider queries multiple providers in order and returns the first
// hit. Useful for "env overrides, else Vault" layering where developers
// can shadow a production secret locally without touching Vault.
//
// Providers returning ErrNotFound are skipped; any other error (including
// ErrProviderUnavailable) is returned immediately so misconfiguration is
// not silently masked by falling through to the next backend.
type ChainProvider struct {
	providers []Provider
}

// NewChainProvider builds a chain from the given providers, in priority
// order (first = highest priority).
func NewChainProvider(providers ...Provider) *ChainProvider {
	return &ChainProvider{providers: providers}
}

// Get walks the chain and returns the first non-ErrNotFound result.
func (c *ChainProvider) Get(ctx context.Context, name string) (string, error) {
	for _, p := range c.providers {
		v, err := p.Get(ctx, name)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", err
		}
	}
	return "", ErrNotFound
}

// Name returns "chain(a,b,c)" for logging.
func (c *ChainProvider) Name() string {
	parts := make([]string, 0, len(c.providers))
	for _, p := range c.providers {
		parts = append(parts, p.Name())
	}
	return "chain(" + strings.Join(parts, ",") + ")"
}
