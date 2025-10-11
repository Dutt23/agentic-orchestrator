package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lyzr/orchestrator/common/config"
	"github.com/lyzr/orchestrator/common/logger"
)

// DB wraps pgxpool with common operations
type DB struct {
	*pgxpool.Pool
	log *logger.Logger
}

// New creates a new database connection pool
func New(ctx context.Context, cfg *config.Config, log *logger.Logger) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL())
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	// Configure connection pool
	poolConfig.MaxConns = int32(cfg.Database.MaxConns)
	poolConfig.MinConns = int32(cfg.Database.MinConns)
	poolConfig.MaxConnLifetime = cfg.Database.MaxLifetime
	poolConfig.MaxConnIdleTime = cfg.Database.MaxIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info("database connected", "host", cfg.Database.Host, "db", cfg.Database.Database)

	return &DB{
		Pool: pool,
		log:  log,
	}, nil
}

// Close closes the database connection pool
func (db *DB) Close() {
	db.log.Info("closing database connection pool")
	db.Pool.Close()
}

// Health checks database health
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	return db.Pool.Ping(ctx)
}
