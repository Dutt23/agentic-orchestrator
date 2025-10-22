package testserver

import (
	"os"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/routes"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
)

// StartTestServer starts an optional HTTP server for performance testing
// Only starts if ENABLE_TEST_SERVER=true
// This is NOT for production - only for measuring inter-service performance
//
// To remove this feature entirely:
// 1. Delete this file (cmd/workflow-runner/testserver/server.go)
// 2. Remove StartTestServerIfEnabled() call from main.go
// 3. Remove import "cmd/workflow-runner/testserver"
func StartTestServerIfEnabled(
	components *bootstrap.Components,
	casClient clients.CASClient,
	orchestratorURL string,
) {
	if os.Getenv("ENABLE_TEST_SERVER") != "true" {
		components.Logger.Info("Test HTTP server disabled (ENABLE_TEST_SERVER not set)")
		return
	}

	components.Logger.Info("⚠️  Starting test HTTP server (ENABLE_TEST_SERVER=true)")
	components.Logger.Info("   This is for performance testing only")

	// Create Echo server
	e := echo.New()
	e.HideBanner = true

	// Register test routes
	routes.RegisterTestRoutes(e, components, casClient, orchestratorURL)

	// Get port
	port := os.Getenv("TEST_PORT")
	if port == "" {
		port = "8082"
	}

	// Start in background
	go func() {
		addr := ":" + port
		components.Logger.Info("Test HTTP server listening", "port", port)

		if err := e.Start(addr); err != nil {
			components.Logger.Error("Test HTTP server failed", "error", err)
		}
	}()
}
