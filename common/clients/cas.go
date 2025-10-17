package clients

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// CASClient interface for content-addressable storage
// All implementations must be context-aware and thread-safe
type CASClient interface {
	Get(ctx context.Context, ref string) (interface{}, error)
	Put(ctx context.Context, data []byte, mediaType string) (string, error)
	Store(ctx context.Context, data interface{}) (string, error)
}

// NewCASClient creates a CAS client based on configuration
// Configuration is loaded once at startup and cached
//
// Two independent flags:
//   USE_MOVER=true        → Use mover for I/O optimization (io_uring)
//   USE_CAS_POSTGRES=true → Use Postgres for storage (vs Redis)
//
// Combinations:
//   false, false → Redis CAS, direct
//   true,  false → Redis CAS, via mover
//   false, true  → Postgres CAS, direct
//   true,  true  → Postgres CAS, via mover
//
// NO CACHING of data - always queries backing store for fresh data
func NewCASClient(redis *redis.Client, logger Logger) (CASClient, error) {
	// Get config (loads once, cached)
	config := GetClientConfig()

	// Determine backend
	backend := "Redis"
	if config.UseCASPostgres {
		backend = "Postgres"
		// TODO: Implement PostgresCASClient
		logger.Warn("Postgres CAS not yet implemented, falling back to Redis")
	}

	// Determine transport
	if config.UseMover {
		logger.Info("Using mover for CAS operations", "backend", backend, "transport", "io_uring", "socket", config.MoverSocket)
		return NewMoverCASClient(config)
	}

	logger.Info("Using direct CAS operations", "backend", backend, "transport", "standard")
	return NewRedisCASClient(redis, logger), nil
}
