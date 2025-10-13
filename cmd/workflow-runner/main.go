package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/coordinator"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/supervisor"
	"github.com/lyzr/orchestrator/common/bootstrap"
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

	// Create Redis client
	redisClient, err := createRedisClient(components)
	if err != nil {
		components.Logger.Error("failed to create Redis client", "error", err)
		os.Exit(1)
	}

	// Ping Redis
	if err := redisClient.Ping(ctx).Err(); err != nil {
		components.Logger.Error("failed to ping Redis", "error", err)
		os.Exit(1)
	}
	components.Logger.Info("connected to Redis")

	// Load Lua script for apply_delta
	luaScript, err := os.ReadFile("scripts/apply_delta.lua")
	if err != nil {
		components.Logger.Error("failed to load Lua script", "error", err)
		os.Exit(1)
	}

	// Create CAS client (placeholder for now)
	casClient := &simpleCASClient{logger: components.Logger}

	// Create SDK
	workflowSDK := sdk.NewSDK(redisClient, casClient, components.Logger, string(luaScript))

	// Create coordinator
	coord := coordinator.NewCoordinator(redisClient, workflowSDK, components.Logger)

	// TODO: Create and start supervisors when needed
	// For MVP integration tests, we only need the coordinator
	_ = supervisor.NewCompletionSupervisor // Avoid unused import error
	_ = supervisor.NewTimeoutDetector

	// Start components in goroutines
	errChan := make(chan error, 1)

	// Start coordinator
	go func() {
		components.Logger.Info("starting coordinator")
		if err := coord.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("coordinator error: %w", err)
		}
	}()

	components.Logger.Info("workflow-runner started successfully",
		"components", []string{"coordinator"})

	// Wait for shutdown signal or error
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

	components.Logger.Info("workflow-runner shutting down gracefully")
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

// simpleCASClient is a placeholder CAS client for MVP
type simpleCASClient struct {
	logger sdk.Logger
}

func (m *simpleCASClient) Put(data []byte, contentType string) (string, error) {
	// For MVP, just return a mock CAS ID
	// TODO: Implement actual CAS integration (S3/MinIO)
	casID := fmt.Sprintf("cas://mock/%d", len(data))
	m.logger.Debug("mock CAS Put", "cas_id", casID, "size", len(data))
	return casID, nil
}

func (m *simpleCASClient) Get(casID string) (interface{}, error) {
	// For MVP, return empty data
	// TODO: Implement actual CAS integration
	m.logger.Debug("mock CAS Get", "cas_id", casID)
	return []byte("{}"), nil
}

func (m *simpleCASClient) Store(data interface{}) (string, error) {
	// For MVP, just return a mock CAS ID
	// TODO: Implement actual CAS integration
	casID := fmt.Sprintf("cas://mock/store")
	m.logger.Debug("mock CAS Store", "cas_id", casID)
	return casID, nil
}
