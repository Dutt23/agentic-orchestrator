package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all service configuration
type Config struct {
	Service    ServiceConfig
	Database   DatabaseConfig
	Cache      CacheConfig
	Queue      QueueConfig
	Telemetry  TelemetryConfig
	Features   FeatureFlags
}

// ServiceConfig holds service-specific settings
type ServiceConfig struct {
	Name        string
	Port        int
	Environment string
	LogLevel    string
	LogFormat   string
}

// DatabaseConfig holds Postgres connection settings
type DatabaseConfig struct {
	Host         string
	Port         int
	Database     string
	User         string
	Password     string
	MaxConns     int
	MinConns     int
	MaxIdleTime  time.Duration
	MaxLifetime  time.Duration
}

// CacheConfig holds cache settings
type CacheConfig struct {
	Enabled    bool
	SizeMB     int
	DefaultTTL time.Duration
}

// QueueConfig holds message queue settings
type QueueConfig struct {
	Type      string // "memory" for MVP, "kafka" for production
	Brokers   []string
	BatchSize int
	LingerMS  int
}

// TelemetryConfig holds observability settings
type TelemetryConfig struct {
	EnablePprof    bool
	PprofPort      int
	EnableTracing  bool
	EnableMetrics  bool
	MetricsPort    int
	TracingBackend string
}

// FeatureFlags for MVP toggles
type FeatureFlags struct {
	EnableKafka            bool
	EnableK8sRunner        bool
	EnableWASMOptimizer    bool
	EnableDistributedCache bool
}

// Load loads configuration from environment variables
func Load(serviceName string) (*Config, error) {
	cfg := &Config{
		Service: ServiceConfig{
			Name:        serviceName,
			Port:        getEnvInt("PORT", 8080),
			Environment: getEnv("ENVIRONMENT", "development"),
			LogLevel:    getEnv("LOG_LEVEL", "info"),
			LogFormat:   getEnv("LOG_FORMAT", "text"), // Default to text for development
		},
		Database: DatabaseConfig{
			Host:        getEnv("POSTGRES_HOST", "localhost"),
			Port:        getEnvInt("POSTGRES_PORT", 5432),
			Database:    getEnv("POSTGRES_DB", "orchestrator"),
			User:        getEnv("POSTGRES_USER", "orchestrator"),
			Password:    getEnv("POSTGRES_PASSWORD", "orchestrator"),
			MaxConns:    getEnvInt("POSTGRES_MAX_CONNS", 50),
			MinConns:    getEnvInt("POSTGRES_MIN_CONNS", 10),
			MaxIdleTime: getEnvDuration("POSTGRES_MAX_IDLE_TIME", 30*time.Minute),
			MaxLifetime: getEnvDuration("POSTGRES_MAX_LIFETIME", 1*time.Hour),
		},
		Cache: CacheConfig{
			Enabled:    getEnvBool("CACHE_ENABLED", true),
			SizeMB:     getEnvInt("CACHE_SIZE_MB", 512),
			DefaultTTL: getEnvDuration("CACHE_DEFAULT_TTL", 1*time.Hour),
		},
		Queue: QueueConfig{
			Type:      getEnv("QUEUE_TYPE", "memory"),
			Brokers:   getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			BatchSize: getEnvInt("KAFKA_BATCH_SIZE", 1000),
			LingerMS:  getEnvInt("KAFKA_LINGER_MS", 10),
		},
		Telemetry: TelemetryConfig{
			EnablePprof:    getEnvBool("ENABLE_PPROF", true),
			PprofPort:      getEnvInt("PPROF_PORT", 6060),
			EnableTracing:  getEnvBool("ENABLE_TRACING", true),
			EnableMetrics:  getEnvBool("ENABLE_METRICS", true),
			MetricsPort:    getEnvInt("METRICS_PORT", 9090),
			TracingBackend: getEnv("TRACING_BACKEND", "stdout"),
		},
		Features: FeatureFlags{
			EnableKafka:            getEnvBool("ENABLE_KAFKA", false),
			EnableK8sRunner:        getEnvBool("ENABLE_K8S_RUNNER", false),
			EnableWASMOptimizer:    getEnvBool("ENABLE_WASM_OPTIMIZER", false),
			EnableDistributedCache: getEnvBool("ENABLE_DISTRIBUTED_CACHE", false),
		},
	}

	return cfg, cfg.Validate()
}

// Validate checks if configuration is valid
func (c *Config) Validate() error {
	if c.Service.Port < 1 || c.Service.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Service.Port)
	}

	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}

	if c.Database.MaxConns < c.Database.MinConns {
		return fmt.Errorf("max_conns must be >= min_conns")
	}

	return nil
}

// DatabaseURL returns the PostgreSQL connection string
func (c *Config) DatabaseURL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.Database.User,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.Database,
	)
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Simple comma-separated parsing
		// For production, use a proper CSV parser
		return []string{value}
	}
	return defaultValue
}