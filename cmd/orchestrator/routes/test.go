package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/container"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
)

// RegisterTestRoutes registers test/benchmark endpoints
// These are PROTECTED by X-Test-Token header and NOT for production use
// Only accessible with valid PERF_TEST_TOKEN
func RegisterTestRoutes(e *echo.Echo, c *container.Container) {
	// Create test handler (creates its own CAS client with mover routing)
	testHandler := handlers.NewTestHandler(c.Components, c.Redis, c.RedisRaw)

	// Test endpoints group with authentication middleware
	// Requires X-Test-Token header matching PERF_TEST_TOKEN env var
	test := e.Group("/api/v1/test", middleware.TestAuthMiddleware())
	{
		// Fetch workflow IR (what workflow-runner does)
		test.GET("/fetch-workflow/:run_id", testHandler.FetchWorkflowIR)

		// Fetch from CAS
		test.GET("/fetch-cas/:cas_id", testHandler.FetchFromCAS)

		// Create test workflow
		test.POST("/create-workflow", testHandler.CreateTestWorkflow)
	}

	c.Components.Logger.Info("Test endpoints registered (protected by X-Test-Token header)")
}
