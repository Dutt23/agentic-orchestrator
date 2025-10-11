package bootstrap

import (
	"context"
	"fmt"

	"github.com/lyzr/orchestrator/common/cache"
	"github.com/lyzr/orchestrator/common/config"
	"github.com/lyzr/orchestrator/common/db"
	"github.com/lyzr/orchestrator/common/logger"
	"github.com/lyzr/orchestrator/common/queue"
	"github.com/lyzr/orchestrator/common/telemetry"
)

// Components holds all initialized service dependencies
type Components struct {
	Config    *config.Config
	Logger    *logger.Logger
	DB        *db.DB
	Queue     queue.Queue
	Cache     cache.Cache
	Telemetry *telemetry.Telemetry

	// Internal
	cleanupFuncs []func() error
}

// Shutdown performs graceful shutdown of all components
// Should be called with defer after Setup()
func (c *Components) Shutdown(ctx context.Context) error {
	c.Logger.Info("shutting down components")

	var errors []error

	// Run cleanup functions in reverse order (LIFO)
	for i := len(c.cleanupFuncs) - 1; i >= 0; i-- {
		if err := c.cleanupFuncs[i](); err != nil {
			errors = append(errors, err)
			c.Logger.Error("cleanup error", "error", err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	c.Logger.Info("shutdown complete")
	return nil
}

// Health checks health of all components
func (c *Components) Health(ctx context.Context) error {
	// Check database
	if c.DB != nil {
		if err := c.DB.Health(ctx); err != nil {
			return fmt.Errorf("database unhealthy: %w", err)
		}
	}

	// Queue health check (memory queue is always healthy)
	// Cache health check (memory cache is always healthy)

	return nil
}

// addCleanup registers a cleanup function
func (c *Components) addCleanup(fn func() error) {
	c.cleanupFuncs = append(c.cleanupFuncs, fn)
}
