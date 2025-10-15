package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lyzr/orchestrator/cmd/hitl-worker/worker"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bootstrap service components
	components, err := bootstrap.Setup(ctx, "hitl-worker")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup service: %v\n", err)
		os.Exit(1)
	}
	defer components.Shutdown(ctx)

	components.Logger.Info("hitl-worker starting")

	// Create Redis client
	redisClient, err := createRedisClient()
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

	// Create CAS client
	casClient := clients.NewRedisCASClient(redisClient, components.Logger)

	// Create SDK
	workflowSDK := sdk.NewSDK(redisClient, casClient, components.Logger, string(luaScript))

	// Create HITL worker
	hitlWorker := worker.NewHITLWorker(redisClient, workflowSDK, components.Logger)

	// Start worker in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := hitlWorker.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("hitl worker error: %w", err)
		}
	}()

	components.Logger.Info("hitl-worker started successfully")

	// Wait for shutdown signal or error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		components.Logger.Error("worker failed", "error", err)
		os.Exit(1)
	case sig := <-sigChan:
		components.Logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}

	components.Logger.Info("hitl-worker shutting down gracefully")
}

// createRedisClient creates a Redis client from environment variables
func createRedisClient() (*redis.Client, error) {
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	redisDB := 0

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
