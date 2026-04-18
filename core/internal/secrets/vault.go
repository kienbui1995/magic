package secrets

import (
	"context"
	"fmt"
)

// VaultConfig holds the connection settings for HashiCorp Vault.
// All fields are read from MAGIC_VAULT_* env vars by NewFromEnv.
type VaultConfig struct {
	Address string // MAGIC_VAULT_ADDR, e.g. https://vault.example.com:8200
	Token   string // MAGIC_VAULT_TOKEN (or use a token helper in production)
	Mount   string // MAGIC_VAULT_MOUNT, e.g. "secret" (KVv2 mount)
	Path    string // MAGIC_VAULT_PATH, base path prefix under the mount
}

// VaultProvider is a stub implementation of the HashiCorp Vault backend.
//
// Get always returns ErrProviderUnavailable with a pointer to
// docs/security/secrets.md. The operator must vendor
// github.com/hashicorp/vault/api and implement the Get method to enable
// this provider in a production build.
//
// TODO(vendor): import github.com/hashicorp/vault/api and replace the
// stub body with a real KVv2 lookup:
//
//	client, _ := vault.NewClient(&vault.Config{Address: cfg.Address})
//	client.SetToken(cfg.Token)
//	sec, err := client.KVv2(cfg.Mount).Get(ctx, path.Join(cfg.Path, name))
//	return sec.Data["value"].(string), err
type VaultProvider struct {
	cfg VaultConfig
}

// NewVaultProvider validates config and returns a stub provider. It does
// not dial Vault — construction is cheap so startup never blocks on the
// secret backend.
func NewVaultProvider(cfg VaultConfig) (*VaultProvider, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("vault: MAGIC_VAULT_ADDR is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("vault: MAGIC_VAULT_TOKEN is required")
	}
	if cfg.Mount == "" {
		cfg.Mount = "secret"
	}
	return &VaultProvider{cfg: cfg}, nil
}

// Get is a stub; see package docs and docs/security/secrets.md for the
// implementation skeleton.
func (v *VaultProvider) Get(_ context.Context, name string) (string, error) {
	return "", fmt.Errorf(
		"%w: vault provider is a stub — vendor github.com/hashicorp/vault/api "+
			"and implement VaultProvider.Get (see docs/security/secrets.md); "+
			"requested secret=%q at %s/%s",
		ErrProviderUnavailable, name, v.cfg.Mount, v.cfg.Path,
	)
}

// Name identifies this provider in logs and health output.
func (v *VaultProvider) Name() string { return "vault (stub)" }
