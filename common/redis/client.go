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

// GetMultiple retrieves multiple keys using pipeline (single network round-trip)
// Returns a map of key -> value. Keys that don't exist are omitted from result.
func (c *Client) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return make(map[string]string), nil
	}

	pipe := c.redis.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))

	// Queue all GET commands
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		c.logger.Error("redis pipeline GET failed", "key_count", len(keys), "error", err)
		return nil, fmt.Errorf("failed to get multiple keys: %w", err)
	}

	// Collect results
	result := make(map[string]string)
	for i, cmd := range cmds {
		val, err := cmd.Result()
		if err == redis.Nil {
			// Key doesn't exist, skip it
			continue
		}
		if err != nil {
			c.logger.Warn("redis GET failed for key in pipeline", "key", keys[i], "error", err)
			continue
		}
		result[keys[i]] = val
	}

	c.logger.Debug("redis pipeline GET", "requested", len(keys), "found", len(result))
	return result, nil
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

// GetAllHash retrieves all fields and values of a hash
func (c *Client) GetAllHash(ctx context.Context, key string) (map[string]string, error) {
	val, err := c.redis.HGetAll(ctx, key).Result()
	if err != nil {
		c.logger.Error("redis HGETALL failed", "key", key, "error", err)
		return nil, fmt.Errorf("failed to get all hash fields %s: %w", key, err)
	}
	c.logger.Debug("redis HGETALL", "key", key, "field_count", len(val))
	return val, nil
}

// Set sets a key with optional expiration (0 = no expiration)
func (c *Client) Set(ctx context.Context, key, value string, expiry time.Duration) error {
	err := c.redis.Set(ctx, key, value, expiry).Err()
	if err != nil {
		c.logger.Error("redis SET failed", "key", key, "error", err)
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	if expiry > 0 {
		c.logger.Debug("redis SET", "key", key, "expiry", expiry)
	} else {
		c.logger.Debug("redis SET", "key", key)
	}
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

// Increment increments a counter and returns the new value
func (c *Client) Increment(ctx context.Context, key string) (int64, error) {
	val, err := c.redis.Incr(ctx, key).Result()
	if err != nil {
		c.logger.Error("redis INCR failed", "key", key, "error", err)
		return 0, fmt.Errorf("failed to increment key %s: %w", key, err)
	}
	c.logger.Debug("redis INCR", "key", key, "value", val)
	return val, nil
}

// Decrement decrements a counter and returns the new value
func (c *Client) Decrement(ctx context.Context, key string) (int64, error) {
	val, err := c.redis.Decr(ctx, key).Result()
	if err != nil {
		c.logger.Error("redis DECR failed", "key", key, "error", err)
		return 0, fmt.Errorf("failed to decrement key %s: %w", key, err)
	}
	c.logger.Debug("redis DECR", "key", key, "value", val)
	return val, nil
}

// ReadFromStreamGroup reads messages from a stream using consumer groups
func (c *Client) ReadFromStreamGroup(ctx context.Context, group, consumer, stream string, count int64, block time.Duration) ([]redis.XStream, error) {
	streams, err := c.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
	}).Result()

	if err == redis.Nil {
		// Timeout/no messages - not an error
		return nil, nil
	}
	if err != nil {
		c.logger.Error("redis XREADGROUP failed", "stream", stream, "group", group, "error", err)
		return nil, fmt.Errorf("failed to read from stream %s: %w", stream, err)
	}

	c.logger.Debug("redis XREADGROUP", "stream", stream, "group", group, "message_count", len(streams))
	return streams, nil
}

// AckStreamMessage acknowledges a message in a stream
func (c *Client) AckStreamMessage(ctx context.Context, stream, group, messageID string) error {
	err := c.redis.XAck(ctx, stream, group, messageID).Err()
	if err != nil {
		c.logger.Error("redis XACK failed", "stream", stream, "group", group, "message_id", messageID, "error", err)
		return fmt.Errorf("failed to ack message %s: %w", messageID, err)
	}
	c.logger.Debug("redis XACK", "stream", stream, "group", group, "message_id", messageID)
	return nil
}

// CreateStreamGroup creates a consumer group for a stream
func (c *Client) CreateStreamGroup(ctx context.Context, stream, group string) error {
	err := c.redis.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		c.logger.Error("redis XGROUP CREATE failed", "stream", stream, "group", group, "error", err)
		return fmt.Errorf("failed to create consumer group %s: %w", group, err)
	}
	c.logger.Debug("redis XGROUP CREATE", "stream", stream, "group", group)
	return nil
}

// Transaction represents a Redis transaction for atomic operations
type Transaction struct {
	pipe   redis.Pipeliner
	client *Client
	cmds   map[string]redis.Cmder // Store commands by label for result retrieval
}

// NewTransaction creates a new transaction (TxPipeline)
func (c *Client) NewTransaction() *Transaction {
	return &Transaction{
		pipe:   c.redis.TxPipeline(),
		client: c,
		cmds:   make(map[string]redis.Cmder),
	}
}

// SetNX queues a SETNX operation and returns a label for retrieving the result
func (t *Transaction) SetNX(ctx context.Context, key, value string, expiry time.Duration) string {
	label := fmt.Sprintf("setnx_%s", key)
	cmd := t.pipe.SetNX(ctx, key, value, expiry)
	t.cmds[label] = cmd
	return label
}

// Incr queues an INCR operation and returns a label for retrieving the result
func (t *Transaction) Incr(ctx context.Context, key string) string {
	label := fmt.Sprintf("incr_%s", key)
	cmd := t.pipe.Incr(ctx, key)
	t.cmds[label] = cmd
	return label
}

// Decr queues a DECR operation and returns a label for retrieving the result
func (t *Transaction) Decr(ctx context.Context, key string) string {
	label := fmt.Sprintf("decr_%s", key)
	cmd := t.pipe.Decr(ctx, key)
	t.cmds[label] = cmd
	return label
}

// Exec executes all queued operations atomically
func (t *Transaction) Exec(ctx context.Context) error {
	_, err := t.pipe.Exec(ctx)
	if err != nil {
		t.client.logger.Error("redis transaction exec failed", "error", err)
		return fmt.Errorf("failed to execute transaction: %w", err)
	}
	t.client.logger.Debug("redis transaction executed successfully")
	return nil
}

// GetBoolResult retrieves a boolean result from a labeled command (for SETNX)
func (t *Transaction) GetBoolResult(label string) (bool, error) {
	cmd, exists := t.cmds[label]
	if !exists {
		return false, fmt.Errorf("command with label %s not found", label)
	}

	boolCmd, ok := cmd.(*redis.BoolCmd)
	if !ok {
		return false, fmt.Errorf("command %s is not a BoolCmd", label)
	}

	return boolCmd.Result()
}

// GetIntResult retrieves an integer result from a labeled command (for INCR/DECR)
func (t *Transaction) GetIntResult(label string) (int64, error) {
	cmd, exists := t.cmds[label]
	if !exists {
		return 0, fmt.Errorf("command with label %s not found", label)
	}

	intCmd, ok := cmd.(*redis.IntCmd)
	if !ok {
		return 0, fmt.Errorf("command %s is not an IntCmd", label)
	}

	return intCmd.Result()
}
