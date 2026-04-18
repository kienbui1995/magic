package config

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/kienbui1995/magic/core/internal/secrets"
)

// stubProvider returns pre-seeded values; missing keys surface ErrNotFound
// so the loader leaves the YAML/default in place.
type stubProvider struct {
	values map[string]string
	err    error // returned for any lookup when non-nil
}

func (s *stubProvider) Get(_ context.Context, name string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if v, ok := s.values[name]; ok {
		return v, nil
	}
	return "", secrets.ErrNotFound
}

func (s *stubProvider) Name() string { return "stub" }

func TestLoadWithSecrets_ProviderWins(t *testing.T) {
	// Env is NOT set — the provider is the sole source of credentials.
	sp := &stubProvider{values: map[string]string{
		"MAGIC_API_KEY":     "k-from-provider",
		"MAGIC_POSTGRES_URL": "postgres://stub",
		"OPENAI_API_KEY":    "sk-openai",
		"ANTHROPIC_API_KEY": "sk-anthropic",
	}}
	cfg, err := LoadWithSecrets(context.Background(), "", sp)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "k-from-provider" {
		t.Errorf("api key = %q", cfg.APIKey)
	}
	if cfg.Store.PostgresURL != "postgres://stub" {
		t.Errorf("pg url = %q", cfg.Store.PostgresURL)
	}
	if cfg.LLM.OpenAI.APIKey != "sk-openai" || cfg.LLM.Anthropic.APIKey != "sk-anthropic" {
		t.Errorf("llm keys not propagated: %+v", cfg.LLM)
	}
	if cfg.Store.Driver != "postgres" {
		t.Errorf("driver = %s, want postgres", cfg.Store.Driver)
	}
}

func TestLoadWithSecrets_ProviderError(t *testing.T) {
	// A non-ErrNotFound error must surface so misconfigured backends
	// do not silently fall through to empty credentials.
	sp := &stubProvider{err: errors.New("vault down")}
	if _, err := LoadWithSecrets(context.Background(), "", sp); err == nil {
		t.Fatal("expected error when provider fails, got nil")
	}
}

func TestLoadWithSecrets_NilProviderDefaultsToEnv(t *testing.T) {
	t.Setenv("MAGIC_API_KEY", "from-env")
	cfg, err := LoadWithSecrets(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-env" {
		t.Errorf("api key = %q, want from-env", cfg.APIKey)
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" {
		t.Errorf("default port = %s, want 8080", cfg.Port)
	}
	if cfg.Store.Driver != "memory" {
		t.Errorf("default driver = %s, want memory", cfg.Store.Driver)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("MAGIC_PORT", "9090")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9090" {
		t.Errorf("port = %s, want 9090", cfg.Port)
	}
	if cfg.LLM.OpenAI.APIKey != "sk-test" {
		t.Errorf("openai key = %s", cfg.LLM.OpenAI.APIKey)
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	f, _ := os.CreateTemp("", "magic-*.yaml")
	f.WriteString("port: \"3000\"\nllm:\n  openai:\n    api_key: sk-yaml\n")
	f.Close()
	defer os.Remove(f.Name())

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "3000" {
		t.Errorf("port = %s, want 3000", cfg.Port)
	}
	if cfg.LLM.OpenAI.APIKey != "sk-yaml" {
		t.Errorf("openai key = %s, want sk-yaml", cfg.LLM.OpenAI.APIKey)
	}
}

func TestLoad_YAMLEnvInterpolation(t *testing.T) {
	t.Setenv("INTERP_PG_URL", "postgres://interp/db")
	t.Setenv("INTERP_API_KEY", "k-from-env-interp")
	f, _ := os.CreateTemp("", "magic-interp-*.yaml")
	// Env vars in YAML should be expanded before parsing.
	f.WriteString("port: \"7000\"\napi_key: \"${INTERP_API_KEY}\"\npostgres_url: \"${INTERP_PG_URL}\"\n")
	f.Close()
	defer os.Remove(f.Name())

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "k-from-env-interp" {
		t.Errorf("api key = %q", cfg.APIKey)
	}
	if cfg.Store.PostgresURL != "postgres://interp/db" {
		t.Errorf("pg url (via flat alias) = %q", cfg.Store.PostgresURL)
	}
}

func TestLoad_YAMLNewFields(t *testing.T) {
	f, _ := os.CreateTemp("", "magic-fields-*.yaml")
	f.WriteString(`port: "8080"
log_level: debug
oidc:
  issuer: "https://example.okta.com"
  client_id: "magic-prod"
otel:
  endpoint: "http://jaeger:4318"
  sampler: "parentbased_traceidratio"
  sampler_arg: "0.1"
rate_limits:
  register_per_minute: 10
  task_per_minute: 200
`)
	f.Close()
	defer os.Remove(f.Name())

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log_level = %q", cfg.LogLevel)
	}
	if cfg.OIDC.Issuer != "https://example.okta.com" || cfg.OIDC.ClientID != "magic-prod" {
		t.Errorf("oidc = %+v", cfg.OIDC)
	}
	if cfg.OTel.Endpoint != "http://jaeger:4318" || cfg.OTel.SamplerArg != "0.1" {
		t.Errorf("otel = %+v", cfg.OTel)
	}
	if cfg.RateLimits.RegisterPerMinute != 10 || cfg.RateLimits.TaskPerMinute != 200 {
		t.Errorf("rate_limits = %+v", cfg.RateLimits)
	}
}

func TestLoad_AutoDetectDriver(t *testing.T) {
	t.Setenv("MAGIC_POSTGRES_URL", "postgres://localhost/magic")
	cfg, _ := Load("")
	if cfg.Store.Driver != "postgres" {
		t.Errorf("driver = %s, want postgres", cfg.Store.Driver)
	}
}
