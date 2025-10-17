package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/handlers"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/middleware"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
)

// RegisterTestRoutes registers test/benchmark endpoints for workflow-runner
// Protected by X-Test-Token header
func RegisterTestRoutes(e *echo.Echo, components *bootstrap.Components, casClient clients.CASClient, orchestratorURL string) {
	testHandler := handlers.NewTestHandler(components, casClient, orchestratorURL)

	// Test endpoints group with authentication
	test := e.Group("/api/v1/test", middleware.TestAuthMiddleware())
	{
		// Fetch from orchestrator (tests inter-service communication)
		test.GET("/fetch-from-orchestrator/:run_id", testHandler.FetchFromOrchestrator)

		// Fetch from CAS (tests CAS with mover)
		test.GET("/fetch-from-cas/:cas_id", testHandler.FetchFromCAS)
	}

	components.Logger.Info("Test endpoints registered in workflow-runner")
}
