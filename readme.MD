# Agentic Orchestration Builder

> **🚀 Quick Start with Docker:**
> 1. Copy `.env.example` to `.env` in this directory and add your `OPENAI_API_KEY`
> 2. Run: `cd docker && ./setup.sh`
> 3. Open: http://localhost:3000
>
> **Full guide:** [docker/START.md](docker/START.md)
>
> **🎥 Video Demos:** [See video_examples/](./video_examples/) - Agent patching, HITL, rate limiting demos

> **Event-driven orchestration with runtime workflow patching**

## 📖 Overview

**What:** Orchestration platform merging deterministic workflows with adaptive agents
**Core Innovation:** Runtime workflow patching - agents safely modify workflows mid-execution
**Status:** Phase 1 MVP complete (6 microservices)

**In this document:**
- [Documentation Links](#-documentation) - All docs organized by category
- [Core Innovation](#core-innovation-runtime-workflow-patching) - How runtime patching works
- [Quick Start](#quick-start) - Build, run, and test
- [Example Flows](#example-flows) - Agent patch & HITL flows
- [Architecture](#architecture-overview) - Services and data flow
- [10 Unique Features](#10-unique-features) - Key differentiators
- [Performance](#performance) - Current & target metrics
- [Testing](#testing) - Integration tests
- [Development](#development) - Project structure

---

## What Is This?

An orchestration platform that merges **deterministic workflows** (like Temporal/n8n) with **adaptive agent behavior** (like AutoGPT) - agents can safely modify workflows mid-execution through validated patches.

**Status:** Phase 1 MVP complete and working (6 microservices, Redis choreography, agent patching, HITL)

---

## 📚 Documentation

**Start here:** [submission_doc/README.md](./submission_doc/README.md)

### Architecture
- [Current Architecture (Phase 1)](./submission_doc/architecture/CURRENT.md) - What's implemented
- [Vision Architecture (Production)](./submission_doc/architecture/VISION.md) - Target architecture
- [Migration Path](./submission_doc/architecture/MIGRATION_PATH.md) - Evolution roadmap

### Services & Technical
- [Services Overview](./submission_doc/services/OVERVIEW.md) - 7 microservices
- [Database Schema](./submission_doc/technical/DATABASE_SCHEMA.md) - Postgres tables
- [Redis Keys](./submission_doc/technical/REDIS_KEYS.md) - Key inventory
- [Rate Limiting](./submission_doc/technical/RATE_LIMITING.md) - Multi-level rate limiting
- [Type Safety](./submission_doc/technical/TYPE_SAFETY.md) - JSON Schema type generation

### Innovation & Operations
- [10 Unique Features](./submission_doc/innovation/UNIQUENESS.md) - Key differentiators
- [Scalability & Network Tuning](./submission_doc/operations/SCALABILITY.md) - OS optimization

### CLI & Deep Technical
- [aob CLI Overview](./submission_doc/cli/README.md) - Developer tool
- [Choreography Execution](./cmd/workflow-runner/docs/CHOREOGRAPHY_EXECUTION_DESIGN.md) - Execution model
- [Agent Service](./cmd/agent-runner-py/docs/AGENT_SERVICE.md) - LLM integration
- [Patch System](./cmd/orchestrator/docs/RUN_PATCHES_ARCHITECTURE.md) - Runtime patching

---

## Core Innovation: Runtime Workflow Patching

Traditional systems: Static DAGs, restart required for changes
**Our approach:** Agents modify workflows while they're running!

```
Base Workflow (v1.0)
  + Agent Patch 1 (adds email node)
  + Agent Patch 2 (adds validation)
  = Materialized Workflow (v1.2)
  → Recompile to IR
  → Coordinator loads NEW IR
  → Routes to NEW nodes
  → Workflow continues with modified topology!
```

**Safe, validated, and observable.**

---

## Quick Start

### Prerequisites
```bash
# Required
- Go 1.21+
- Python 3.11+
- Rust 1.75+
- Postgres 15+
- Redis 7+

# Optional
- Node.js 18+ (for UI)
```

### Build & Run

```bash
# 1. Build all services
make build

# 2. Start infrastructure
docker-compose up -d  # or manually: postgres + redis

# 3. Run migrations
./dev_scripts/migrate.sh

# 4. Start services (separate terminals)
make start-orchestrator
make start-workflow-runner
make start-http-worker
make start-hitl-worker
cd cmd/agent-runner-py && python main.py

# 5. Start UI (optional)
cd frontend/flow-builder && npm run dev

# 6. Test
./dev_scripts/test_run.sh
```

### Using the CLI

```bash
# Build CLI
cd cmd/aob-cli && cargo build --release

# Start workflow
aob run start examples/workflow.json

# Stream logs
aob logs stream run_7f3e4a

# HITL approval
aob approve ticket_456 approve --reason "LGTM"
```

---

## Example Flows

### Flow 1: Simple Agent Patch

```
User submits workflow: fetch_data → process → save
   ↓
fetch_data completes
   ↓
Agent node executes:
  - LLM analyzes data
  - Decides: "Data needs validation before processing"
  - Generates patch: Add validate_schema node
   ↓
Patch validation (3 layers)
   ↓
Orchestrator applies patch:
  Base + Patch → Materialized workflow
   ↓
IR recompilation:
  New topology: fetch_data → validate_schema → process → save
   ↓
Coordinator loads NEW IR
   ↓
Routes to validate_schema (NEW node!)
   ↓
Workflow continues with validation step!
```

### Flow 2: HITL Approval

```
Workflow: process_payment → charge_customer → send_receipt
   ↓
process_payment completes (amount: $50,000)
   ↓
charge_customer is HITL node (requires approval)
   ↓
HITL worker:
  - Creates approval record
  - Sets counter=0, pending_approvals=[ticket_456]
  - Workflow PAUSES
   ↓
Manager reviews in UI or CLI:
  aob approve ticket_456 approve --reason "Verified"
   ↓
HITL worker:
  - Removes from pending_approvals
  - Publishes completion signal
   ↓
Coordinator:
  - Counter increments
  - Routes to charge_customer
   ↓
Workflow RESUMES and completes!
```

---

## Architecture Overview

### Services (7 total)

1. **Orchestrator** - Workflow metadata, patch application, materialization
2. **Workflow-Runner** - Stateless coordinator (routes tokens, loads IR)
3. **Agent-Runner-Py** - LLM integration (can run in K8s/Lambda/customer env)
4. **HTTP-Worker** - SSRF-protected HTTP execution
5. **HITL-Worker** - Human approval gates
6. **Fanout** - Real-time WebSocket streaming
7. **aob CLI** - Developer tool (Rust)

### Data Flow

```
User/CLI → API → Orchestrator → Materialization → Redis (IR)
                                                      ↓
                                          Coordinator (stateless)
                                                      ↓
                        Workers (agent, http, hitl) execute nodes
                                                      ↓
                                    Completion signals → Coordinator
                                                      ↓
                                      Routes to next nodes → Repeat
```

---

## 10 Unique Features

1. **Runtime workflow patching** - Agents modify workflows mid-execution
2. **Workflow-aware rate limiting** - Tiered by complexity
3. **Stateless coordinator** - Crash-resume without data loss
4. **Triple-layer agent protection** - Prevents runaway costs
5. **Customer execution environments** - K8s, Lambda, or BYOE
6. **OS-level optimization** - Advanced network tuning (0-RTT, BBR, RPS/XPS)
7. **LLM optimizations** - Prompt caching for improved performance
8. **Graceful degradation** - Unknown nodes don't hang workflows
9. **Fast CLI tool** - Rust-based with SSE streaming
10. **Multi-language type safety** - JSON Schema → Rust/Go/TypeScript/Python

---

## Performance

**Current (MVP):** 1,000 workflows/sec, 5-10ms latency
**Target (Production):** 10,000+ workflows/sec, <2ms latency

**Optimizations:**
- CPU pinning (control vs. runner plane)
- Advanced network tuning (0-RTT, BBR, tcp_tw_reuse)
- LLM prompt caching
- Multi-core packet steering (RPS/XPS/RSS)

---

## Testing

```bash
# Integration test
./dev_scripts/test_run.sh

# Rate limiting test
./dev_scripts/test_rate_limit.sh

# Agent patching test
cd cmd/agent-runner-py && pytest tests/
```

---

## Development

### Project Structure

```
orchestrator/
├── cmd/                    # Services (7 microservices)
│   ├── orchestrator/       # API + workflow management
│   ├── workflow-runner/    # Coordinator
│   ├── agent-runner-py/    # LLM integration
│   ├── http-worker/        # HTTP execution
│   ├── hitl-worker/        # Approvals
│   ├── fanout/             # WebSocket streaming
│   └── aob-cli/            # CLI tool (Rust)
│
├── common/                 # Shared libraries
│   ├── schema/             # JSON Schema (single source of truth)
│   ├── sdk/                # Workflow operations
│   ├── ratelimit/          # Workflow-aware rate limiting
│   └── metrics/            # Performance tracking
│
├── crates/                 # Rust crates (dag-optimizer)
├── submission_doc/         # Submission documentation
├── dev_scripts/            # Development scripts
├── scripts/systemd/        # OS-level configs
└── frontend/               # UI (React + ReactFlow)
```

### Adding New Node Type

1. Edit [`frontend/flow-builder/public/node-registry.json`](./frontend/flow-builder/public/node-registry.json)
2. Add node definition with config schema
3. UI loads dynamically (no code changes needed!)
4. Implement worker for the new node type
5. Backend controls node availability (active, coming_soon, experimental)

**Dynamic node registry:** Backend can serve this JSON via API, allowing runtime node type updates without UI redeployment.

---

## Documentation

**Start here:** [submission_doc/README.md](./submission_doc/README.md)

**Quick links:**
- [What's built (Phase 1)](./submission_doc/architecture/CURRENT.md)
- [Production target](./submission_doc/architecture/VISION.md)
- [10 unique features](./submission_doc/innovation/UNIQUENESS.md)
- [7 services overview](./submission_doc/services/OVERVIEW.md)
- [Advanced network tuning](./submission_doc/operations/SCALABILITY.md)
