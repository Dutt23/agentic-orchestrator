yes# Orchestrator Service

The orchestrator service manages Git-like versioning of workflows with branches, patches, and undo/redo support.

## Architecture

```
cmd/orchestrator/
├── main.go              # Entry point (Echo server)
├── models/              # Database models (match schema exactly)
│   ├── artifact.go      # Artifact, ArtifactKind
│   ├── cas_blob.go      # CASBlob
│   ├── tag.go           # Tag (Git-like branches)
│   ├── tag_move.go      # TagMove (undo/redo history)
│   ├── patch_chain.go   # PatchChainMember
│   └── run.go           # Run, RunSnapshotIndex
├── handlers/            # HTTP handlers (logic stubs)
│   ├── workflow.go      # Workflow CRUD
│   ├── tag.go           # Tag management (undo/redo)
│   └── run.go           # Run submission, patch creation
└── routes/              # Route registration (modular)
    ├── workflow.go      # /api/v1/workflows/*
    ├── tag.go           # /api/v1/tags/*
    └── run.go           # /api/v1/runs/*, /api/v1/patches/*
```

## API Endpoints

### Workflows
- `GET    /api/v1/workflows/:tag` - Get workflow by tag
- `POST   /api/v1/workflows` - Create new workflow
- `GET    /api/v1/workflows` - List all workflows
- `DELETE /api/v1/workflows/:tag` - Delete workflow tag

### Tags (Git-like branching)
- `GET  /api/v1/tags` - List all tags
- `GET  /api/v1/tags/:name` - Get specific tag
- `POST /api/v1/tags/:name/move` - Move tag to different artifact
- `POST /api/v1/tags/:name/undo` - Undo last move
- `POST /api/v1/tags/:name/redo` - Redo last undo
- `GET  /api/v1/tags/:name/history` - Get tag movement history

### Runs
- `POST /api/v1/runs` - Submit new run
- `GET  /api/v1/runs/:id` - Get run status
- `GET  /api/v1/runs?status=running` - List runs with filters
- `POST /api/v1/runs/:id/cancel` - Cancel running workflow

### Patches
- `POST /api/v1/patches` - Create patch on workflow
- `GET  /api/v1/patches/:id` - Get patch details

### Health
- `GET /health` - Health check

## Running

```bash
# Start with default config
./start.sh

# Or build and run
go build -o orchestrator ./cmd/orchestrator/
./orchestrator
```

## Environment Variables

See `common/bootstrap` for configuration options:
- `DB_HOST`, `DB_PORT`, `DB_NAME` - PostgreSQL connection
- `KAFKA_BROKERS` - Kafka brokers
- `REDIS_ADDR` - Redis/Dragonfly address
- `SERVICE_PORT` - HTTP port (default: 8081)

## Next Steps

1. ✅ Models created (match schema)
2. ✅ Routes structured (modular)
3. ✅ Handlers stubbed (ready for logic)
4. ⏳ Implement handler logic
5. ⏳ Add repository layer (DB queries)
6. ⏳ Add service layer (business logic)
7. ⏳ Add tests

## Design Principles

- **Modular**: Easy to add new routes (create new file in `routes/`)
- **Separation of Concerns**: handlers → service → repository
- **Database Models**: Match `migrations/001_final_schema.sql` exactly
- **Dependencies Injected**: All handlers receive `*bootstrap.Components`

## Adding New Routes

1. Create handler in `handlers/`:
```go
type NewHandler struct {
    components *bootstrap.Components
}

func NewNewHandler(components *bootstrap.Components) *NewHandler {
    return &NewHandler{components: components}
}

func (h *NewHandler) Handle(c echo.Context) error {
    // Implementation
}
```

2. Register routes in `routes/`:
```go
func RegisterNewRoutes(e *echo.Echo, components *bootstrap.Components) {
    h := handlers.NewNewHandler(components)
    e.GET("/api/v1/new", h.Handle)
}
```

3. Call in `main.go`:
```go
routes.RegisterNewRoutes(e, components)
```

Done!
