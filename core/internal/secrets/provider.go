// Package secrets defines a pluggable abstraction for fetching sensitive
// configuration (API keys, DB credentials, tokens) at runtime. The env
// provider is zero-dependency; Vault and AWS providers are stubs that
// return an error until the operator vendors the required SDK and wires
// them up.
//
// The abstraction is intentionally minimal: a Provider exposes a single
// Get(ctx, name) method returning a plaintext value. Callers should not
// cache the value indefinitely — rotation is the provider's responsibility.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Provider looks up secret values by logical name.
// Implementations MUST be safe for concurrent use by multiple goroutines.
type Provider interface {
	// Get returns the plaintext value for the given secret name.
	// Returns ErrNotFound if the secret does not exist in this backend.
	// Returns ErrProviderUnavailable if the backend is configured but
	// not reachable or not yet implemented in this build.
	Get(ctx context.Context, name string) (string, error)

	// Name returns a human-readable identifier for logs / health output.
	Name() string
}

// ErrNotFound indicates the requested secret is not configured in this
// provider. Callers may fall through to a default or try another provider.
var ErrNotFound = errors.New("secret not found")

// ErrProviderUnavailable indicates the backend is selected but unreachable,
// misconfigured, or not yet implemented in this build. Distinct from
// ErrNotFound — operators must act on this error rather than silently fall
// back to defaults.
var ErrProviderUnavailable = errors.New("secret provider unavailable")

// NewFromEnv constructs a Provider based on the MAGIC_SECRETS_PROVIDER env
// var. Supported values:
//
//   - "" or "env" (default): EnvProvider — reads from os.Getenv.
//   - "vault": HashiCorp Vault (stub — returns ErrProviderUnavailable
//     from Get until the operator vendors github.com/hashicorp/vault/api).
//   - "aws": AWS Secrets Manager (stub — returns ErrProviderUnavailable
//     from Get until github.com/aws/aws-sdk-go-v2/service/secretsmanager
//     is vendored).
//
// Provider-specific configuration is read from MAGIC_VAULT_* and
// AWS_REGION / MAGIC_AWS_SECRETS_PREFIX env vars respectively.
func NewFromEnv() (Provider, error) {
	kind := strings.ToLower(strings.TrimSpace(os.Getenv("MAGIC_SECRETS_PROVIDER")))
	switch kind {
	case "", "env":
		return NewEnvProvider(), nil
	case "vault":
		return NewVaultProvider(VaultConfig{
			Address: os.Getenv("MAGIC_VAULT_ADDR"),
			Token:   os.Getenv("MAGIC_VAULT_TOKEN"),
			Mount:   os.Getenv("MAGIC_VAULT_MOUNT"),
			Path:    os.Getenv("MAGIC_VAULT_PATH"),
		})
	case "aws":
		return NewAWSSecretsManagerProvider(AWSConfig{
			Region: os.Getenv("AWS_REGION"),
			Prefix: os.Getenv("MAGIC_AWS_SECRETS_PREFIX"),
		})
	default:
		return nil, fmt.Errorf("unknown MAGIC_SECRETS_PROVIDER=%q (valid: env, vault, aws)", kind)
	}
}
