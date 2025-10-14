package routes

import (
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/container"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
	_ "github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/lyzr/orchestrator/common/logger"
)

// RegisterRunRoutes registers run and patch routes
func RegisterRunRoutes(e *echo.Echo, c *container.Container) {
	// Create CAS client (mock for MVP)
	casClient := &mockCASClient{logger: c.Components.Logger}

	// Create handlers using services from container
	runHandler := handlers.NewRunHandler(c.Components, c.Redis, casClient, c.RunService)
	artifactHandler := handlers.NewArtifactHandler(c.Components, c.CASService, c.ArtifactService)

	// Placeholder handler for unimplemented routes
	placeholder := handlers.NewPlaceholderHandler(c.Components)

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

	// Artifact routes
	artifacts := e.Group("/api/v1/artifacts")
	{
		artifacts.GET("/:id", artifactHandler.GetArtifact) // GET /api/v1/artifacts/{artifact_id}
	}
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
