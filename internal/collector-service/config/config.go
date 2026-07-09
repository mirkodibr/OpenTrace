package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the collector-service.
// Values are loaded from environment variables at startup; the service
// fails fast if any required field is missing.
type Config struct {
	// HTTP server
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	// Ingest limits
	MaxBatchSize    int
	MaxPayloadBytes int64

	// Database
	DatabaseURL string
	DBMaxConns  int

	// Broker
	RedpandaBrokers   string
	RedpandaLogsTopic string

	// Observability
	LogLevel    string
	Environment string
}

// Load reads configuration from environment variables.
// It returns an error if any required variable is absent.
func Load() (*Config, error) {
	cfg := &Config{
		Port:            8080,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		MaxBatchSize:    1000,
		MaxPayloadBytes: 5 * 1024 * 1024, // 5 MB
		DBMaxConns:      20,
		LogLevel:        "info",
		Environment:     "development",
	}

	var errs []error

	if v, ok := os.LookupEnv("COLLECTOR_PORT"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("COLLECTOR_PORT: %w", err))
		} else {
			cfg.Port = n
		}
	}

	if v, ok := os.LookupEnv("COLLECTOR_MAX_BATCH_SIZE"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("COLLECTOR_MAX_BATCH_SIZE: %w", err))
		} else {
			cfg.MaxBatchSize = n
		}
	}

	if v, ok := os.LookupEnv("COLLECTOR_MAX_PAYLOAD_BYTES"); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("COLLECTOR_MAX_PAYLOAD_BYTES: %w", err))
		} else {
			cfg.MaxPayloadBytes = n
		}
	}

	// Required fields
	dbURL, ok := os.LookupEnv("COLLECTOR_DATABASE_URL")
	if !ok || dbURL == "" {
		errs = append(errs, errors.New("COLLECTOR_DATABASE_URL is required"))
	}
	cfg.DatabaseURL = dbURL

	brokers, ok := os.LookupEnv("REDPANDA_BROKERS")
	if !ok || brokers == "" {
		errs = append(errs, errors.New("REDPANDA_BROKERS is required"))
	}
	cfg.RedpandaBrokers = brokers

	if v := os.Getenv("REDPANDA_LOGS_TOPIC"); v != "" {
		cfg.RedpandaLogsTopic = v
	} else {
		cfg.RedpandaLogsTopic = "opentrace.logs"
	}

	if v := os.Getenv("COLLECTOR_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("ENVIRONMENT"); v != "" {
		cfg.Environment = v
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

// Log emits a structured INFO entry with all config values (passwords redacted).
func (c *Config) Log(logger *slog.Logger) {
	logger.Info("collector-service configuration loaded",
		slog.Int("port", c.Port),
		slog.Int("max_batch_size", c.MaxBatchSize),
		slog.Int64("max_payload_bytes", c.MaxPayloadBytes),
		slog.String("db_url", redact(c.DatabaseURL)),
		slog.String("redpanda_brokers", c.RedpandaBrokers),
		slog.String("redpanda_logs_topic", c.RedpandaLogsTopic),
		slog.String("log_level", c.LogLevel),
		slog.String("environment", c.Environment),
	)
}

// redact masks the password component of a DSN for safe logging.
func redact(dsn string) string {
	if len(dsn) > 12 {
		return dsn[:12] + "***"
	}
	return "***"
}
