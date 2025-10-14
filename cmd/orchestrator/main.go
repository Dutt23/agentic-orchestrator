package main

import (
	"context"
	"fmt"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lyzr/orchestrator/cmd/orchestrator/container"
	"github.com/lyzr/orchestrator/cmd/orchestrator/routes"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

func main() {
	ctx := context.Background()

	// Bootstrap common components (DB, logger, queue, cache, telemetry)
	components, err := bootstrap.Setup(ctx, "orchestrator")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bootstrap orchestrator: %v\n", err)
		os.Exit(1)
	}
	defer components.Shutdown(ctx)

	// Initialize service container (singleton pattern - all services created once)
	serviceContainer, err := container.NewContainer(components)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize service container: %v\n", err)
		os.Exit(1)
	}

	// Initialize Echo server
	e := setupEcho()

	// Setup middleware
	setupMiddleware(e)

	// Setup health check
	setupHealthCheck(e)

	// Register all routes
	registerRoutes(e, serviceContainer)

	// Start server
	startServer(e, components)
}

// setupEcho initializes the Echo server with basic configuration
func setupEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	return e
}

// setupMiddleware configures all middleware for the Echo server
func setupMiddleware(e *echo.Echo) {
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.RequestID())
}

// setupHealthCheck registers the health check endpoint
func setupHealthCheck(e *echo.Echo) {
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status":  "ok",
			"service": "orchestrator",
		})
	})
}

// registerRoutes registers all application routes using the service container
func registerRoutes(e *echo.Echo, serviceContainer *container.Container) {
	routes.RegisterWorkflowRoutes(e, serviceContainer)
	routes.RegisterTagRoutes(e, serviceContainer)
	routes.RegisterRunRoutes(e, serviceContainer)
	routes.RegisterRunPatchRoutes(e, serviceContainer)
}

// startServer starts the Echo server on the configured port
func startServer(e *echo.Echo, components *bootstrap.Components) {
	port := components.Config.Service.Port
	components.Logger.Info("Starting orchestrator", "port", port)

	// Start with graceful shutdown
	if err := e.Start(fmt.Sprintf(":%d", port)); err != nil {
		components.Logger.Error("Server error", "error", err)
		os.Exit(1)
	}
}
