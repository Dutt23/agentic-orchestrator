package clients

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	redisWrapper "github.com/lyzr/orchestrator/common/redis"
	"github.com/redis/go-redis/v9"
)

// RedisCASClient stores CAS blobs in Redis (for workflow execution results)
// This is used by workflow-runner for temporary storage of execution results
// NO CACHING - always queries Redis for fresh data
type RedisCASClient struct {
	redis  *redisWrapper.Client
	logger Logger
}

// NewRedisCASClient creates a new Redis-based CAS client (direct, no mover)
func NewRedisCASClient(redis *redis.Client, logger Logger) *RedisCASClient {
	return &RedisCASClient{
		redis:  redisWrapper.NewClient(redis, logger),
		logger: logger,
	}
}

// Put stores data in Redis and returns the CAS ID (SHA256 hash)
func (c *RedisCASClient) Put(ctx context.Context, data []byte, contentType string) (string, error) {
	// Generate SHA256 hash as CAS ID
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	casKey := fmt.Sprintf("cas:%s", hash)

	// Store in Redis with no expiry
	err := c.redis.SetWithExpiry(ctx, casKey, string(data), 0)
	if err != nil {
		c.logger.Error("failed to store in CAS", "cas_id", hash, "error", err)
		return "", fmt.Errorf("failed to store in CAS: %w", err)
	}

	c.logger.Debug("stored in CAS", "cas_id", hash, "size", len(data))
	return hash, nil
}

// Get retrieves data from Redis by CAS ID (no caching, always fresh)
func (c *RedisCASClient) Get(ctx context.Context, casID string) (interface{}, error) {
	casKey := fmt.Sprintf("cas:%s", casID)

	data, err := c.redis.Get(ctx, casKey)
	if err != nil {
		c.logger.Warn("CAS entry not found", "cas_id", casID)
		return nil, fmt.Errorf("CAS entry not found: %s", casID)
	}

	c.logger.Debug("retrieved from CAS", "cas_id", casID, "size", len(data))
	return []byte(data), nil
}

// Store marshals data to JSON and stores it
func (c *RedisCASClient) Store(ctx context.Context, data interface{}) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}
	return c.Put(ctx, jsonData, "application/json")
}
