package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/coordinator"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/executor"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/supervisor"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/worker"
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

	// Create Redis-based CAS client for storing execution results
	casClient := &redisCASClient{
		redis:  redisClient,
		logger: components.Logger,
	}

	// Create SDK
	workflowSDK := sdk.NewSDK(redisClient, casClient, components.Logger, string(luaScript))

	// Create coordinator
	coord := coordinator.NewCoordinator(redisClient, workflowSDK, components.Logger)

	// Create HTTP worker
	httpWorker := worker.NewHTTPWorker(redisClient, workflowSDK, components.Logger)

	// Create run request consumer
	orchestratorURL := getEnv("ORCHESTRATOR_URL", "http://localhost:8081")
	runConsumer := executor.NewRunRequestConsumer(redisClient, workflowSDK, components.Logger, orchestratorURL)

	// TODO: Create and start supervisors when needed
	// For MVP integration tests, we only need the coordinator
	_ = supervisor.NewCompletionSupervisor // Avoid unused import error
	_ = supervisor.NewTimeoutDetector

	// Start components in goroutines
	errChan := make(chan error, 3)

	// Start coordinator
	go func() {
		components.Logger.Info("starting coordinator")
		if err := coord.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("coordinator error: %w", err)
		}
	}()

	// Start HTTP worker
	go func() {
		components.Logger.Info("starting HTTP worker")
		if err := httpWorker.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("HTTP worker error: %w", err)
		}
	}()

	// Start run request consumer
	go func() {
		components.Logger.Info("starting run request consumer")
		if err := runConsumer.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("run request consumer error: %w", err)
		}
	}()

	components.Logger.Info("workflow-runner started successfully",
		"components", []string{"coordinator", "http_worker", "run_request_consumer"})

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

// redisCASClient stores CAS blobs in Redis (for workflow execution results)
type redisCASClient struct {
	redis  *redis.Client
	logger sdk.Logger
}

func (c *redisCASClient) Put(data []byte, contentType string) (string, error) {
	// Generate SHA256 hash as CAS ID
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	casKey := fmt.Sprintf("cas:%s", hash)

	// Store in Redis with 24 hour TTL (adjust based on needs)
	err := c.redis.Set(context.Background(), casKey, data, 24*time.Hour).Err()
	if err != nil {
		c.logger.Error("failed to store in CAS", "cas_id", hash, "error", err)
		return "", fmt.Errorf("failed to store in CAS: %w", err)
	}

	c.logger.Debug("stored in CAS", "cas_id", hash, "size", len(data))
	return hash, nil
}

func (c *redisCASClient) Get(casID string) (interface{}, error) {
	casKey := fmt.Sprintf("cas:%s", casID)

	data, err := c.redis.Get(context.Background(), casKey).Bytes()
	if err == redis.Nil {
		c.logger.Warn("CAS entry not found", "cas_id", casID)
		return nil, fmt.Errorf("CAS entry not found: %s", casID)
	}
	if err != nil {
		c.logger.Error("failed to get from CAS", "cas_id", casID, "error", err)
		return nil, fmt.Errorf("failed to get from CAS: %w", err)
	}

	c.logger.Debug("retrieved from CAS", "cas_id", casID, "size", len(data))
	return data, nil
}

func (c *redisCASClient) Store(data interface{}) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}
	return c.Put(jsonData, "application/json")
}
