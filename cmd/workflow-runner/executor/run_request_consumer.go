package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/compiler"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/redis/go-redis/v9"
)

// RunRequestConsumer listens to wf.run.requests stream and starts workflow execution
type RunRequestConsumer struct {
	redis           *redis.Client
	sdk             *sdk.SDK
	logger          sdk.Logger
	stream          string
	consumerGroup   string
	consumerName    string
	orchestratorURL string
}

// RunRequest represents a workflow execution request
type RunRequest struct {
	RunID     string                 `json:"run_id"`
	Tag       string                 `json:"tag"`
	Username  string                 `json:"username"`
	Inputs    map[string]interface{} `json:"inputs"`
	CreatedAt int64                  `json:"created_at"`
}

// NewRunRequestConsumer creates a new run request consumer
func NewRunRequestConsumer(redisClient *redis.Client, workflowSDK *sdk.SDK, logger sdk.Logger, orchestratorURL string) *RunRequestConsumer {
	return &RunRequestConsumer{
		redis:           redisClient,
		sdk:             workflowSDK,
		logger:          logger,
		stream:          "wf.run.requests",
		consumerGroup:   "run_executors",
		consumerName:    fmt.Sprintf("executor_%s", uuid.New().String()[:8]),
		orchestratorURL: orchestratorURL,
	}
}

// Start begins processing run requests
func (c *RunRequestConsumer) Start(ctx context.Context) error {
	c.logger.Info("starting run request consumer",
		"stream", c.stream,
		"consumer_group", c.consumerGroup,
		"consumer_name", c.consumerName)

	// Create consumer group if it doesn't exist
	err := c.redis.XGroupCreateMkStream(ctx, c.stream, c.consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Process messages in a loop
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("run request consumer stopping")
			return nil
		default:
			if err := c.processNextMessage(ctx); err != nil {
				c.logger.Error("failed to process message", "error", err)
				time.Sleep(1 * time.Second) // Back off on error
			}
		}
	}
}

// processNextMessage reads and processes one message from the stream
func (c *RunRequestConsumer) processNextMessage(ctx context.Context) error {
	// Read message from stream (XREADGROUP)
	streams, err := c.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.consumerGroup,
		Consumer: c.consumerName,
		Streams:  []string{c.stream, ">"},
		Count:    1,
		Block:    5 * time.Second,
	}).Result()

	if err == redis.Nil {
		// No messages, continue
		return nil
	}
	if err != nil {
		return fmt.Errorf("XREADGROUP error: %w", err)
	}

	// Process each message
	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := c.handleMessage(ctx, message); err != nil {
				c.logger.Error("failed to handle message", "message_id", message.ID, "error", err)
				// Continue to next message even if this one fails
			}

			// Acknowledge message
			if err := c.redis.XAck(ctx, c.stream, c.consumerGroup, message.ID).Err(); err != nil {
				c.logger.Error("failed to ACK message", "message_id", message.ID, "error", err)
			}
		}
	}

	return nil
}

// handleMessage processes a single run request message
func (c *RunRequestConsumer) handleMessage(ctx context.Context, message redis.XMessage) error {
	// Parse request from message
	requestJSON, ok := message.Values["request"].(string)
	if !ok {
		return fmt.Errorf("message missing request field")
	}

	var runRequest RunRequest
	if err := json.Unmarshal([]byte(requestJSON), &runRequest); err != nil {
		return fmt.Errorf("failed to unmarshal run request: %w", err)
	}

	c.logger.Info("processing run request",
		"run_id", runRequest.RunID,
		"tag", runRequest.Tag)

	// Check idempotency: ensure this run hasn't started already
	idempotencyKey := fmt.Sprintf("run:started:%s", runRequest.RunID)
	wasSet, err := c.redis.SetNX(ctx, idempotencyKey, "1", 24*time.Hour).Result()
	if err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	}

	if !wasSet {
		c.logger.Info("run already started, skipping", "run_id", runRequest.RunID)
		return nil
	}

	// Fetch workflow from orchestrator
	workflow, err := c.fetchWorkflow(ctx, runRequest.Tag, runRequest.Username)
	if err != nil {
		return fmt.Errorf("failed to fetch workflow: %w", err)
	}

	c.logger.Info("wf", workflow)
	// Compile workflow to IR
	ir, err := compiler.CompileWorkflowSchema(workflow, c.sdk.CASClient)
	if err != nil {
		return fmt.Errorf("failed to compile workflow: %w", err)
	}
	c.logger.Info("compiled", ir)
	// Store username in IR metadata for event publishing
	if ir.Metadata == nil {
		ir.Metadata = make(map[string]interface{})
	}
	ir.Metadata["username"] = runRequest.Username
	ir.Metadata["tag"] = runRequest.Tag

	c.logger.Info("compiled workflow to IR",
		"run_id", runRequest.RunID,
		"nodes", len(ir.Nodes))

	// Store IR in Redis
	irJSON, err := json.Marshal(ir)
	if err != nil {
		return fmt.Errorf("failed to marshal IR: %w", err)
	}

	irKey := fmt.Sprintf("ir:%s", runRequest.RunID)
	if err := c.redis.Set(ctx, irKey, irJSON, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store IR: %w", err)
	}

	// Find entry nodes (nodes with no dependencies)
	entryNodes := c.findEntryNodes(ir)
	if len(entryNodes) == 0 {
		return fmt.Errorf("workflow has no entry nodes")
	}

	// Initialize counter
	if err := c.sdk.InitializeCounter(ctx, runRequest.RunID, len(entryNodes)); err != nil {
		return fmt.Errorf("failed to initialize counter: %w", err)
	}

	// Emit initial tokens for entry nodes
	for _, nodeID := range entryNodes {
		node := ir.Nodes[nodeID]

		// Build metadata with task field from node config
		metadata := make(map[string]interface{})

		// Get node config (inline or from CAS)
		var nodeConfig map[string]interface{}
		if len(node.Config) > 0 {
			nodeConfig = node.Config
		} else if node.ConfigRef != "" {
			// Load from CAS if needed
			configData, err := c.sdk.LoadConfig(ctx, node.ConfigRef)
			if err != nil {
				c.logger.Error("failed to load config from CAS for initial token",
					"node_id", nodeID,
					"config_ref", node.ConfigRef,
					"error", err)
			} else if configMap, ok := configData.(map[string]interface{}); ok {
				nodeConfig = configMap
			}
		}

		// Extract task from node config (support both "task" and "prompt")
		if nodeConfig != nil {
			if task, ok := nodeConfig["task"]; ok {
				metadata["task"] = task
			} else if prompt, ok := nodeConfig["prompt"]; ok {
				metadata["task"] = prompt
			}
		}

		// Merge with run inputs
		for k, v := range runRequest.Inputs {
			metadata[k] = v
		}

		c.logger.Info("emitting initial token",
			"run_id", runRequest.RunID,
			"node_id", nodeID,
			"has_task", metadata["task"] != nil,
			"metadata", metadata)

		token := sdk.Token{
			ID:       uuid.New().String()[:12],
			RunID:    runRequest.RunID,
			FromNode: "",
			ToNode:   nodeID,
			Metadata: metadata,
		}

		tokenJSON, err := json.Marshal(token)
		if err != nil {
			c.logger.Error("failed to marshal token", "node", nodeID, "error", err)
			continue
		}

		// Route to appropriate stream based on node type
		stream := c.getStreamForNodeType(node.Type)
		err = c.redis.XAdd(ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"token": string(tokenJSON),
			},
		}).Err()

		if err != nil {
			c.logger.Error("failed to emit token", "node", nodeID, "stream", stream, "error", err)
			return fmt.Errorf("failed to emit initial token: %w", err)
		}

		c.logger.Info("emitted initial token",
			"run_id", runRequest.RunID,
			"node_id", nodeID,
			"stream", stream)
	}

	c.logger.Info("run started successfully",
		"run_id", runRequest.RunID,
		"nodes", len(ir.Nodes),
		"entry_nodes", len(entryNodes))

	// Publish workflow_started event
	c.publishWorkflowEvent(ctx, runRequest.Username, map[string]interface{}{
		"type":        "workflow_started",
		"run_id":      runRequest.RunID,
		"tag":         runRequest.Tag,
		"nodes":       len(ir.Nodes),
		"entry_nodes": len(entryNodes),
		"timestamp":   time.Now().Unix(),
	})

	return nil
}

// fetchWorkflow fetches workflow from orchestrator API
func (c *RunRequestConsumer) fetchWorkflow(ctx context.Context, tag, username string) (*compiler.WorkflowSchema, error) {
	// Call orchestrator API: GET /api/v1/workflows/:tag?materialize=true
	url := fmt.Sprintf("%s/api/v1/workflows/%s?materialize=true", c.orchestratorURL, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user from run request
	req.Header.Set("X-User-ID", username)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workflow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("orchestrator returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		Workflow map[string]interface{} `json:"workflow"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Workflow == nil {
		return nil, fmt.Errorf("workflow is null (materialize=true required)")
	}

	// Convert to WorkflowSchema
	schemaJSON, err := json.Marshal(response.Workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal workflow: %w", err)
	}

	var schema compiler.WorkflowSchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow schema: %w", err)
	}

	return &schema, nil
}

// findEntryNodes finds nodes with no dependencies
func (c *RunRequestConsumer) findEntryNodes(ir *sdk.IR) []string {
	hasIncoming := make(map[string]bool)
	for _, node := range ir.Nodes {
		for _, dep := range node.Dependents {
			hasIncoming[dep] = true
		}
	}

	entryNodes := []string{}
	for nodeID := range ir.Nodes {
		if !hasIncoming[nodeID] {
			entryNodes = append(entryNodes, nodeID)
		}
	}

	return entryNodes
}

// getStreamForNodeType returns the appropriate stream for a node type
func (c *RunRequestConsumer) getStreamForNodeType(nodeType string) string {
	switch nodeType {
	case "agent":
		return "wf.tasks.agent"
	case "http":
		return "wf.tasks.http"
	case "function":
		return "wf.tasks.function"
	default:
		return "wf.tasks.function"
	}
}

// publishWorkflowEvent publishes an event to Redis PubSub for fanout service
func (c *RunRequestConsumer) publishWorkflowEvent(ctx context.Context, username string, event map[string]interface{}) {
	channel := fmt.Sprintf("workflow:events:%s", username)

	eventJSON, err := json.Marshal(event)
	if err != nil {
		c.logger.Error("failed to marshal workflow event", "error", err)
		return
	}

	err = c.redis.Publish(ctx, channel, eventJSON).Err()
	if err != nil {
		c.logger.Error("failed to publish workflow event",
			"channel", channel,
			"error", err)
		return
	}

	c.logger.Debug("published workflow event",
		"channel", channel,
		"type", event["type"])
}
