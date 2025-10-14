# Run History & Run Detail Pages - Implementation Guide

## Status

✅ **Phase 1 Complete**: Backend repository layer (`ListByWorkflowTag` added)

## Remaining Implementation

This document provides complete code for all remaining files.

---

## Backend Implementation

### 1. Service Layer (`cmd/orchestrator/service/run.go`)

Add these structs and methods at the end of the file:

```go
// RunDetails represents comprehensive run information
type RunDetails struct {
	Run            *models.Run                   `json:"run"`
	WorkflowIR     map[string]interface{}        `json:"workflow_ir"`
	NodeExecutions map[string]*NodeExecution     `json:"node_executions"`
	Patches        []PatchInfo                   `json:"patches,omitempty"`
}

// NodeExecution represents execution details for a single node
type NodeExecution struct {
	NodeID      string                 `json:"node_id"`
	Status      string                 `json:"status"` // completed, failed, running, pending
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Error       *string                `json:"error,omitempty"`
}

// PatchInfo represents a patch applied during execution
type PatchInfo struct {
	Seq         int                        `json:"seq"`
	Operations  []map[string]interface{}   `json:"operations"`
	Description string                     `json:"description"`
}

// ListRunsForWorkflow lists runs for a specific workflow tag
func (s *RunService) ListRunsForWorkflow(ctx context.Context, tag string, limit int) ([]*models.Run, error) {
	return s.runRepo.ListByWorkflowTag(ctx, tag, limit)
}

// GetRunDetails retrieves comprehensive run details including execution data
func (s *RunService) GetRunDetails(ctx context.Context, runID uuid.UUID) (*RunDetails, error) {
	// 1. Get run from database
	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	// 2. Load workflow IR from Redis
	irKey := fmt.Sprintf("ir:%s", runID.String())
	irJSON, err := s.redis.Get(ctx, irKey).Result()
	if err != nil {
		s.components.Logger.Warn("failed to load IR from Redis (may have expired)", "run_id", runID, "error", err)
		// Return partial data without execution details
		return &RunDetails{
			Run:            run,
			WorkflowIR:     make(map[string]interface{}),
			NodeExecutions: make(map[string]*NodeExecution),
		}, nil
	}

	var workflowIR map[string]interface{}
	if err := json.Unmarshal([]byte(irJSON), &workflowIR); err != nil {
		return nil, fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	// 3. Load node execution context from Redis
	contextKey := fmt.Sprintf("context:%s", runID.String())
	nodeExecutions := make(map[string]*NodeExecution)

	// Get all fields from the context hash
	contextData, err := s.redis.HGetAll(ctx, contextKey).Result()
	if err != nil {
		s.components.Logger.Warn("failed to load context", "run_id", runID, "error", err)
	}

	// Parse node outputs and build execution map
	nodes, ok := workflowIR["nodes"].(map[string]interface{})
	if ok {
		for nodeID := range nodes {
			execution := &NodeExecution{
				NodeID: nodeID,
				Status: "pending", // Default status
			}

			// Check if node has output in context
			outputKey := nodeID + ":output"
			if outputRef, exists := contextData[outputKey]; exists {
				// Load output from CAS
				if output, err := s.loadFromCAS(ctx, outputRef); err == nil {
					execution.Output = output
					execution.Status = "completed"
				}
			}

			// Check for failure
			failureKey := nodeID + ":failure"
			if failureData, exists := contextData[failureKey]; exists {
				var failure map[string]interface{}
				if err := json.Unmarshal([]byte(failureData), &failure); err == nil {
					execution.Status = "failed"
					if errMsg, ok := failure["error"].(string); ok {
						execution.Error = &errMsg
					}
				}
			}

			nodeExecutions[nodeID] = execution
		}
	}

	// 4. TODO: Load patches if this run had any
	// For now, return empty patches array
	patches := []PatchInfo{}

	return &RunDetails{
		Run:            run,
		WorkflowIR:     workflowIR,
		NodeExecutions: nodeExecutions,
		Patches:        patches,
	}, nil
}

// loadFromCAS helper to load and parse CAS data
func (s *RunService) loadFromCAS(ctx context.Context, casRef string) (map[string]interface{}, error) {
	casKey := fmt.Sprintf("cas:%s", casRef)
	data, err := s.redis.Get(ctx, casKey).Result()
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, err
	}

	return result, nil
}
```

### 2. Handler Layer (`cmd/orchestrator/handlers/run.go`)

Add these methods at the end of the file:

```go
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
```

Don't forget to add the `strconv` import at the top:

```go
import (
	// ... existing imports
	"strconv"
)
```

### 3. Routes (`cmd/orchestrator/routes/run.go`)

Update the `RegisterRunRoutes` function to add the new routes:

```go
// Add these lines in the workflows group
workflows.GET("/:tag/runs", runHandler.ListWorkflowRuns) // List runs for workflow

// Add this line in the runs group
runs.GET("/:id/details", runHandler.GetRunDetails) // Get run details
```

---

## Frontend Implementation

### 1. API Service (`frontend/flow-builder/src/services/api.js`)

Add at the end of the file:

```javascript
/**
 * List runs for a workflow tag
 * @param {string} tag - Workflow tag
 * @param {number} limit - Max runs to return (default: 20)
 * @returns {Promise<Array>} List of runs
 */
export async function listWorkflowRuns(tag, limit = 20) {
  const encodedTag = encodeURIComponent(tag);
  const data = await apiRequest(`/workflows/${encodedTag}/runs?limit=${limit}`);
  return data.runs || [];
}

/**
 * Get detailed run information
 * @param {string} runId - Run ID
 * @returns {Promise<Object>} Run details with node executions
 */
export async function getRunDetails(runId) {
  return await apiRequest(`/runs/${runId}/details`);
}
```

Update the default export:

```javascript
export default {
  // ... existing exports
  listWorkflowRuns,
  getRunDetails,
};
```

### 2. RunHistoryList Component

**Create file**: `frontend/flow-builder/src/components/workflow/RunHistoryList.jsx`

[See complete code in Phase 4.1 of the plan above]

### 3. Update ExecutionDrawer

**File**: `frontend/flow-builder/src/components/workflow/ExecutionDrawer.jsx`

Add import:

```javascript
import RunHistoryList from './RunHistoryList';
```

Update the render, replace the section before `{!hasStarted && (`:

```jsx
{/* Inputs Form - Show only before workflow starts */}
{!hasStarted && (
  <>
    <Box>
      <Heading size="sm" mb={4}>
        Workflow Inputs
      </Heading>
      <WorkflowInputsForm
        onSubmit={handleSubmit}
        isSubmitting={isRunning}
      />
    </Box>

    <Divider my={4} />

    {/* Run History */}
    <Box>
      <RunHistoryList workflowTag={workflowTag} />
    </Box>
  </>
)}
```

### 4. RunDetail Page

**Create file**: `frontend/flow-builder/src/pages/RunDetail.jsx`

[See complete code in Phase 4.2 of the plan above]

### 5. RunExecutionGraph Component

**Create file**: `frontend/flow-builder/src/components/workflow/RunExecutionGraph.jsx`

[See complete code in Phase 4.3 of the plan above]

### 6. NodeExecutionDetails Component

**Create file**: `frontend/flow-builder/src/components/workflow/NodeExecutionDetails.jsx`

[See complete code in Phase 4.4 of the plan above]

### 7. RunPatchesList Component

**Create file**: `frontend/flow-builder/src/components/workflow/RunPatchesList.jsx`

[See complete code in Phase 4.5 of the plan above]

### 8. Update Router

**File**: `frontend/flow-builder/src/main.jsx` (or `App.jsx`)

Add import:

```javascript
import RunDetail from './pages/RunDetail';
```

Add route in your router configuration:

```jsx
<Route path="/runs/:runId" element={<RunDetail />} />
```

---

## Testing

### Backend Tests

**Create file**: `cmd/orchestrator/repository/run_test.go`

```go
package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/common/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRepository_ListByWorkflowTag(t *testing.T) {
	// TODO: Setup test database
	// This requires test database configuration
	t.Skip("Integration test - requires database")

	// Example test structure:
	ctx := context.Background()
	db := setupTestDB(t) // You need to implement this
	repo := NewRunRepository(db)

	// Create test runs
	run1 := &models.Run{
		RunID:        uuid.New(),
		Status:       models.StatusCompleted,
		TagsSnapshot: map[string]string{"main": "artifact-1"},
		SubmittedBy:  stringPtr("user1"),
		SubmittedAt:  time.Now().Add(-2 * time.Hour),
	}
	err := repo.Create(ctx, run1)
	require.NoError(t, err)

	run2 := &models.Run{
		RunID:        uuid.New(),
		Status:       models.StatusFailed,
		TagsSnapshot: map[string]string{"main": "artifact-2"},
		SubmittedBy:  stringPtr("user1"),
		SubmittedAt:  time.Now().Add(-1 * time.Hour),
	}
	err = repo.Create(ctx, run2)
	require.NoError(t, err)

	// Test listing
	runs, err := repo.ListByWorkflowTag(ctx, "main", 10)
	assert.NoError(t, err)
	assert.Len(t, runs, 2)
	assert.Equal(t, run2.RunID, runs[0].RunID) // Most recent first
}

func stringPtr(s string) *string {
	return &s
}
```

### Frontend Tests

**Create file**: `frontend/flow-builder/src/components/workflow/__tests__/RunHistoryList.test.jsx`

[See complete test code in Phase 5.1 of the plan above]

---

## Quick Start Checklist

### Backend
- [x] Add `ListByWorkflowTag` to repository
- [ ] Add service methods (RunDetails structs + methods)
- [ ] Add handler methods
- [ ] Register routes
- [ ] Test with Postman/curl

### Frontend
- [ ] Add API methods to `services/api.js`
- [ ] Create `RunHistoryList.jsx`
- [ ] Update `ExecutionDrawer.jsx`
- [ ] Create `RunDetail.jsx` page
- [ ] Create `RunExecutionGraph.jsx`
- [ ] Create `NodeExecutionDetails.jsx`
- [ ] Create `RunPatchesList.jsx`
- [ ] Add route to router
- [ ] Test in browser

---

## API Testing Examples

### List runs for workflow
```bash
curl -H "X-User-ID: test-user" \
  http://localhost:8081/api/v1/workflows/main/runs?limit=10
```

### Get run details
```bash
curl -H "X-User-ID: test-user" \
  http://localhost:8081/api/v1/runs/{run-id}/details
```

---

## Notes

1. **CAS Data Expiry**: IR and context data expire after 24 hours in Redis. Runs older than 24 hours will show basic info only.

2. **Patches**: The patch loading is a TODO. You'll need to implement patch retrieval logic based on your patch storage strategy.

3. **Real-time Updates**: Consider adding WebSocket listeners to `RunHistoryList` to update statuses in real-time.

4. **Performance**: For workflows with many nodes, consider paginating node executions or lazy-loading outputs.

5. **Error Handling**: The current implementation gracefully degrades when Redis data is missing. Enhance error messages for better UX.

## Next Steps

After completing this implementation:
1. Add comprehensive error handling
2. Implement patch history retrieval
3. Add filtering/search to run history
4. Add export functionality for run data
5. Implement run cancellation
6. Add performance metrics to run details
