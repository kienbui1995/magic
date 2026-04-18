package secrets

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestEnvProvider_GetAndNotFound(t *testing.T) {
	p := NewEnvProvider()
	t.Setenv("MAGIC_TEST_SECRET", "hunter2")

	v, err := p.Get(context.Background(), "MAGIC_TEST_SECRET")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v != "hunter2" {
		t.Fatalf("want hunter2, got %q", v)
	}

	_, err = p.Get(context.Background(), "MAGIC_DOES_NOT_EXIST_X9Z")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	if p.Name() != "env" {
		t.Fatalf("unexpected name %q", p.Name())
	}
}

func TestEnvProvider_Concurrent(t *testing.T) {
	p := NewEnvProvider()
	t.Setenv("MAGIC_CONCURRENT_SECRET", "ok")

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := p.Get(context.Background(), "MAGIC_CONCURRENT_SECRET")
			if err != nil || v != "ok" {
				t.Errorf("concurrent get: v=%q err=%v", v, err)
			}
		}()
	}
	wg.Wait()
}

func TestVaultProvider_StubBehavior(t *testing.T) {
	p, err := NewVaultProvider(VaultConfig{
		Address: "https://vault.example",
		Token:   "t",
		Mount:   "secret",
		Path:    "magic",
	})
	if err != nil {
		t.Fatalf("constructor err: %v", err)
	}

	_, err = p.Get(context.Background(), "api-key")
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("want ErrProviderUnavailable, got %v", err)
	}

	// Missing address rejected at construction.
	if _, err := NewVaultProvider(VaultConfig{Token: "t"}); err == nil {
		t.Fatalf("expected error for missing addr")
	}
	// Missing token rejected at construction.
	if _, err := NewVaultProvider(VaultConfig{Address: "x"}); err == nil {
		t.Fatalf("expected error for missing token")
	}
}

func TestAWSProvider_StubBehavior(t *testing.T) {
	p, err := NewAWSSecretsManagerProvider(AWSConfig{Region: "ap-southeast-1", Prefix: "magic/"})
	if err != nil {
		t.Fatalf("constructor err: %v", err)
	}
	_, err = p.Get(context.Background(), "api-key")
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("want ErrProviderUnavailable, got %v", err)
	}

	if _, err := NewAWSSecretsManagerProvider(AWSConfig{}); err == nil {
		t.Fatalf("expected error for missing region")
	}
}

// stubProvider is a minimal in-memory Provider used to exercise
// ChainProvider semantics without touching os.Environ.
type stubProvider struct {
	name   string
	values map[string]string
	err    error // if non-nil, Get returns (zero, err) instead of map lookup
}

func (s *stubProvider) Get(_ context.Context, name string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	v, ok := s.values[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (s *stubProvider) Name() string { return s.name }

func TestChainProvider_FirstHitWins(t *testing.T) {
	a := &stubProvider{name: "a", values: map[string]string{"shared": "from-a"}}
	b := &stubProvider{name: "b", values: map[string]string{"shared": "from-b", "only-b": "bval"}}
	c := NewChainProvider(a, b)

	v, err := c.Get(context.Background(), "shared")
	if err != nil || v != "from-a" {
		t.Fatalf("first-hit: got v=%q err=%v", v, err)
	}

	v, err = c.Get(context.Background(), "only-b")
	if err != nil || v != "bval" {
		t.Fatalf("fallthrough: got v=%q err=%v", v, err)
	}

	_, err = c.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestChainProvider_NonNotFoundStops(t *testing.T) {
	boom := &stubProvider{name: "boom", err: ErrProviderUnavailable}
	fallback := &stubProvider{name: "fallback", values: map[string]string{"k": "v"}}
	c := NewChainProvider(boom, fallback)

	_, err := c.Get(context.Background(), "k")
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("want ErrProviderUnavailable to short-circuit, got %v", err)
	}
}

func TestChainProvider_Name(t *testing.T) {
	c := NewChainProvider(&stubProvider{name: "env"}, &stubProvider{name: "vault"})
	if got := c.Name(); got != "chain(env,vault)" {
		t.Fatalf("unexpected name %q", got)
	}
}

func TestNewFromEnv_DefaultAndSelection(t *testing.T) {
	t.Setenv("MAGIC_SECRETS_PROVIDER", "")
	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("default err: %v", err)
	}
	if p.Name() != "env" {
		t.Fatalf("default provider name = %q", p.Name())
	}

	t.Setenv("MAGIC_SECRETS_PROVIDER", "env")
	if p, _ := NewFromEnv(); p.Name() != "env" {
		t.Fatalf("env selection failed: %q", p.Name())
	}

	t.Setenv("MAGIC_SECRETS_PROVIDER", "vault")
	t.Setenv("MAGIC_VAULT_ADDR", "https://vault.example")
	t.Setenv("MAGIC_VAULT_TOKEN", "t")
	if p, err := NewFromEnv(); err != nil || p.Name() != "vault (stub)" {
		t.Fatalf("vault selection failed: name=%q err=%v", p.Name(), err)
	}

	t.Setenv("MAGIC_SECRETS_PROVIDER", "aws")
	t.Setenv("AWS_REGION", "ap-southeast-1")
	if p, err := NewFromEnv(); err != nil || p.Name() != "aws-secrets-manager (stub)" {
		t.Fatalf("aws selection failed: name=%q err=%v", p.Name(), err)
	}

	t.Setenv("MAGIC_SECRETS_PROVIDER", "bogus")
	if _, err := NewFromEnv(); err == nil {
		t.Fatalf("expected error for unknown provider")
	}
}
