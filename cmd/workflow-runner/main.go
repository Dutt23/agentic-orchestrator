package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/consumer"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/coordinator"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/executor"
	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/supervisor"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bootstrap service components
	components, err := bootstrap.Setup(ctx, "workflow-runner")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup service: %v\n", err)
		os.Exit(1)
	}
	defer components.Shutdown(ctx)

	components.Logger.Info("workflow-runner starting")

	// Initialize dependencies
	deps, err := initializeDependencies(ctx, components)
	if err != nil {
		components.Logger.Error("failed to initialize dependencies", "error", err)
		os.Exit(1)
	}

	// Create all workflow components
	workflowComponents := createWorkflowComponents(deps, components)

	// Start all components
	errChan := startComponents(ctx, workflowComponents, components)

	components.Logger.Info("workflow-runner started successfully",
		"components", []string{"coordinator", "run_request_consumer", "status_update_consumer"},
		"note", "workers (http, hitl) now run as separate services")

	// Wait for shutdown signal or error
	waitForShutdown(ctx, cancel, errChan, components)

	components.Logger.Info("workflow-runner shutting down gracefully")
}

// dependencies holds all external dependencies needed by workflow components
type dependencies struct {
	redisClient     *redis.Client
	casClient       clients.CASClient
	workflowSDK     *sdk.SDK
	orchestratorURL string
}

// workflowComponents holds all workflow-runner components
type workflowComponents struct {
	coordinator    *coordinator.Coordinator
	runConsumer    *executor.RunRequestConsumer
	statusConsumer *consumer.StatusUpdateConsumer
}

// initializeDependencies sets up Redis, CAS client, and SDK
func initializeDependencies(ctx context.Context, components *bootstrap.Components) (*dependencies, error) {
	// Create Redis client
	redisClient, err := createRedisClient(components)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %w", err)
	}

	// Ping Redis
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}
	components.Logger.Info("connected to Redis")

	// Load Lua script for apply_delta
	luaScript, err := os.ReadFile("scripts/apply_delta.lua")
	if err != nil {
		return nil, fmt.Errorf("failed to load Lua script: %w", err)
	}

	// Create Redis-based CAS client for storing execution results
	casClient := clients.NewRedisCASClient(redisClient, components.Logger)

	// Create SDK
	workflowSDK := sdk.NewSDK(redisClient, casClient, components.Logger, string(luaScript))

	// Get orchestrator URL
	orchestratorURL := getEnv("ORCHESTRATOR_URL", "http://localhost:8081")

	return &dependencies{
		redisClient:     redisClient,
		casClient:       casClient,
		workflowSDK:     workflowSDK,
		orchestratorURL: orchestratorURL,
	}, nil
}

// createWorkflowComponents initializes all workflow-runner components
func createWorkflowComponents(deps *dependencies, components *bootstrap.Components) *workflowComponents {
	// TODO: Create and start supervisors when needed
	// For MVP integration tests, we only need the coordinator
	_ = supervisor.NewCompletionSupervisor // Avoid unused import error
	_ = supervisor.NewTimeoutDetector

	// Create run repository for status updates
	runRepo := repository.NewRunRepository(components.DB)

	return &workflowComponents{
		coordinator: coordinator.NewCoordinator(&coordinator.CoordinatorOpts{
			Redis:               deps.redisClient,
			SDK:                 deps.workflowSDK,
			Logger:              components.Logger,
			OrchestratorBaseURL: deps.orchestratorURL,
			CASClient:           deps.casClient,
		}),
		runConsumer:    executor.NewRunRequestConsumer(deps.redisClient, deps.workflowSDK, components.Logger, deps.orchestratorURL),
		statusConsumer: consumer.NewStatusUpdateConsumer(deps.redisClient, runRepo, components.Logger),
	}
}

// startComponents starts all workflow components in goroutines
func startComponents(ctx context.Context, wc *workflowComponents, components *bootstrap.Components) chan error {
	errChan := make(chan error, 3) // Reduced to 3 (coordinator, run consumer, status consumer)

	// Start coordinator
	go func() {
		components.Logger.Info("starting coordinator")
		if err := wc.coordinator.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("coordinator error: %w", err)
		}
	}()

	// HTTP worker now runs as separate service (cmd/http-worker)
	// Start with: make start-http-worker

	// HITL worker now runs as separate service (cmd/hitl-worker)
	// Start with: make start-hitl-worker

	// Start run request consumer
	go func() {
		components.Logger.Info("starting run request consumer")
		if err := wc.runConsumer.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("run request consumer error: %w", err)
		}
	}()

	// Start status update consumer
	go func() {
		components.Logger.Info("starting status update consumer")
		if err := wc.statusConsumer.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("status update consumer error: %w", err)
		}
	}()

	return errChan
}

// waitForShutdown waits for either an error or shutdown signal
func waitForShutdown(ctx context.Context, cancel context.CancelFunc, errChan chan error, components *bootstrap.Components) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		components.Logger.Error("component failed", "error", err)
		os.Exit(1)
	case sig := <-sigChan:
		components.Logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}
}

// createRedisClient creates a Redis client from config
func createRedisClient(components *bootstrap.Components) (*redis.Client, error) {
	// Get Redis config from environment or use defaults
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	redisDB := 0 // Use database 0

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       redisDB,
	})

	return client, nil
}

// getEnv gets an environment variable or returns a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
