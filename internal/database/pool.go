package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig holds connection pool parameters. Defaults are tuned for a
// single service replica sharing a 100-connection PostgreSQL instance.
type PoolConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnIdleTime time.Duration
	MaxConnLifetime time.Duration
	LifetimeJitter  time.Duration
	HealthCheck     time.Duration
}

func defaultPoolConfig(dsn string) PoolConfig {
	return PoolConfig{
		DSN:             dsn,
		MaxConns:        20,
		MinConns:        2,
		MaxConnIdleTime: 5 * time.Minute,
		MaxConnLifetime: 30 * time.Minute,
		LifetimeJitter:  1 * time.Minute,
		HealthCheck:     1 * time.Minute,
	}
}

// NewPool opens and validates a pgxpool connection pool.
// The pool is ready to use immediately on return; the caller owns its lifecycle
// and must call pool.Close() on shutdown.
func NewPool(ctx context.Context, dsn string, logger *slog.Logger) (*pgxpool.Pool, error) {
	cfg := defaultPoolConfig(dsn)
	return NewPoolWithConfig(ctx, cfg, logger)
}

// NewPoolWithConfig constructs a pool from an explicit PoolConfig.
func NewPoolWithConfig(ctx context.Context, cfg PoolConfig, logger *slog.Logger) (*pgxpool.Pool, error) {
	pxCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	pxCfg.MaxConns = cfg.MaxConns
	pxCfg.MinConns = cfg.MinConns
	pxCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	pxCfg.MaxConnLifetime = cfg.MaxConnLifetime
	pxCfg.MaxConnLifetimeJitter = cfg.LifetimeJitter
	pxCfg.HealthCheckPeriod = cfg.HealthCheck

	pool, err := pgxpool.NewWithConfig(ctx, pxCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	logger.Info("database pool ready",
		slog.Int("max_conns", int(cfg.MaxConns)),
		slog.Int("min_conns", int(cfg.MinConns)),
	)
	return pool, nil
}
