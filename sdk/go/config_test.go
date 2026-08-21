package opentrace

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildConfig replicates New()'s config assembly without starting the
// pipeline: defaults → env → options.
func buildConfig(opts ...Option) *Config {
	cfg := defaultConfig()
	loadFromEnv(cfg)
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

func TestConfigDefaults(t *testing.T) {
	cfg := defaultConfig()
	if cfg.BatchSize != 500 {
		t.Errorf("BatchSize default = %d, want 500", cfg.BatchSize)
	}
	if cfg.BatchInterval != 2*time.Second {
		t.Errorf("BatchInterval default = %v, want 2s", cfg.BatchInterval)
	}
	if cfg.BufferSize != 10000 {
		t.Errorf("BufferSize default = %d, want 10000", cfg.BufferSize)
	}
	if cfg.MinLevel != LevelInfo {
		t.Errorf("MinLevel default = %v, want info", cfg.MinLevel)
	}
	if !cfg.CompressionEnabled {
		t.Error("CompressionEnabled default = false, want true")
	}
	if cfg.Environment != "production" {
		t.Errorf("Environment default = %q, want production", cfg.Environment)
	}
}

func TestEnvFallback(t *testing.T) {
	t.Setenv("OPENTRACE_COLLECTOR_ENDPOINT", "https://collector.acme.dev")
	t.Setenv("OPENTRACE_SERVICE_NAME", "env-service")
	t.Setenv("OPENTRACE_SERVICE_VERSION", "2.3.4")
	t.Setenv("OPENTRACE_ENVIRONMENT", "staging")
	t.Setenv("OPENTRACE_MIN_LEVEL", "warn")
	t.Setenv("OPENTRACE_BATCH_SIZE", "250")
	t.Setenv("OPENTRACE_BATCH_INTERVAL_MS", "750")
	t.Setenv("OPENTRACE_BUFFER_SIZE", "2048")
	t.Setenv("OPENTRACE_HTTP_TIMEOUT_MS", "4000")
	t.Setenv("OPENTRACE_SHUTDOWN_TIMEOUT_MS", "5000")
	t.Setenv("OPENTRACE_DEBUG", "true")

	cfg := buildConfig()
	if cfg.CollectorEndpoint != "https://collector.acme.dev" {
		t.Errorf("CollectorEndpoint = %q", cfg.CollectorEndpoint)
	}
	if cfg.ServiceName != "env-service" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "2.3.4" {
		t.Errorf("ServiceVersion = %q", cfg.ServiceVersion)
	}
	if cfg.Environment != "staging" {
		t.Errorf("Environment = %q", cfg.Environment)
	}
	if cfg.MinLevel != LevelWarn {
		t.Errorf("MinLevel = %v", cfg.MinLevel)
	}
	if cfg.BatchSize != 250 {
		t.Errorf("BatchSize = %d", cfg.BatchSize)
	}
	if cfg.BatchInterval != 750*time.Millisecond {
		t.Errorf("BatchInterval = %v", cfg.BatchInterval)
	}
	if cfg.BufferSize != 2048 {
		t.Errorf("BufferSize = %d", cfg.BufferSize)
	}
	if cfg.HTTPTimeout != 4*time.Second {
		t.Errorf("HTTPTimeout = %v", cfg.HTTPTimeout)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout = %v", cfg.ShutdownTimeout)
	}
	if !cfg.Debug {
		t.Error("Debug = false, want true")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("env-driven config failed validation: %v", err)
	}
}

func TestProgrammaticOptionsBeatEnv(t *testing.T) {
	t.Setenv("OPENTRACE_COLLECTOR_ENDPOINT", "https://env-endpoint")
	t.Setenv("OPENTRACE_SERVICE_NAME", "env-service")
	t.Setenv("OPENTRACE_MIN_LEVEL", "error")
	t.Setenv("OPENTRACE_BATCH_SIZE", "100")
	t.Setenv("OPENTRACE_ENVIRONMENT", "staging")

	cfg := buildConfig(
		WithCollectorEndpoint("https://option-endpoint"),
		WithServiceName("option-service"),
		WithMinLevel(LevelDebug),
		WithBatchSize(999),
		WithEnvironment("qa"),
	)
	if cfg.CollectorEndpoint != "https://option-endpoint" {
		t.Errorf("CollectorEndpoint = %q — env var beat the explicit option", cfg.CollectorEndpoint)
	}
	if cfg.ServiceName != "option-service" {
		t.Errorf("ServiceName = %q — env var beat the explicit option", cfg.ServiceName)
	}
	if cfg.MinLevel != LevelDebug {
		t.Errorf("MinLevel = %v — env var beat the explicit option", cfg.MinLevel)
	}
	if cfg.BatchSize != 999 {
		t.Errorf("BatchSize = %d — env var beat the explicit option", cfg.BatchSize)
	}
	if cfg.Environment != "qa" {
		t.Errorf("Environment = %q — env var beat the explicit option", cfg.Environment)
	}
}

func TestWithHeaderAccumulates(t *testing.T) {
	cfg := buildConfig(
		WithHeader("X-API-Key", "k1"),
		WithHeader("X-Tenant", "acme"),
	)
	if cfg.Headers["X-API-Key"] != "k1" || cfg.Headers["X-Tenant"] != "acme" {
		t.Errorf("Headers = %v, want both entries", cfg.Headers)
	}
}

func TestValidateCollectsAllErrors(t *testing.T) {
	cfg := &Config{ // deliberately violates every rule at once
		CollectorEndpoint: "not a url",
		ServiceName:       "",
		BatchSize:         0,
		BatchInterval:     time.Millisecond,
		HTTPTimeout:       time.Millisecond,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil for a config violating five rules")
	}
	msg := err.Error()
	for _, fragment := range []string{
		"collector_endpoint must be a valid URL",
		"service_name is required",
		"batch_size must be between 1 and 5000",
		"batch_interval must be between 100ms and 60s",
		"http_timeout must be between 1s and 120s",
	} {
		if !strings.Contains(msg, fragment) {
			t.Errorf("Validate() error missing %q; got:\n%s", fragment, msg)
		}
	}
}

func TestValidateRequiresEndpointMessage(t *testing.T) {
	cfg := defaultConfig()
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "set OPENTRACE_COLLECTOR_ENDPOINT") {
		t.Errorf("empty-endpoint error should point at the env var; got: %v", err)
	}
}

func TestNewSpawnsNoGoroutinesOnInvalidConfig(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 10; i++ {
		if _, err := New(); err == nil {
			t.Fatal("New() with empty config succeeded")
		}
	}
	// Allow the runtime a moment to settle, then compare.
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before {
		t.Errorf("goroutines grew from %d to %d after 10 failed New() calls", before, after)
	}
}
