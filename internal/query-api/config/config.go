package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the query-api service.
type Config struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	DatabaseURL     string
	DBMaxConns      int
	DefaultPageSize int
	MaxTimeRangeDays int

	LogLevel    string
	Environment string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:             8081,
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     10 * time.Second,
		IdleTimeout:      120 * time.Second,
		ShutdownTimeout:  30 * time.Second,
		DBMaxConns:       20,
		DefaultPageSize:  100,
		MaxTimeRangeDays: 7,
		LogLevel:         "info",
		Environment:      "development",
	}

	var errs []error

	if v, ok := os.LookupEnv("QUERY_API_PORT"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("QUERY_API_PORT: %w", err))
		} else {
			cfg.Port = n
		}
	}

	if v, ok := os.LookupEnv("QUERY_API_DEFAULT_PAGE_SIZE"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("QUERY_API_DEFAULT_PAGE_SIZE: %w", err))
		} else {
			cfg.DefaultPageSize = n
		}
	}

	if v, ok := os.LookupEnv("QUERY_API_MAX_TIME_RANGE_DAYS"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("QUERY_API_MAX_TIME_RANGE_DAYS: %w", err))
		} else {
			cfg.MaxTimeRangeDays = n
		}
	}

	dbURL, ok := os.LookupEnv("QUERY_API_DATABASE_URL")
	if !ok || dbURL == "" {
		errs = append(errs, errors.New("QUERY_API_DATABASE_URL is required"))
	}
	cfg.DatabaseURL = dbURL

	if v := os.Getenv("QUERY_API_LOG_LEVEL"); v != "" {
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
	logger.Info("query-api configuration loaded",
		slog.Int("port", c.Port),
		slog.String("db_url", redact(c.DatabaseURL)),
		slog.Int("default_page_size", c.DefaultPageSize),
		slog.Int("max_time_range_days", c.MaxTimeRangeDays),
		slog.String("log_level", c.LogLevel),
		slog.String("environment", c.Environment),
	)
}

func redact(dsn string) string {
	if len(dsn) > 12 {
		return dsn[:12] + "***"
	}
	return "***"
}
