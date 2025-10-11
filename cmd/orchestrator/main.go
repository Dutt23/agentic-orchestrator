package main

import (
	"context"
	"fmt"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

	// Initialize Echo
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.RequestID())

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status":  "ok",
			"service": "orchestrator",
		})
	})

	// Register routes (modular)
	routes.RegisterWorkflowRoutes(e, components)
	routes.RegisterTagRoutes(e, components)
	routes.RegisterRunRoutes(e, components)

	// Start server
	port := components.Config.Service.Port
	components.Logger.Info("Starting orchestrator", "port", port)

	// Start with graceful shutdown
	if err := e.Start(fmt.Sprintf(":%d", port)); err != nil {
		components.Logger.Error("Server error", "error", err)
		os.Exit(1)
	}
}
