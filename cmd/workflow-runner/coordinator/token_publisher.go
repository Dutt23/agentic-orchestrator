package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
)

// publishToken publishes a token to a Redis stream with resolved config
func (c *Coordinator) publishToken(ctx context.Context, stream, runID, fromNode, toNode, payloadRef string, resolvedConfig map[string]interface{}, ir *sdk.IR) error {
	// Generate unique job ID for this token
	jobID := fmt.Sprintf("%s-%s-%d", runID, toNode, time.Now().UnixNano())

	// Debug log the resolvedConfig
	c.logger.Info("publishToken called",
		"run_id", runID,
		"to_node", toNode,
		"resolvedConfig_nil", resolvedConfig == nil,
		"resolvedConfig", resolvedConfig)

	sentAt := time.Now().UTC()
	token := map[string]interface{}{
		"id":          jobID, // Add job ID for agent-runner-py
		"run_id":      runID,
		"from_node":   fromNode,
		"to_node":     toNode,
		"payload_ref": payloadRef,
		"created_at":  sentAt.Format(time.RFC3339),
		"sent_at":     sentAt.Format(time.RFC3339Nano), // High precision timestamp for metrics
	}

	// Include resolved config if available
	if resolvedConfig != nil {
		token["config"] = resolvedConfig
		c.logger.Info("added config to token", "config", resolvedConfig)
	} else {
		c.logger.Warn("resolvedConfig is nil, skipping config and metadata")
	}

	// Extract task from config and add to metadata for agent-runner-py
	// Agent runner expects metadata.task
	// Support both "task" (new) and "prompt" (old) for backward compatibility
	metadata := make(map[string]interface{})
	if resolvedConfig != nil {
		// Try "task" first (new field name)
		if task, ok := resolvedConfig["task"]; ok {
			metadata["task"] = task
		} else if prompt, ok := resolvedConfig["prompt"]; ok {
			// Fall back to "prompt" for backward compatibility
			metadata["task"] = prompt
		}
		// Also pass the entire workflow context if available
		if workflow, ok := resolvedConfig["workflow"]; ok {
			metadata["workflow"] = workflow
		}
	}

	// Add workflow_owner from IR metadata (required for patch_workflow tool)
	if ir.Metadata != nil {
		if username, ok := ir.Metadata["username"].(string); ok {
			token["workflow_owner"] = username
			c.logger.Info("added workflow_owner to metadata", "workflow_owner", username)
		}
		// Also add tag if available
		if tag, ok := ir.Metadata["tag"].(string); ok {
			metadata["workflow_tag"] = tag
			c.logger.Info("added workflow_tag to metadata", "workflow_tag", tag)
		}
	}

	if len(metadata) > 0 {
		token["metadata"] = metadata
		c.logger.Info("added metadata to token",
			"metadata", metadata,
			"task_value", metadata["task"],
			"workflow_owner", metadata["workflow_owner"])
	} else {
		c.logger.Warn("metadata is empty, not adding to token",
			"resolvedConfig_nil", resolvedConfig == nil)
	}

	tokenJSON, err := json.Marshal(token)
	c.logger.Info("marshaled token", "token_json", string(tokenJSON))
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.redisWrapper.AddToStream(ctx, stream, map[string]interface{}{
		"token":   string(tokenJSON),
		"run_id":  runID,
		"to_node": toNode,
	})

	if err != nil {
		return fmt.Errorf("failed to add to stream: %w", err)
	}

	c.logger.Debug("published token with job_id",
		"run_id", runID,
		"job_id", jobID,
		"to_node", toNode,
		"has_task", metadata["task"] != nil)

	return nil
}

// loadAndResolveConfig loads node config (inline or from CAS) and resolves variables
// Returns the resolved config map, or nil if config loading/resolution fails
func (c *Coordinator) loadAndResolveConfig(ctx context.Context, runID, nodeID string, node *sdk.Node) map[string]interface{} {
	// Load node config (inline or CAS)
	c.logger.Info("loading node config",
		"run_id", runID,
		"node_id", nodeID,
		"node_type", node.Type,
		"has_inline_config", len(node.Config) > 0,
		"has_config_ref", node.ConfigRef != "",
		"inline_config", node.Config,
		"config_ref", node.ConfigRef)

	var config map[string]interface{}
	c.logger.Info("config is ehre",
		"config", node.Config)
	if len(node.Config) > 0 {
		config = node.Config
		c.logger.Info("using inline config", "config", config)
	} else if node.ConfigRef != "" {
		c.logger.Info("loading config from CAS", "config_ref", node.ConfigRef)
		configData, err := c.sdk.LoadConfig(ctx, node.ConfigRef)
		if err != nil {
			c.logger.Error("failed to load config from CAS",
				"run_id", runID,
				"node_id", nodeID,
				"config_ref", node.ConfigRef,
				"error", err)
			return nil
		}
		// Convert to map
		if configMap, ok := configData.(map[string]interface{}); ok {
			config = configMap
			c.logger.Info("loaded config from CAS", "config", config)
		} else {
			c.logger.Error("config is not a map",
				"run_id", runID,
				"node_id", nodeID)
			return nil
		}
	} else {
		c.logger.Warn("node has no config (neither inline nor CAS ref)",
			"run_id", runID,
			"node_id", nodeID)
	}

	// Resolve variables in config (e.g., $nodes.node_id)
	c.logger.Info("about to resolve config",
		"run_id", runID,
		"node_id", nodeID,
		"config_is_nil", config == nil,
		"config", config)

	var resolvedConfig map[string]interface{}
	if config != nil {
		var err error
		resolvedConfig, err = c.resolver.ResolveConfig(ctx, runID, config)
		if err != nil {
			c.logger.Error("failed to resolve config variables",
				"run_id", runID,
				"node_id", nodeID,
				"error", err)
			// Continue with unresolved config as fallback
			resolvedConfig = config
		} else {
			c.logger.Info("resolved config variables successfully",
				"run_id", runID,
				"node_id", nodeID,
				"resolvedConfig", resolvedConfig)
		}
	} else {
		c.logger.Warn("config is nil, cannot resolve - resolvedConfig will be nil",
			"run_id", runID,
			"node_id", nodeID)
	}

	return resolvedConfig
}
