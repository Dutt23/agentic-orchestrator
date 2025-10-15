package ratelimit

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/redis/go-redis/v9"
)

//go:embed rate_limit.lua
var rateLimitScript string

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	Allowed           bool  // Whether the request is allowed
	CurrentCount      int64 // Current count in the window
	Limit             int64 // The limit that was checked
	RetryAfterSeconds int64 // Seconds until the limit resets (0 if allowed)
}

// RateLimiter provides workflow-aware rate limiting using Redis + Lua
type RateLimiter struct {
	redis  *redis.Client
	script *redis.Script
	logger Logger
}

// NewRateLimiter creates a new rate limiter with embedded Lua script
func NewRateLimiter(redisClient *redis.Client, logger Logger) *RateLimiter {
	return &RateLimiter{
		redis:  redisClient,
		script: redis.NewScript(rateLimitScript),
		logger: logger,
	}
}

// CheckGlobalLimit checks the global service-wide rate limit
func (r *RateLimiter) CheckGlobalLimit(ctx context.Context, limit int64) (*RateLimitResult, error) {
	key := "rate_limit:global"
	return r.checkLimit(ctx, key, limit, 60) // 1 minute window
}

// CheckUserLimit checks rate limit for a specific user
func (r *RateLimiter) CheckUserLimit(ctx context.Context, username string, limit int64, windowSec int) (*RateLimitResult, error) {
	key := fmt.Sprintf("rate_limit:user:%s", username)
	return r.checkLimit(ctx, key, limit, windowSec)
}

// CheckWorkflowLimit checks rate limit for a specific workflow
func (r *RateLimiter) CheckWorkflowLimit(ctx context.Context, username, workflowTag string, limit int64, windowSec int) (*RateLimitResult, error) {
	key := fmt.Sprintf("rate_limit:workflow:%s:%s", username, workflowTag)
	return r.checkLimit(ctx, key, limit, windowSec)
}

// CheckTieredLimit checks rate limit based on workflow tier
// Uses separate counters for each tier to prevent simple workflows from being blocked by heavy ones
func (r *RateLimiter) CheckTieredLimit(ctx context.Context, username string, tier WorkflowTier) (*RateLimitResult, error) {
	key := fmt.Sprintf("rate_limit:user:%s:tier:%s", username, tier)
	limit := GetLimitForTier(tier)
	return r.checkLimit(ctx, key, limit, 60) // 1 minute window
}

// checkLimit executes the rate limit Lua script
func (r *RateLimiter) checkLimit(ctx context.Context, key string, limit int64, windowSec int) (*RateLimitResult, error) {
	// Run Lua script atomically
	result, err := r.script.Run(ctx, r.redis, []string{key}, limit, windowSec).Result()
	if err != nil {
		r.logger.Error("rate limit check failed", "key", key, "error", err)
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	// Parse result array: {allowed, current_count, limit, retry_after}
	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) != 4 {
		return nil, fmt.Errorf("unexpected script result format")
	}

	allowed := resultArray[0].(int64) == 1
	currentCount := resultArray[1].(int64)
	returnedLimit := resultArray[2].(int64)
	retryAfter := resultArray[3].(int64)

	rateLimitResult := &RateLimitResult{
		Allowed:           allowed,
		CurrentCount:      currentCount,
		Limit:             returnedLimit,
		RetryAfterSeconds: retryAfter,
	}

	if !allowed {
		r.logger.Warn("rate limit exceeded",
			"key", key,
			"current", currentCount,
			"limit", limit,
			"retry_after", retryAfter)
	} else {
		r.logger.Debug("rate limit check passed",
			"key", key,
			"current", currentCount,
			"limit", limit)
	}

	return rateLimitResult, nil
}

// GetCurrentCount returns current count without incrementing (for monitoring)
func (r *RateLimiter) GetCurrentCount(ctx context.Context, key string) (int64, error) {
	count, err := r.redis.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil // Key doesn't exist = no requests yet
	}
	return count, err
}

// ResetLimit clears a rate limit counter (for testing/admin)
func (r *RateLimiter) ResetLimit(ctx context.Context, key string) error {
	return r.redis.Del(ctx, key).Err()
}
