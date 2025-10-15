package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
	"github.com/lyzr/orchestrator/common/compiler"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
	rediscommon "github.com/lyzr/orchestrator/common/redis"
	"github.com/lyzr/orchestrator/common/sdk"
)

// RunHandler handles run-related operations including patching
type RunHandler struct {
	components *bootstrap.Components
	redis      *rediscommon.Client
	casClient  clients.CASClient
	runService *service.RunService
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
func NewRunHandler(components *bootstrap.Components, redis *rediscommon.Client, casClient clients.CASClient, runService *service.RunService) *RunHandler {
	return &RunHandler{
		components: components,
		redis:      redis,
		casClient:  casClient,
		runService: runService,
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
	irJSON, err := h.redis.Get(c.Request().Context(), irKey)
	if err != nil {
		// Check if it's a "not found" error
		if err.Error() == fmt.Sprintf("key not found: %s", irKey) {
			return echo.NewHTTPError(http.StatusNotFound, "run not found")
		}
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

	if err := h.redis.Set(c.Request().Context(), irKey, string(newIRJSON), 0); err != nil {
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
			configData, err := h.casClient.Get(context.Background(), node.ConfigRef)
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

// ExecuteWorkflow creates a new workflow run with materialized workflow
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

	h.components.Logger.Info("execute workflow request",
		"tag", tagName,
		"username", username)

	// Create run using RunService
	// This will: materialize workflow, store as artifact, create run entry, publish to stream
	createReq := &service.CreateRunRequest{
		Tag:      tagName,
		Username: username,
		Inputs:   req.Inputs,
	}

	response, err := h.runService.CreateRun(ctx, createReq)
	if err != nil {
		// Check if it's a rate limit error
		if rateLimitErr, ok := err.(*service.RateLimitError); ok {
			h.components.Logger.Warn("rate limit exceeded",
				"username", username,
				"tier", rateLimitErr.Tier,
				"limit", rateLimitErr.Limit)

			return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
				"error":   "rate_limit_exceeded",
				"message": rateLimitErr.Error(),
				"details": map[string]interface{}{
					"tier":                rateLimitErr.Tier.String(),
					"limit":               rateLimitErr.Limit,
					"window":              "60 seconds",
					"current_count":       rateLimitErr.CurrentCount,
					"retry_after_seconds": rateLimitErr.RetryAfterSeconds,
				},
			})
		}

		h.components.Logger.Error("failed to create run", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create run: %v", err))
	}

	h.components.Logger.Info("run created successfully",
		"run_id", response.RunID,
		"artifact_id", response.ArtifactID,
		"tag", tagName)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"run_id":      response.RunID.String(),
		"artifact_id": response.ArtifactID.String(),
		"status":      response.Status,
		"tag":         response.Tag,
	})
}

// GetRun returns run status and metadata
func (h *RunHandler) GetRun(c echo.Context) error {
	runIDStr := c.Param("id")

	// Parse UUID
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run_id format")
	}

	// Get run from service
	run, err := h.runService.GetRun(c.Request().Context(), runID)
	if err != nil {
		h.components.Logger.Error("failed to get run", "run_id", runID, "error", err)
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	return c.JSON(http.StatusOK, run)
}

// ListWorkflowRuns returns runs for a workflow tag
func (h *RunHandler) ListWorkflowRuns(c echo.Context) error {
	tag := c.Param("tag")
	limitStr := c.QueryParam("limit")

	limit := 20 // Default
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	runs, err := h.runService.ListRunsForWorkflow(c.Request().Context(), tag, limit)
	if err != nil {
		h.components.Logger.Error("failed to list workflow runs", "tag", tag, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list runs")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"runs": runs,
	})
}

// GetRunDetails returns comprehensive run details
func (h *RunHandler) GetRunDetails(c echo.Context) error {
	runIDStr := c.Param("id")

	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run_id format")
	}

	details, err := h.runService.GetRunDetails(c.Request().Context(), runID)
	if err != nil {
		h.components.Logger.Error("failed to get run details", "run_id", runID, "error", err)
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	return c.JSON(http.StatusOK, details)
}
