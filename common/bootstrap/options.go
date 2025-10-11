package bootstrap

import (
	"github.com/lyzr/orchestrator/common/config"
	"github.com/lyzr/orchestrator/common/db"
	"github.com/lyzr/orchestrator/common/logger"
)

// Option configures the bootstrap process
type Option func(*options)

type options struct {
	skipDB        bool
	skipQueue     bool
	skipCache     bool
	skipTelemetry bool
	customLogger  *logger.Logger
	customConfig  *config.Config
	dbInitHook    func(*db.DB) error
}

// WithoutDB skips database initialization
func WithoutDB() Option {
	return func(o *options) {
		o.skipDB = true
	}
}

// WithoutQueue skips queue initialization
func WithoutQueue() Option {
	return func(o *options) {
		o.skipQueue = true
	}
}

// WithoutCache skips cache initialization
func WithoutCache() Option {
	return func(o *options) {
		o.skipCache = true
	}
}

// WithoutTelemetry skips telemetry initialization
func WithoutTelemetry() Option {
	return func(o *options) {
		o.skipTelemetry = true
	}
}

// WithCustomLogger uses a custom logger instead of creating one
func WithCustomLogger(log *logger.Logger) Option {
	return func(o *options) {
		o.customLogger = log
	}
}

// WithCustomConfig uses a custom config instead of loading from env
func WithCustomConfig(cfg *config.Config) Option {
	return func(o *options) {
		o.customConfig = cfg
	}
}

// WithDBInitHook runs a custom function after DB initialization
// Useful for running migrations, seeding data, etc.
func WithDBInitHook(hook func(*db.DB) error) Option {
	return func(o *options) {
		o.dbInitHook = hook
	}
}

func defaultOptions() *options {
	return &options{
		skipDB:        false,
		skipQueue:     false,
		skipCache:     false,
		skipTelemetry: false,
	}
}
