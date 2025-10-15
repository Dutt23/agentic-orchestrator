# Project Structure

Current repository structure (simplified, actual implementation).

```
orchestrator/
├── cmd/                          # Services (7 microservices)
│   ├── orchestrator/            # API + workflow management
│   ├── workflow-runner/         # Coordinator (stateless)
│   ├── agent-runner-py/         # LLM integration (Python)
│   ├── http-worker/             # HTTP execution
│   ├── hitl-worker/             # Human approvals
│   ├── fanout/                  # WebSocket streaming
│   └── aob-cli/                 # CLI tool (Rust)
│
├── common/                       # Shared libraries
│   ├── schema/                  # JSON Schema (single source of truth)
│   ├── sdk/                     # Workflow operations
│   ├── ratelimit/               # Workflow-aware rate limiting
│   ├── metrics/                 # Performance tracking
│   └── validation/              # Patch validators
│
├── crates/                       # Rust crates
│   └── dag-optimizer/           # WASM optimizer (stubs)
│
├── migrations/                   # Database migrations
│   └── *.sql
│
├── frontend/                     # UI (React + ReactFlow)
│   └── flow-builder/
│
├── scripts/systemd/              # Production systemd configs
│
├── dev_scripts/                  # Development scripts
│
├── submission_doc/               # Submission documentation
│
└── delete/                       # Obsolete/over-engineered docs
```

## Service Responsibilities

### cmd/orchestrator
- REST API (workflow CRUD, run submission)
- Patch application (base + patches → materialized)
- IR compilation
- Rate limiting

### cmd/workflow-runner
- Stateless coordinator
- Completion signal processing
- Node routing by type
- IR reloading (picks up patches!)

### cmd/agent-runner-py
- LLM integration (OpenAI)
- Tool execution (5 tools)
- Patch generation
- Prompt caching

### cmd/http-worker, cmd/hitl-worker, cmd/fanout
- Type-specific node execution
- Consume from Redis streams
- Publish completion signals

### cmd/aob-cli
- Developer CLI tool (Rust)
- Real-time log streaming (SSE)
- HITL approvals
- Patch management

## Shared Libraries

### common/schema/
JSON Schema definitions - single source of truth for types across all languages.

### common/ratelimit/
Workflow-aware rate limiting - analyzes workflow complexity to determine tier.

### common/sdk/
Workflow operations, IR compilation, CAS integration.

## Data Storage

### Redis (Hot Path)
- Compiled IR: `ir:{run_id}`
- Node outputs: `context:{run_id}`
- Token counter: `counter:{run_id}`
- Work queues: `wf.tasks.*` streams

### Postgres (Cold Path)
- Run metadata
- Patches
- Artifacts (workflow versions)

### CAS (Content-Addressed Storage)
- Workflow definitions
- Node outputs
- All content keyed by sha256

## Build Commands

```bash
make build        # Build all services
make start        # Start all services (dev)
make test         # Run tests
make clean        # Clean artifacts
```

## Docker Deployment

```bash
docker-compose build     # Build all images
docker-compose up -d     # Start all services
```

Each service has its own optimized Dockerfile in its directory.

## Development vs. Production

**MVP (Current):**
- Redis Streams (queues)
- Local processes (agents)
- Direct Postgres queries

**Production (Vision):**
- Kafka/Redpanda (partitioned queues)
- K8s Jobs / Lambda (agents)
- CQRS projections
- gRPC communication

See `submission_doc/architecture/` for complete architecture documentation.
