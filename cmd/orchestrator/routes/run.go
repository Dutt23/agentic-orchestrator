package routes

import (
	"fmt"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
	_ "github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/logger"
	"github.com/redis/go-redis/v9"
)

// RegisterRunRoutes registers run and patch routes
func RegisterRunRoutes(e *echo.Echo, components *bootstrap.Components) {
	// Create Redis client
	redisClient := createRedisClient(components)

	// Create CAS client (mock for MVP)
	casClient := &mockCASClient{logger: components.Logger}

	// Create run handler
	runHandler := handlers.NewRunHandler(components, redisClient, casClient)

	// Placeholder handler for unimplemented routes
	placeholder := handlers.NewPlaceholderHandler(components)

	// Workflow execution routes
	workflows := e.Group("/api/v1/workflows")
	workflows.Use(middleware.ExtractUsername()) // Extract X-User-ID into context
	{
		workflows.POST("/:tag/execute", runHandler.ExecuteWorkflow) // POST /api/v1/workflows/:tag/execute
	}

	// Run routes
	runs := e.Group("/api/v1/runs")
	{
		runs.GET("/:id", runHandler.GetRun)                  // GET /api/v1/runs/{run_id}
		runs.GET("", placeholder.NotImplemented)             // GET /api/v1/runs?status=running (TODO)
		runs.POST("/:id/cancel", placeholder.NotImplemented) // POST /api/v1/runs/{run_id}/cancel (TODO)
		runs.POST("/:id/patch", runHandler.PatchRun)         // POST /api/v1/runs/{run_id}/patch
	}

	// Patch routes (not yet implemented)
	patches := e.Group("/api/v1/patches")
	{
		patches.POST("", placeholder.NotImplemented)    // POST /api/v1/patches
		patches.GET("/:id", placeholder.NotImplemented) // GET /api/v1/patches/{patch_id}
	}
}

// createRedisClient creates a Redis client from config
func createRedisClient(components *bootstrap.Components) *redis.Client {
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       0,
	})

	return client
}

// getEnv gets an environment variable or returns a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// mockCASClient is a placeholder CAS client for MVP
type mockCASClient struct {
	logger *logger.Logger
}

func (m *mockCASClient) Put(data []byte, contentType string) (string, error) {
	casID := fmt.Sprintf("cas://mock/%d", len(data))
	m.logger.Debug("mock CAS Put", "cas_id", casID, "size", len(data))
	return casID, nil
}

func (m *mockCASClient) Get(casID string) (interface{}, error) {
	m.logger.Debug("mock CAS Get", "cas_id", casID)
	return []byte("{}"), nil
}

func (m *mockCASClient) Store(data interface{}) (string, error) {
	casID := fmt.Sprintf("cas://mock/store")
	m.logger.Debug("mock CAS Store", "cas_id", casID)
	return casID, nil
}
