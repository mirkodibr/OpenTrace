package opentrace

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime parameters for the SDK. Use the functional-options
// constructor (New with WithXxx options) rather than populating this directly.
type Config struct {
	// Required
	CollectorEndpoint string
	ServiceName       string

	// Optional with defaults
	ServiceVersion string
	Environment    string
	MinLevel       Level
	BatchSize      int
	BatchInterval  time.Duration
	MaxBatchBytes  int
	BufferSize     int
	HTTPTimeout    time.Duration
	MaxRetries     int
	Headers        map[string]string

	// Advanced
	CompressionEnabled bool
	Debug              bool
}

func defaultConfig() *Config {
	return &Config{
		ServiceVersion:     "unknown",
		Environment:        "production",
		MinLevel:           LevelInfo,
		BatchSize:          500,
		BatchInterval:      2 * time.Second,
		MaxBatchBytes:      1 * 1024 * 1024, // 1 MB
		BufferSize:         10000,
		HTTPTimeout:        10 * time.Second,
		MaxRetries:         5,
		Headers:            map[string]string{},
		CompressionEnabled: true,
	}
}

// Validate checks that all required fields are present and within bounds.
// It returns all validation errors at once so callers can fix everything in
// one iteration rather than discovering errors one by one.
func (c *Config) Validate() error {
	var errs []error

	if c.CollectorEndpoint == "" {
		errs = append(errs, errors.New("collector_endpoint is required; set OPENTRACE_COLLECTOR_ENDPOINT"))
	} else {
		u, err := url.Parse(c.CollectorEndpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			errs = append(errs, errors.New("collector_endpoint must be a valid URL with http/https scheme"))
		}
	}

	if c.ServiceName == "" {
		errs = append(errs, errors.New("service_name is required; set OPENTRACE_SERVICE_NAME"))
	}

	if c.BatchSize < 1 || c.BatchSize > 5000 {
		errs = append(errs, fmt.Errorf("batch_size must be between 1 and 5000, got %d", c.BatchSize))
	}

	if c.BatchInterval < 100*time.Millisecond || c.BatchInterval > 60*time.Second {
		errs = append(errs, fmt.Errorf("batch_interval must be between 100ms and 60s, got %s", c.BatchInterval))
	}

	if c.HTTPTimeout < time.Second || c.HTTPTimeout > 120*time.Second {
		errs = append(errs, fmt.Errorf("http_timeout must be between 1s and 120s, got %s", c.HTTPTimeout))
	}

	return errors.Join(errs...)
}

// loadFromEnv populates zero-value Config fields from environment variables.
// Programmatic options (already set) take priority — this only fills gaps.
func loadFromEnv(cfg *Config) {
	if cfg.CollectorEndpoint == "" {
		cfg.CollectorEndpoint = os.Getenv("OPENTRACE_COLLECTOR_ENDPOINT")
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = os.Getenv("OPENTRACE_SERVICE_NAME")
	}
	if cfg.ServiceVersion == "unknown" {
		if v := os.Getenv("OPENTRACE_SERVICE_VERSION"); v != "" {
			cfg.ServiceVersion = v
		}
	}
	if v := os.Getenv("OPENTRACE_ENVIRONMENT"); v != "" && cfg.Environment == "production" {
		cfg.Environment = v
	}
	if v := os.Getenv("OPENTRACE_MIN_LEVEL"); v != "" {
		if level, ok := ParseLevel(v); ok {
			cfg.MinLevel = level
		}
	}
	if v := os.Getenv("OPENTRACE_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.BatchSize = n
		}
	}
	if v := os.Getenv("OPENTRACE_BATCH_INTERVAL_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.BatchInterval = time.Duration(n) * time.Millisecond
		}
	}
	if v := os.Getenv("OPENTRACE_BUFFER_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.BufferSize = n
		}
	}
	if v := os.Getenv("OPENTRACE_HTTP_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.HTTPTimeout = time.Duration(n) * time.Millisecond
		}
	}
	if v := os.Getenv("OPENTRACE_DEBUG"); v == "true" || v == "1" {
		cfg.Debug = true
	}
}
