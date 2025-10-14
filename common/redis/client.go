package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// Client wraps redis.Client with common operations and instrumentation
type Client struct {
	redis  *redis.Client
	logger Logger
}

// NewClient creates a new Redis client wrapper
func NewClient(redisClient *redis.Client, logger Logger) *Client {
	return &Client{
		redis:  redisClient,
		logger: logger,
	}
}

// GetUnderlying returns the underlying redis.Client for advanced operations
func (c *Client) GetUnderlying() *redis.Client {
	return c.redis
}

// SetWithExpiry sets a key with expiration
func (c *Client) SetWithExpiry(ctx context.Context, key, value string, expiry time.Duration) error {
	err := c.redis.Set(ctx, key, value, expiry).Err()
	if err != nil {
		c.logger.Error("redis SET failed", "key", key, "error", err)
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	c.logger.Debug("redis SET", "key", key, "expiry", expiry)
	return nil
}

// Get retrieves a value by key
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	val, err := c.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		c.logger.Debug("redis GET key not found", "key", key)
		return "", fmt.Errorf("key not found: %s", key)
	}
	if err != nil {
		c.logger.Error("redis GET failed", "key", key, "error", err)
		return "", fmt.Errorf("failed to get key %s: %w", key, err)
	}
	c.logger.Debug("redis GET", "key", key)
	return val, nil
}

// SetNX sets a key only if it doesn't exist (for idempotency checks)
func (c *Client) SetNX(ctx context.Context, key, value string, expiry time.Duration) (bool, error) {
	wasSet, err := c.redis.SetNX(ctx, key, value, expiry).Result()
	if err != nil {
		c.logger.Error("redis SETNX failed", "key", key, "error", err)
		return false, fmt.Errorf("failed to setnx key %s: %w", key, err)
	}
	c.logger.Debug("redis SETNX", "key", key, "was_set", wasSet)
	return wasSet, nil
}

// Delete removes a key
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	err := c.redis.Del(ctx, keys...).Err()
	if err != nil {
		c.logger.Error("redis DEL failed", "keys", keys, "error", err)
		return fmt.Errorf("failed to delete keys: %w", err)
	}
	c.logger.Debug("redis DEL", "keys", keys)
	return nil
}

// AddToStream adds a message to a Redis stream
func (c *Client) AddToStream(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	id, err := c.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}).Result()
	if err != nil {
		c.logger.Error("redis XADD failed", "stream", stream, "error", err)
		return "", fmt.Errorf("failed to add to stream %s: %w", stream, err)
	}
	c.logger.Debug("redis XADD", "stream", stream, "id", id)
	return id, nil
}

// PublishEvent publishes an event to a Redis channel
func (c *Client) PublishEvent(ctx context.Context, channel string, message string) error {
	err := c.redis.Publish(ctx, channel, message).Err()
	if err != nil {
		c.logger.Error("redis PUBLISH failed", "channel", channel, "error", err)
		return fmt.Errorf("failed to publish to channel %s: %w", channel, err)
	}
	c.logger.Debug("redis PUBLISH", "channel", channel)
	return nil
}

// IncrementHash increments a hash field and returns the new value
func (c *Client) IncrementHash(ctx context.Context, key, field string, increment int64) (int64, error) {
	val, err := c.redis.HIncrBy(ctx, key, field, increment).Result()
	if err != nil {
		c.logger.Error("redis HINCRBY failed", "key", key, "field", field, "error", err)
		return 0, fmt.Errorf("failed to increment hash %s field %s: %w", key, field, err)
	}
	c.logger.Debug("redis HINCRBY", "key", key, "field", field, "value", val)
	return val, nil
}

// GetHash retrieves a hash field value
func (c *Client) GetHash(ctx context.Context, key, field string) (string, error) {
	val, err := c.redis.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		c.logger.Debug("redis HGET field not found", "key", key, "field", field)
		return "", fmt.Errorf("field not found: %s.%s", key, field)
	}
	if err != nil {
		c.logger.Error("redis HGET failed", "key", key, "field", field, "error", err)
		return "", fmt.Errorf("failed to get hash %s field %s: %w", key, field, err)
	}
	c.logger.Debug("redis HGET", "key", key, "field", field)
	return val, nil
}

// SetHash sets a hash field value
func (c *Client) SetHash(ctx context.Context, key, field, value string) error {
	err := c.redis.HSet(ctx, key, field, value).Err()
	if err != nil {
		c.logger.Error("redis HSET failed", "key", key, "field", field, "error", err)
		return fmt.Errorf("failed to set hash %s field %s: %w", key, field, err)
	}
	c.logger.Debug("redis HSET", "key", key, "field", field)
	return nil
}

// PushToList pushes values to the right of a list
func (c *Client) PushToList(ctx context.Context, key string, values ...interface{}) error {
	err := c.redis.RPush(ctx, key, values...).Err()
	if err != nil {
		c.logger.Error("redis RPUSH failed", "key", key, "error", err)
		return fmt.Errorf("failed to rpush to %s: %w", key, err)
	}
	c.logger.Debug("redis RPUSH", "key", key, "count", len(values))
	return nil
}

// BlockingPopList blocks and pops from a list (left side)
func (c *Client) BlockingPopList(ctx context.Context, timeout time.Duration, keys ...string) ([]string, error) {
	result, err := c.redis.BLPop(ctx, timeout, keys...).Result()
	if err == redis.Nil {
		// Timeout - not an error
		return nil, nil
	}
	if err != nil {
		c.logger.Error("redis BLPOP failed", "keys", keys, "error", err)
		return nil, fmt.Errorf("failed to blpop from %v: %w", keys, err)
	}
	c.logger.Debug("redis BLPOP", "keys", keys)
	return result, nil
}

// Pipeline batches multiple Redis operations for better performance
type Pipeline struct {
	pipe   redis.Pipeliner
	client *Client
}

// NewPipeline creates a new pipeline for batching operations
func (c *Client) NewPipeline() *Pipeline {
	return &Pipeline{
		pipe:   c.redis.Pipeline(),
		client: c,
	}
}

// SetWithExpiry queues a SET operation in the pipeline
func (p *Pipeline) SetWithExpiry(ctx context.Context, key, value string, expiry time.Duration) {
	p.pipe.Set(ctx, key, value, expiry)
}

// AddToStream queues an XADD operation in the pipeline
func (p *Pipeline) AddToStream(ctx context.Context, stream string, values map[string]interface{}) {
	p.pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: values,
	})
}

// PublishEvent queues a PUBLISH operation in the pipeline
func (p *Pipeline) PublishEvent(ctx context.Context, channel string, message string) {
	p.pipe.Publish(ctx, channel, message)
}

// Exec executes all queued operations in the pipeline
func (p *Pipeline) Exec(ctx context.Context) error {
	_, err := p.pipe.Exec(ctx)
	if err != nil {
		p.client.logger.Error("redis pipeline exec failed", "error", err)
		return fmt.Errorf("failed to execute pipeline: %w", err)
	}
	p.client.logger.Debug("redis pipeline executed successfully")
	return nil
}
