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

// Setup initializes all service components
// This is the main entry point for all services
func Setup(ctx context.Context, serviceName string, opts ...Option) (*Components, error) {
	fmt.Println("%s", opts)
	// Apply options
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	components := &Components{
		cleanupFuncs: make([]func() error, 0),
	}

	// 1. Load configuration
	var err error
	if options.customConfig != nil {
		components.Config = options.customConfig
	} else {
		components.Config, err = config.Load(serviceName)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	// 2. Initialize logger
	if options.customLogger != nil {
		components.Logger = options.customLogger
	} else {
		components.Logger = logger.New(
			components.Config.Service.LogLevel,
			components.Config.Service.LogFormat,
		)
	}

	components.Logger.Info("initializing service",
		"service", serviceName,
		"environment", components.Config.Service.Environment,
	)

	// 3. Initialize database (if not skipped)
	if !options.skipDB {
		components.Logger.Info("connecting to database")
		components.DB, err = db.New(ctx, components.Config, components.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}

		// Register cleanup
		components.addCleanup(func() error {
			components.Logger.Info("closing database connection")
			components.DB.Close()
			return nil
		})

		// Run DB init hook if provided
		if options.dbInitHook != nil {
			components.Logger.Info("running database init hook")
			if err := options.dbInitHook(components.DB); err != nil {
				components.Shutdown(ctx) // Cleanup what we've initialized
				return nil, fmt.Errorf("database init hook failed: %w", err)
			}
		}
	}

	// 4. Initialize queue (if not skipped)
	if !options.skipQueue {
		components.Logger.Info("initializing queue",
			"type", components.Config.Queue.Type,
		)

		switch components.Config.Queue.Type {
		case "memory":
			components.Queue = queue.NewMemoryQueue(components.Logger)
		case "kafka":
			// TODO: Implement Kafka queue for production
			return nil, fmt.Errorf("kafka queue not yet implemented")
		default:
			return nil, fmt.Errorf("unknown queue type: %s", components.Config.Queue.Type)
		}

		// Register cleanup
		components.addCleanup(func() error {
			components.Logger.Info("closing queue")
			return components.Queue.Close()
		})
	}

	// 5. Initialize cache (if not skipped)
	if !options.skipCache && components.Config.Cache.Enabled {
		components.Logger.Info("initializing cache",
			"size_mb", components.Config.Cache.SizeMB,
		)

		// For MVP, always use memory cache
		components.Cache = cache.NewMemoryCache(components.Logger)

		// Register cleanup
		components.addCleanup(func() error {
			components.Logger.Info("closing cache")
			return components.Cache.Close()
		})
	}

	// 6. Initialize telemetry (if not skipped)
	if !options.skipTelemetry && components.Config.Telemetry.EnablePprof {
		components.Logger.Info("initializing telemetry")
		components.Telemetry = telemetry.New(
			components.Config.Telemetry.PprofPort,
			components.Config.Telemetry.MetricsPort,
			components.Logger,
		)

		if err := components.Telemetry.Start(ctx); err != nil {
			components.Logger.Warn("failed to start telemetry", "error", err)
			// Don't fail startup if telemetry fails
		}
	}

	components.Logger.Info("service initialization complete",
		"service", serviceName,
		"db", components.DB != nil,
		"queue", components.Queue != nil,
		"cache", components.Cache != nil,
		"telemetry", components.Telemetry != nil,
	)

	return components, nil
}

// MustSetup is like Setup but panics on error
// Useful for services that can't recover from initialization failure
func MustSetup(ctx context.Context, serviceName string, opts ...Option) *Components {
	components, err := Setup(ctx, serviceName, opts...)
	if err != nil {
		panic(fmt.Sprintf("failed to setup service %s: %v", serviceName, err))
	}
	return components
}
