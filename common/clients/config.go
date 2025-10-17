package clients

import (
	"os"
	"strings"
	"sync"
)

// ClientConfig holds all client configuration loaded from environment
// Read once at startup and passed to all client constructors
type ClientConfig struct {
	// Mover settings
	UseMover     bool
	MoverSocket  string

	// CAS backend selection
	UseCASPostgres bool

	// HTTP client settings
	// (Add more as needed)
}

var (
	globalConfig *ClientConfig
	configOnce   sync.Once
)

// LoadClientConfig loads client configuration from environment variables
// This should be called once at application startup
func LoadClientConfig() *ClientConfig {
	configOnce.Do(func() {
		globalConfig = &ClientConfig{
			UseMover:       strings.ToLower(os.Getenv("USE_MOVER")) == "true",
			MoverSocket:    getEnvOrDefault("MOVER_SOCKET", "/tmp/mover.sock"),
			UseCASPostgres: strings.ToLower(os.Getenv("USE_CAS_POSTGRES")) == "true",
		}
	})

	return globalConfig
}

// GetClientConfig returns the global client config (loads if not already loaded)
func GetClientConfig() *ClientConfig {
	return LoadClientConfig()
}

// Helper to get env with default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
