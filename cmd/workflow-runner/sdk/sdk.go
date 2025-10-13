package sdk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// SDK provides core workflow execution capabilities
type SDK struct {
	redis     *redis.Client
	CASClient CASClient
	logger    Logger
	script    *redis.Script
}

// Logger interface for SDK logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// NewSDK creates a new SDK instance
func NewSDK(redisClient *redis.Client, casClient CASClient, logger Logger, luaScript string) *SDK {
	return &SDK{
		redis:     redisClient,
		CASClient: casClient,
		logger:    logger,
		script:    redis.NewScript(luaScript),
	}
}

// ApplyDelta applies a counter operation (idempotent)
// Returns (counter_value, hit_zero, error)
func (s *SDK) ApplyDelta(ctx context.Context, runID string, opKey string, delta int) (*ApplyDeltaResult, error) {
	appliedSet := fmt.Sprintf("applied:%s", runID)
	counterKey := fmt.Sprintf("counter:%s", runID)

	keys := []string{appliedSet, counterKey, runID}
	args := []interface{}{opKey, delta}

	result, err := s.script.Run(ctx, s.redis, keys, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("apply delta failed: %w", err)
	}

	// Parse result: [new_value, changed, hit_zero]
	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) != 3 {
		return nil, fmt.Errorf("unexpected result format from Lua script")
	}

	counterValue, ok := resultSlice[0].(int64)
	if !ok {
		return nil, fmt.Errorf("invalid counter value type")
	}

	changed, ok := resultSlice[1].(int64)
	if !ok {
		return nil, fmt.Errorf("invalid changed flag type")
	}

	hitZero, ok := resultSlice[2].(int64)
	if !ok {
		return nil, fmt.Errorf("invalid hit_zero flag type")
	}

	return &ApplyDeltaResult{
		CounterValue: int(counterValue),
		Changed:      changed == 1,
		HitZero:      hitZero == 1,
	}, nil
}

// Consume applies -1 to counter (token consumption)
func (s *SDK) Consume(ctx context.Context, runID, nodeID string) error {
	opKey := fmt.Sprintf("consume:%s:%s", runID, nodeID)

	result, err := s.ApplyDelta(ctx, runID, opKey, -1)
	if err != nil {
		return err
	}

	if result.Changed {
		s.logger.Info("token consumed",
			"run_id", runID,
			"node_id", nodeID,
			"counter", result.CounterValue)
	} else {
		s.logger.Info("token already consumed (idempotent)",
			"run_id", runID,
			"node_id", nodeID)
	}

	return nil
}

// Emit applies +N to counter (don't publish tokens - coordinator does that)
func (s *SDK) Emit(ctx context.Context, runID, fromNode string, toNodes []string, payloadRef string) error {
	if len(toNodes) == 0 {
		s.logger.Info("no next nodes to emit to", "run_id", runID, "from", fromNode)
		return nil
	}

	// Apply counter +N
	emitID := uuid.New().String()
	opKey := fmt.Sprintf("emit:%s:%s:%s", runID, fromNode, emitID)

	result, err := s.ApplyDelta(ctx, runID, opKey, len(toNodes))
	if err != nil {
		return err
	}

	if !result.Changed {
		s.logger.Warn("emit already applied (idempotent)",
			"run_id", runID,
			"from", fromNode,
			"emit_id", emitID)
		return nil
	}

	s.logger.Info("counter increased",
		"run_id", runID,
		"from", fromNode,
		"delta", len(toNodes),
		"counter", result.CounterValue)

	return nil
}

// StoreContext stores node output in Redis for cross-node access
func (s *SDK) StoreContext(ctx context.Context, runID, nodeID, outputRef string) error {
	contextKey := fmt.Sprintf("context:%s", runID)

	err := s.redis.HSet(ctx, contextKey, nodeID+":output", outputRef).Err()
	if err != nil {
		return fmt.Errorf("failed to store context: %w", err)
	}

	s.logger.Info("context stored",
		"run_id", runID,
		"node", nodeID,
		"output_ref", outputRef)

	return nil
}

// LoadContext loads all previous node outputs
func (s *SDK) LoadContext(ctx context.Context, runID string) (map[string]interface{}, error) {
	contextKey := fmt.Sprintf("context:%s", runID)

	outputs, err := s.redis.HGetAll(ctx, contextKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to load context: %w", err)
	}

	context := make(map[string]interface{})

	for key, outputRef := range outputs {
		// Load actual output from CAS
		output, err := s.CASClient.Get(outputRef)
		if err != nil {
			s.logger.Warn("failed to load output from CAS",
				"key", key,
				"ref", outputRef,
				"error", err)
			continue
		}

		context[key] = output
	}

	s.logger.Info("context loaded",
		"run_id", runID,
		"keys", len(context))

	return context, nil
}

// LoadNodeOutput loads a specific node's output from context
func (s *SDK) LoadNodeOutput(ctx context.Context, runID, nodeID string) (interface{}, error) {
	contextKey := fmt.Sprintf("context:%s", runID)
	outputKey := fmt.Sprintf("%s:output", nodeID)

	// Get CAS reference for this node's output
	casRef, err := s.redis.HGet(ctx, contextKey, outputKey).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("node output not found: %s", nodeID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node output reference: %w", err)
	}

	// Load from CAS
	data, err := s.CASClient.Get(casRef)
	if err != nil {
		return nil, fmt.Errorf("failed to load node output from CAS: %w", err)
	}

	// If it's JSON bytes, unmarshal it
	if bytes, ok := data.([]byte); ok {
		var result interface{}
		if err := json.Unmarshal(bytes, &result); err != nil {
			// If unmarshal fails, return raw bytes
			return bytes, nil
		}
		return result, nil
	}

	return data, nil
}

// LoadConfig loads node configuration from CAS
func (s *SDK) LoadConfig(ctx context.Context, configRef string) (interface{}, error) {
	return s.CASClient.Get(configRef)
}

// LoadPayload loads payload from CAS
func (s *SDK) LoadPayload(ctx context.Context, payloadRef string) (interface{}, error) {
	data, err := s.CASClient.Get(payloadRef)
	if err != nil {
		return nil, err
	}

	// Unmarshal if it's JSON bytes
	if bytes, ok := data.([]byte); ok {
		var result interface{}
		if err := json.Unmarshal(bytes, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		return result, nil
	}

	return data, nil
}

// StoreOutput stores output in CAS and returns reference
func (s *SDK) StoreOutput(ctx context.Context, output interface{}) (string, error) {
	data, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to marshal output: %w", err)
	}
	return s.CASClient.Put(data, "application/json")
}

// GetCounter returns the current counter value
func (s *SDK) GetCounter(ctx context.Context, runID string) (int, error) {
	counterKey := fmt.Sprintf("counter:%s", runID)

	val, err := s.redis.Get(ctx, counterKey).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get counter: %w", err)
	}

	return val, nil
}

// InitializeCounter initializes the counter for a new run
func (s *SDK) InitializeCounter(ctx context.Context, runID string, initialValue int) error {
	counterKey := fmt.Sprintf("counter:%s", runID)

	err := s.redis.Set(ctx, counterKey, initialValue, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to initialize counter: %w", err)
	}

	s.logger.Info("counter initialized",
		"run_id", runID,
		"value", initialValue)

	return nil
}
