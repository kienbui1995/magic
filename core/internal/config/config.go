// Package config loads MagiC server configuration from YAML files.
// Environment variables override YAML values (env takes precedence).
//
// Credential values (API keys, DB connection strings) are resolved through
// a secrets.Provider so operators can plug in Vault / AWS Secrets Manager
// without changing call sites. Non-secret knobs (ports, proxy trust,
// CORS origin, pool sizes) continue to read os.Getenv directly.
package config

import (
	"context"
	"errors"
	"os"

	"github.com/kienbui1995/magic/core/internal/secrets"
	"go.yaml.in/yaml/v2"
)

// Config is the top-level server configuration.
type Config struct {
	Port         string         `yaml:"port"`
	LogLevel     string         `yaml:"log_level"`
	APIKey       string         `yaml:"api_key"`
	Store        StoreConf      `yaml:"store"`
	LLM          LLMConf        `yaml:"llm"`
	CORS         string         `yaml:"cors_origin"`
	TrustedProxy bool           `yaml:"trusted_proxy"`
	// PostgresURL is a flat-key alias for store.postgres_url that makes
	// config files read more naturally (mirrors MAGIC_POSTGRES_URL env).
	PostgresURL string         `yaml:"postgres_url"`
	RedisURL    string         `yaml:"redis_url"`
	OIDC        OIDCConf       `yaml:"oidc"`
	OTel        OTelConf       `yaml:"otel"`
	RateLimits  RateLimitsConf `yaml:"rate_limits"`
}

// OIDCConf mirrors the MAGIC_OIDC_* env vars consumed in main.go.
type OIDCConf struct {
	Issuer   string `yaml:"issuer"`
	ClientID string `yaml:"client_id"`
	Audience string `yaml:"audience"`
}

// OTelConf mirrors OTEL_* env vars for tracing.
type OTelConf struct {
	Endpoint    string `yaml:"endpoint"`
	ServiceName string `yaml:"service_name"`
	Sampler     string `yaml:"sampler"`
	SamplerArg  string `yaml:"sampler_arg"`
}

// RateLimitsConf mirrors gateway rate-limit knobs.
type RateLimitsConf struct {
	RegisterPerMinute int `yaml:"register_per_minute"`
	TaskPerMinute     int `yaml:"task_per_minute"`
}

// StoreConf configures the storage backend.
type StoreConf struct {
	Driver      string `yaml:"driver"` // memory, sqlite, postgres
	SQLitePath  string `yaml:"sqlite_path"`
	PostgresURL string `yaml:"postgres_url"`
}

// LLMConf configures LLM providers.
type LLMConf struct {
	OpenAI    OpenAIConf    `yaml:"openai"`
	Anthropic AnthropicConf `yaml:"anthropic"`
	Ollama    OllamaConf    `yaml:"ollama"`
}

type OpenAIConf struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

type AnthropicConf struct {
	APIKey string `yaml:"api_key"`
}

type OllamaConf struct {
	URL string `yaml:"url"`
}

// credentialKeys lists the env-var names resolved via secrets.Provider
// instead of direct os.Getenv. These are the only values that should
// ever leave the process as plaintext credentials.
//
//nolint:gochecknoglobals // read-only registry
var credentialKeys = []string{
	"MAGIC_API_KEY",
	"MAGIC_POSTGRES_URL",
	"OPENAI_API_KEY",
	"ANTHROPIC_API_KEY",
}

// Load reads config from a YAML file, then overlays environment variables.
// Credentials are resolved via the default EnvProvider. Prefer
// LoadWithSecrets when a custom provider is available (e.g. from main).
func Load(path string) (*Config, error) {
	return LoadWithSecrets(context.Background(), path, secrets.NewEnvProvider())
}

// LoadWithSecrets reads config from a YAML file, then overlays values
// from env vars (non-secrets) and the supplied secrets.Provider (the
// four credentials listed in credentialKeys).
//
// If sp is nil, behaves like Load.
func LoadWithSecrets(ctx context.Context, path string, sp secrets.Provider) (*Config, error) {
	if sp == nil {
		sp = secrets.NewEnvProvider()
	}
	cfg := &Config{Port: "8080"}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// Expand ${VAR} / $VAR references against the process environment
		// before YAML parsing, so operators can reference secrets via env
		// without hardcoding them in the file.
		expanded := os.ExpandEnv(string(data))
		if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
			return nil, err
		}
	}

	// Non-secret env overrides (port, proxy trust, base URLs, CORS).
	envOverride(&cfg.Port, "MAGIC_PORT")
	envOverride(&cfg.LogLevel, "MAGIC_LOG_LEVEL")
	envOverride(&cfg.Store.SQLitePath, "MAGIC_STORE")
	envOverride(&cfg.LLM.OpenAI.BaseURL, "OPENAI_BASE_URL")
	envOverride(&cfg.LLM.Ollama.URL, "OLLAMA_URL")
	envOverride(&cfg.CORS, "MAGIC_CORS_ORIGIN")
	envOverride(&cfg.RedisURL, "MAGIC_REDIS_URL")
	envOverride(&cfg.OIDC.Issuer, "MAGIC_OIDC_ISSUER")
	envOverride(&cfg.OIDC.ClientID, "MAGIC_OIDC_CLIENT_ID")
	envOverride(&cfg.OIDC.Audience, "MAGIC_OIDC_AUDIENCE")
	envOverride(&cfg.OTel.Endpoint, "OTEL_EXPORTER_OTLP_ENDPOINT")
	envOverride(&cfg.OTel.ServiceName, "OTEL_SERVICE_NAME")
	envOverride(&cfg.OTel.Sampler, "OTEL_TRACES_SAMPLER")
	envOverride(&cfg.OTel.SamplerArg, "OTEL_TRACES_SAMPLER_ARG")
	if os.Getenv("MAGIC_TRUSTED_PROXY") == "true" {
		cfg.TrustedProxy = true
	}

	// Credential overrides via secrets.Provider. Missing secrets
	// (ErrNotFound) are silently skipped so YAML values survive; any
	// other error is surfaced so misconfigured backends do not silently
	// fall back to empty credentials.
	if err := secretOverride(ctx, sp, &cfg.APIKey, "MAGIC_API_KEY"); err != nil {
		return nil, err
	}
	if err := secretOverride(ctx, sp, &cfg.Store.PostgresURL, "MAGIC_POSTGRES_URL"); err != nil {
		return nil, err
	}
	// Accept flat `postgres_url:` key as a fallback for the nested form.
	if cfg.Store.PostgresURL == "" && cfg.PostgresURL != "" {
		cfg.Store.PostgresURL = cfg.PostgresURL
	}
	if err := secretOverride(ctx, sp, &cfg.LLM.OpenAI.APIKey, "OPENAI_API_KEY"); err != nil {
		return nil, err
	}
	if err := secretOverride(ctx, sp, &cfg.LLM.Anthropic.APIKey, "ANTHROPIC_API_KEY"); err != nil {
		return nil, err
	}

	// Auto-detect store driver
	if cfg.Store.Driver == "" {
		switch {
		case cfg.Store.PostgresURL != "":
			cfg.Store.Driver = "postgres"
		case cfg.Store.SQLitePath != "":
			cfg.Store.Driver = "sqlite"
		default:
			cfg.Store.Driver = "memory"
		}
	}

	return cfg, nil
}

func envOverride(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

// secretOverride resolves a credential via the provider. Treats
// ErrNotFound as "leave YAML value alone"; propagates anything else.
func secretOverride(ctx context.Context, sp secrets.Provider, target *string, name string) error {
	v, err := sp.Get(ctx, name)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return nil
		}
		return err
	}
	if v != "" {
		*target = v
	}
	return nil
}
