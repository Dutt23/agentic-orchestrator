package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/compiler"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/redis/go-redis/v9"
)

// RunHandler handles run-related operations including patching
type RunHandler struct {
	components *bootstrap.Components
	redis      *redis.Client
	casClient  sdk.CASClient
}

// PatchRequest represents a request to patch a workflow
type PatchRequest struct {
	Operations  []PatchOperation `json:"operations"`
	Description string           `json:"description"`
}

// PatchOperation represents a JSON Patch operation
type PatchOperation struct {
	Op    string      `json:"op"`    // add, remove, replace
	Path  string      `json:"path"`  // JSON pointer
	Value interface{} `json:"value"` // New value (for add/replace)
}

// NewRunHandler creates a new run handler
func NewRunHandler(components *bootstrap.Components, redis *redis.Client, casClient sdk.CASClient) *RunHandler {
	return &RunHandler{
		components: components,
		redis:      redis,
		casClient:  casClient,
	}
}

// PatchRun applies JSON Patch operations to a running workflow
func (h *RunHandler) PatchRun(c echo.Context) error {
	runID := c.Param("id")

	var req PatchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	h.components.Logger.Info("received patch request",
		"run_id", runID,
		"operations", len(req.Operations),
		"description", req.Description)

	// 1. Load current IR from Redis
	irKey := fmt.Sprintf("ir:%s", runID)
	irJSON, err := h.redis.Get(c.Request().Context(), irKey).Result()
	if err == redis.Nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}
	if err != nil {
		h.components.Logger.Error("failed to load IR", "run_id", runID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load workflow IR")
	}

	var currentIR sdk.IR
	if err := json.Unmarshal([]byte(irJSON), &currentIR); err != nil {
		h.components.Logger.Error("failed to unmarshal IR", "run_id", runID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to parse workflow IR")
	}

	// 2. Convert IR to workflow schema
	workflowSchema := h.irToWorkflowSchema(&currentIR)

	// 3. Apply JSON Patch operations
	patchedSchema, err := h.applyPatch(workflowSchema, req.Operations)
	if err != nil {
		h.components.Logger.Warn("failed to apply patch",
			"run_id", runID,
			"error", err)
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to apply patch: %v", err))
	}

	// 4. Recompile to IR
	newIR, err := compiler.CompileWorkflowSchema(patchedSchema, h.casClient)
	if err != nil {
		h.components.Logger.Warn("failed to compile patched workflow",
			"run_id", runID,
			"error", err)
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to compile patched workflow: %v", err))
	}

	// 5. Update Redis with new IR
	newIRJSON, err := json.Marshal(newIR)
	if err != nil {
		h.components.Logger.Error("failed to marshal new IR", "run_id", runID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to serialize new IR")
	}

	if err := h.redis.Set(c.Request().Context(), irKey, newIRJSON, 0).Err(); err != nil {
		h.components.Logger.Error("failed to update IR in Redis",
			"run_id", runID,
			"error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update workflow IR")
	}

	// 6. Log event
	h.components.Logger.Info("workflow patched successfully",
		"run_id", runID,
		"old_nodes", len(currentIR.Nodes),
		"new_nodes", len(newIR.Nodes),
		"description", req.Description)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run_id":      runID,
		"patched":     true,
		"old_nodes":   len(currentIR.Nodes),
		"new_nodes":   len(newIR.Nodes),
		"description": req.Description,
	})
}

// irToWorkflowSchema converts IR back to workflow schema format
func (h *RunHandler) irToWorkflowSchema(ir *sdk.IR) *compiler.WorkflowSchema {
	schema := &compiler.WorkflowSchema{
		Nodes: make([]compiler.WorkflowNode, 0, len(ir.Nodes)),
		Edges: []compiler.WorkflowEdge{},
	}

	// Convert nodes
	for _, node := range ir.Nodes {
		wfNode := compiler.WorkflowNode{
			ID:     node.ID,
			Type:   node.Type,
			Config: make(map[string]interface{}),
		}

		// Load config from CAS if available
		if node.ConfigRef != "" {
			configData, err := h.casClient.Get(node.ConfigRef)
			if err == nil {
				if bytes, ok := configData.([]byte); ok {
					json.Unmarshal(bytes, &wfNode.Config)
				}
			}
		}

		// Handle loop config
		if node.Loop != nil && node.Loop.Enabled {
			wfNode.Type = "loop"
			wfNode.Config["max_iterations"] = node.Loop.MaxIterations
			wfNode.Config["loop_back_to"] = node.Loop.LoopBackTo
			if node.Loop.Condition != nil {
				wfNode.Config["condition"] = node.Loop.Condition.Expression
			}
			if len(node.Loop.BreakPath) > 0 {
				wfNode.Config["break_path"] = node.Loop.BreakPath
			}
			if len(node.Loop.TimeoutPath) > 0 {
				wfNode.Config["timeout_path"] = node.Loop.TimeoutPath
			}
		}

		// Handle branch config
		if node.Branch != nil && node.Branch.Enabled {
			wfNode.Type = "conditional"
		}

		schema.Nodes = append(schema.Nodes, wfNode)

		// Convert edges (dependencies → edges)
		for _, dep := range node.Dependents {
			edge := compiler.WorkflowEdge{
				From: node.ID,
				To:   dep,
			}
			schema.Edges = append(schema.Edges, edge)
		}

		// Add branch edges with conditions
		if node.Branch != nil && node.Branch.Enabled {
			for _, rule := range node.Branch.Rules {
				for _, nextNode := range rule.NextNodes {
					edge := compiler.WorkflowEdge{
						From: node.ID,
						To:   nextNode,
					}
					if rule.Condition != nil {
						edge.Condition = rule.Condition.Expression
					}
					schema.Edges = append(schema.Edges, edge)
				}
			}
			// Default edges
			for _, nextNode := range node.Branch.Default {
				edge := compiler.WorkflowEdge{
					From: node.ID,
					To:   nextNode,
				}
				schema.Edges = append(schema.Edges, edge)
			}
		}
	}

	return schema
}

// applyPatch applies JSON Patch operations to the workflow schema
func (h *RunHandler) applyPatch(schema *compiler.WorkflowSchema, operations []PatchOperation) (*compiler.WorkflowSchema, error) {
	// For MVP, we'll handle the most common operation: adding a node

	for _, op := range operations {
		switch op.Op {
		case "add":
			if op.Path == "/nodes/-" {
				// Add node to the end
				nodeMap, ok := op.Value.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid node value")
				}

				node := compiler.WorkflowNode{}
				nodeJSON, err := json.Marshal(nodeMap)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal node: %w", err)
				}
				if err := json.Unmarshal(nodeJSON, &node); err != nil {
					return nil, fmt.Errorf("failed to unmarshal node: %w", err)
				}

				schema.Nodes = append(schema.Nodes, node)

			} else if op.Path == "/edges/-" {
				// Add edge to the end
				edgeMap, ok := op.Value.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid edge value")
				}

				edge := compiler.WorkflowEdge{}
				edgeJSON, err := json.Marshal(edgeMap)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal edge: %w", err)
				}
				if err := json.Unmarshal(edgeJSON, &edge); err != nil {
					return nil, fmt.Errorf("failed to unmarshal edge: %w", err)
				}

				schema.Edges = append(schema.Edges, edge)

			} else {
				return nil, fmt.Errorf("unsupported add path: %s", op.Path)
			}

		case "remove":
			// TODO: Implement remove operation
			return nil, fmt.Errorf("remove operation not yet implemented")

		case "replace":
			// TODO: Implement replace operation
			return nil, fmt.Errorf("replace operation not yet implemented")

		default:
			return nil, fmt.Errorf("unsupported operation: %s", op.Op)
		}
	}

	return schema, nil
}

// ExecuteWorkflow publishes a workflow execution request to Redis stream
func (h *RunHandler) ExecuteWorkflow(c echo.Context) error {
	ctx := c.Request().Context()
	tagName := c.Param("tag")

	// Parse request
	var req struct {
		Inputs map[string]interface{} `json:"inputs"`
	}

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	// Extract username from context
	username, ok := c.Get("username").(string)
	if !ok || username == "" {
		username = "system"
	}

	// Generate idempotent run_id: tag:timestamp_hash
	timestamp := time.Now().UnixNano()
	runID := fmt.Sprintf("%s:%x", tagName, timestamp)

	h.components.Logger.Info("execute workflow request",
		"tag", tagName,
		"run_id", runID,
		"username", username)

	// Publish run request to Redis stream
	runRequest := map[string]interface{}{
		"run_id":     runID,
		"tag":        tagName,
		"username":   username,
		"inputs":     req.Inputs,
		"created_at": time.Now().Unix(),
	}

	requestJSON, err := json.Marshal(runRequest)
	if err != nil {
		h.components.Logger.Error("failed to marshal run request", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create run request")
	}

	// Publish to wf.run.requests stream
	err = h.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: "wf.run.requests",
		Values: map[string]interface{}{
			"request": string(requestJSON),
		},
	}).Err()

	if err != nil {
		h.components.Logger.Error("failed to publish run request", "run_id", runID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to publish run request")
	}

	h.components.Logger.Info("published run request",
		"run_id", runID,
		"tag", tagName)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"run_id": runID,
		"status": "queued",
		"tag":    tagName,
	})
}

// GetRun returns run status and metadata
func (h *RunHandler) GetRun(c echo.Context) error {
	runID := c.Param("id")

	// Query database for run
	query := `
		SELECT run_id, status, submitted_at, started_at, ended_at
		FROM run
		WHERE run_id = $1
	`

	var run struct {
		RunID       string
		Status      string
		SubmittedAt string
		StartedAt   *string
		EndedAt     *string
	}

	err := h.components.DB.QueryRow(context.Background(), query, runID).Scan(
		&run.RunID,
		&run.Status,
		&run.SubmittedAt,
		&run.StartedAt,
		&run.EndedAt,
	)

	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	return c.JSON(http.StatusOK, run)
}
