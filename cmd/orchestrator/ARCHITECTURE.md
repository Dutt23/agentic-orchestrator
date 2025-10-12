# Orchestrator Service Architecture

## Layered Architecture

### 1. Repository Layer (`repository/`)
**Responsibility**: Direct database access, SQL queries

- `cas_blob.go` - CAS blob CRUD operations
- `artifact.go` - Artifact CRUD + patch chain queries
- `tag.go` - Tag CRUD + history + CAS operations

**Key Principle**: Repositories have ZERO business logic, only data access.

---

### 2. Service Layer (`service/`)
**Responsibility**: Business logic, orchestration

#### Focused Services (Single Responsibility)

**`cas.go` - Content-Addressed Storage Service**
```go
- StoreContent(content []byte, mediaType string) string
- GetContent(casID string) []byte
- ComputeHash(content []byte) string
- Exists(casID string) bool
```

**`artifact.go` - Artifact Catalog Service**
```go
- CreateDAGVersion(...) uuid.UUID
- CreatePatchSet(...) uuid.UUID
- CreateRunSnapshot(...) uuid.UUID
- GetByID(artifactID uuid.UUID) *Artifact
- GetByVersionHash(versionHash string) *Artifact
- GetPatchChain(headID uuid.UUID) []*Artifact
```

**`tag.go` - Tag Management Service**
```go
- CreateTag(tagName, targetKind, targetID, ...) error
- MoveTag(tagName, targetKind, targetID, ...) error
- CreateOrMoveTag(...) error (idempotent)
- GetTag(tagName string) *Tag
- ListTags() []*Tag
- DeleteTag(tagName string) error
- GetHistory(tagName string) []*TagMove
- CompareAndSwap(...) bool (optimistic locking)
```

#### Orchestrator Services (Compose Multiple Services)

**`workflow.go` - Workflow Orchestrator**
```go
type WorkflowServiceV2 struct {
    casService      *CASService
    artifactService *ArtifactService
    tagService      *TagService
}

- CreateWorkflow(req) (*CreateWorkflowResponse, error)
- GetWorkflowByTag(tagName string) (map[string]interface{}, error)
```

**Key Principle**: Services contain business logic but NO HTTP concerns.

---

### 3. Handler Layer (`handlers/`)
**Responsibility**: HTTP request/response, validation, status codes

**`workflow.go` - Workflow HTTP Handler**
```go
type WorkflowHandler struct {
    // Direct access to services
    casService      *CASService
    artifactService *ArtifactService
    tagService      *TagService

    // Optional: use orchestrator for simple flows
    workflowService *WorkflowServiceV2
}

// Two orchestration patterns:
1. Use WorkflowServiceV2 (cleaner, less code)
2. Orchestrate services directly (more control, transactions)
```

**Key Principle**: Handlers have ZERO business logic, only HTTP concerns.

---

## Orchestration Patterns

### Pattern 1: Use Lightweight Orchestrator Service
**When to use**: Simple multi-step workflows, no transactions needed

```go
func (h *WorkflowHandler) CreateWorkflow(c echo.Context) error {
    var req service.CreateWorkflowRequest
    c.Bind(&req)

    // Delegate to orchestrator
    resp, err := h.workflowService.CreateWorkflow(ctx, &req)

    return c.JSON(http.StatusCreated, resp)
}
```

**Pros**: Clean, less code in controller
**Cons**: Less control over transactions

---

### Pattern 2: Direct Service Orchestration in Controller
**When to use**: Complex transactions, need explicit control

```go
func (h *WorkflowHandler) CreateWorkflow(c echo.Context) error {
    var req service.CreateWorkflowRequest
    c.Bind(&req)

    // Step 1: CAS
    casID, err := h.casService.StoreContent(ctx, ...)

    // Step 2: Artifact
    artifactID, err := h.artifactService.CreateDAGVersion(ctx, ...)

    // Step 3: Tag
    err = h.tagService.CreateOrMoveTag(ctx, ...)

    return c.JSON(http.StatusCreated, resp)
}
```

**Pros**: Full control, explicit transaction boundaries
**Cons**: More code, repeated orchestration logic

---

## Service Reusability

### Example: Run Handler Can Reuse Services

```go
type RunHandler struct {
    artifactService *service.ArtifactService  // Shared!
    tagService      *service.TagService       // Shared!
    runService      *service.RunService       // Run-specific
}

func (h *RunHandler) CreateRun(c echo.Context) error {
    // Resolve tag using shared service
    tag := h.tagService.GetTag(ctx, "main")

    // Get artifact using shared service
    artifact := h.artifactService.GetByID(ctx, tag.TargetID)

    // Create run using run-specific service
    run := h.runService.CreateRun(ctx, artifact, ...)
}
```

---

## Benefits of This Architecture

### ✅ Separation of Concerns
- **Repository**: SQL only
- **Service**: Business logic only
- **Handler**: HTTP only

### ✅ Reusability
All services can be used by ANY handler:
- `WorkflowHandler` uses `ArtifactService`
- `RunHandler` uses `ArtifactService`
- `PatchHandler` uses `ArtifactService`

### ✅ Testability
```go
// Test service without HTTP
mockRepo := &MockArtifactRepository{}
service := NewArtifactService(mockRepo, log)
artifact, err := service.CreateDAGVersion(...)

// Test handler with mock service
mockService := &MockWorkflowService{}
handler := NewWorkflowHandler(components)
handler.workflowService = mockService
```

### ✅ Single Responsibility
- Each service has ONE reason to change
- CAS service changes don't affect Tag service
- Tag service changes don't affect Artifact service

### ✅ Flexibility
Choose orchestration pattern per endpoint:
- Simple endpoints: use orchestrator service
- Complex transactions: orchestrate directly in controller

---

## File Structure

```
cmd/orchestrator/
├── handlers/
│   ├── workflow.go      # HTTP layer
│   ├── run.go
│   └── tag.go
│
├── service/
│   ├── cas.go           # Focused services
│   ├── artifact.go
│   ├── tag.go
│   ├── workflow.go      # Orchestrator (composes above)
│   └── run.go
│
├── repository/
│   ├── cas_blob.go      # Data access
│   ├── artifact.go
│   └── tag.go
│
├── routes/
│   ├── workflow.go      # Route registration
│   ├── run.go
│   └── tag.go
│
└── models/
    ├── artifact.go      # Domain models
    ├── cas_blob.go
    └── tag.go
```

---

## Error Handling Strategy

### Service Layer
```go
// Return domain errors
func (s *ArtifactService) GetByID(ctx, id) (*Artifact, error) {
    artifact, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("artifact not found: %w", err)
    }
    return artifact, nil
}
```

### Handler Layer
```go
// Map domain errors to HTTP status codes
func (h *WorkflowHandler) GetWorkflow(c echo.Context) error {
    workflow, err := h.workflowService.GetWorkflowByTag(ctx, tagName)
    if err != nil {
        h.components.Logger.Error("failed to get workflow", "error", err)
        return c.JSON(http.StatusNotFound, map[string]interface{}{
            "error": "workflow not found",
        })
    }
    return c.JSON(http.StatusOK, workflow)
}
```

---

## Best Practices

1. **Keep Services Focused**: One service = one domain concept
2. **No Business Logic in Handlers**: Only HTTP concerns
3. **No SQL in Services**: Delegate to repositories
4. **Choose Right Pattern**: Orchestrator vs direct orchestration
5. **Log at Service Level**: Business events
6. **Log at Handler Level**: HTTP events
7. **Return Domain Errors**: Let handler map to HTTP status

---

## Migration Notes

- Old `workflow.go` service had 300+ lines with mixed concerns
- New architecture splits into:
  - `cas.go` (~80 lines) - storage only
  - `artifact.go` (~150 lines) - artifacts only
  - `tag.go` (~150 lines) - tags only
  - `workflow.go` (~150 lines) - orchestration only

Total: ~530 lines but each piece is focused, reusable, and testable.
