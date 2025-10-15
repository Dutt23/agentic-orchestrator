# Submission Documentation

> **Complete documentation package for the Agentic Orchestration Builder**

## 📖 Document Overview

**Purpose:** Navigation index for all submission documentation

**This is the documentation index.** For complete overview with flows and quick start, see [../readme.MD](../readme.MD)

**In this document:**
- [Documentation Index](#documentation-index) - Links to all 22 docs organized by category
- [Quick Links](#quick-links) - Jump to key documents

---

## Start Here

**Main README:** [../readme.MD](../readme.MD)

This folder contains comprehensive submission documentation organized by category.

---

## Documentation Index

### Architecture
- [CURRENT.md](./architecture/CURRENT.md) - Phase 1 MVP implementation
- [VISION.md](./architecture/VISION.md) - Production target architecture
- [MIGRATION_PATH.md](./architecture/MIGRATION_PATH.md) - Evolution roadmap

### Services
- [OVERVIEW.md](./services/OVERVIEW.md) - 7 services catalog with links

### Technical
- [DATABASE_SCHEMA.md](./technical/DATABASE_SCHEMA.md) - Postgres tables
- [REDIS_KEYS.md](./technical/REDIS_KEYS.md) - Redis key inventory
- [RATE_LIMITING.md](./technical/RATE_LIMITING.md) - Rate limiting architecture

### CLI Tool
- [README.md](./cli/README.md) - aob CLI overview
- [COMMANDS.md](./cli/COMMANDS.md) - Command reference

### Innovation
- [UNIQUENESS.md](./innovation/UNIQUENESS.md) - 10 unique features
- [TYPE_SAFETY.md](./innovation/TYPE_SAFETY.md) - JSON Schema type generation

### Operations
- [SCALABILITY.md](./operations/SCALABILITY.md) - OS tuning & performance

### References
- [references/](./references/) - Root documentation moved here

---

## Quick Links

**Core Innovation:** Runtime workflow patching - agents safely modify workflows mid-execution

**Key Docs:**
- [What's implemented](./architecture/CURRENT.md)
- [Production vision](./architecture/VISION.md)
- [10 unique features](./innovation/UNIQUENESS.md)
- [Rate limiting](./technical/RATE_LIMITING.md)

**Deep Technical:**
- [Choreography execution](../cmd/workflow-runner/docs/CHOREOGRAPHY_EXECUTION_DESIGN.md)
- [Agent service](../cmd/agent-runner-py/docs/AGENT_SERVICE.md)
- [Patch system](../cmd/orchestrator/docs/RUN_PATCHES_ARCHITECTURE.md)

---

**For complete overview, flows, and quick start:** See [../readme.MD](../readme.MD)
