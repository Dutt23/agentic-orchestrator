package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/server"
)

func main() {
	ctx := context.Background()

	// Runner doesn't need database, only queue and cache
	components, err := bootstrap.Setup(ctx, "runner",
		bootstrap.WithoutDB(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
		os.Exit(1)
	}
	defer components.Shutdown(ctx)

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.HealthHandler())

	// TODO: Add runner-specific routes
	// mux.HandleFunc("/execute", handleExecute)

	// Start HTTP server
	srv := server.New(
		components.Config.Service.Name,
		components.Config.Service.Port,
		mux,
		components.Logger,
	)

	components.Logger.Info("runner service ready",
		"port", components.Config.Service.Port,
		"workers", os.Getenv("RUNNER_WORKERS"),
	)

	if err := srv.Start(); err != nil {
		components.Logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
