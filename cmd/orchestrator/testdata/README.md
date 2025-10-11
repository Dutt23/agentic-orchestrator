# Workflow Orchestrator Test Data

This directory contains test data and scripts for the Workflow Orchestrator API.

## Directory Structure

```
testdata/
├── workflow_simple.json        # Simple 3-node workflow
├── workflow_complex.json       # Complex workflow with parallel branches
├── patch_add_node.json         # Patch: Add a new node
├── patch_update_timeout.json   # Patch: Update timeouts
├── patch_remove_node.json      # Patch: Remove a node
├── test_api.sh                 # Automated API test script
└── README.md                   # This file
```

## Test Data Files

### Workflows

#### 1. `workflow_simple.json`
Simple lead processing workflow with 3 sequential steps:
- validate_lead → enrich_data → score_lead
- Good for testing basic workflow creation and retrieval

#### 2. `workflow_complex.json`
Customer onboarding pipeline with:
- 7 nodes (start, validate, parallel KYC/credit checks, setup, email, assign, end)
- 9 edges including parallel branches
- Good for testing complex DAG structures

### Patches (JSON Patch Format - RFC 6902)

#### 1. `patch_add_node.json`
Adds a new "notify_slack" node and connects it to the workflow

#### 2. `patch_update_timeout.json`
Updates timeout values for existing nodes

#### 3. `patch_remove_node.json`
Removes a node and its associated edges

## Running Tests

### Prerequisites

1. **Database Setup**
   ```bash
   # Ensure PostgreSQL is running
   psql -U postgres -f ../../migrations/001_final_schema.sql
   ```

2. **Start the Orchestrator**
   ```bash
   cd ../..
   ./bin/orchestrator
   # Server should start on http://localhost:8080
   ```

3. **Install jq** (for JSON formatting)
   ```bash
   # macOS
   brew install jq

   # Ubuntu/Debian
   sudo apt-get install jq
   ```

### Run Automated Tests

```bash
cd testdata
./test_api.sh
```

This will:
- Create workflows on different branches (main, dev, prod)
- Test retrieval with and without materialization
- List all workflows
- Test error handling
- Delete a workflow tag

### Manual Testing with curl

#### 1. Create a Workflow

```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -H "X-User-ID: your-user-id" \
  -d '{
    "tag_name": "main",
    "workflow": '"$(cat workflow_simple.json)"',
    "created_by": "your-user-id"
  }' | jq
```

**Expected Response:**
```json
{
  "artifact_id": "uuid",
  "cas_id": "sha256:...",
  "version_hash": "sha256:...",
  "tag_name": "main",
  "nodes_count": 3,
  "edges_count": 2,
  "created_at": "2025-01-11T..."
}
```

#### 2. Get Workflow (Metadata Only)

```bash
curl -X GET "http://localhost:8080/api/v1/workflows/main?materialize=false" | jq
```

**Expected Response:**
```json
{
  "tag": "main",
  "artifact_id": "uuid",
  "kind": "dag_version",
  "depth": 0,
  "patch_count": 0,
  "components": {
    "base_cas_id": "sha256:...",
    "base_version_hash": "sha256:..."
  },
  "workflow": null
}
```

#### 3. Get Workflow (Fully Materialized)

```bash
curl -X GET "http://localhost:8080/api/v1/workflows/main?materialize=true" | jq
```

**Expected Response:**
```json
{
  "tag": "main",
  "artifact_id": "uuid",
  "kind": "dag_version",
  "depth": 0,
  "patch_count": 0,
  "components": { ... },
  "workflow": {
    "name": "Lead Processing Workflow",
    "nodes": [...],
    "edges": [...]
  }
}
```

#### 4. List All Workflows

```bash
curl -X GET http://localhost:8080/api/v1/workflows | jq
```

**Expected Response:**
```json
{
  "workflows": [
    {
      "tag_name": "main",
      "target_kind": "dag_version",
      "target_id": "uuid",
      ...
    }
  ],
  "count": 1
}
```

#### 5. Delete a Workflow Tag

```bash
curl -X DELETE http://localhost:8080/api/v1/workflows/dev | jq
```

**Expected Response:**
```json
{
  "message": "workflow tag deleted successfully",
  "tag": "dev"
}
```

## Testing Scenarios

### Scenario 1: Simple Workflow Lifecycle
```bash
# 1. Create workflow
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{"tag_name": "main", "workflow": '"$(cat workflow_simple.json)"'}'

# 2. Get metadata
curl http://localhost:8080/api/v1/workflows/main?materialize=false

# 3. Get full workflow
curl http://localhost:8080/api/v1/workflows/main?materialize=true
```

### Scenario 2: Multiple Branches
```bash
# Create workflows on different branches
curl -X POST http://localhost:8080/api/v1/workflows \
  -d '{"tag_name": "main", "workflow": '"$(cat workflow_simple.json)"'}'

curl -X POST http://localhost:8080/api/v1/workflows \
  -d '{"tag_name": "dev", "workflow": '"$(cat workflow_complex.json)"'}'

curl -X POST http://localhost:8080/api/v1/workflows \
  -d '{"tag_name": "prod", "workflow": '"$(cat workflow_simple.json)"'}'

# List all
curl http://localhost:8080/api/v1/workflows
```

### Scenario 3: Performance Testing (Bulk Query)
```bash
# Create workflow with patches (to be implemented in Phase 2)
# This will test the bulk query optimization for patch chains
```

## Query Performance Metrics

Based on the implementation:

| Scenario | Query Count | Notes |
|----------|-------------|-------|
| dag_version (simple) | 3 queries | Tag resolution + base content |
| patch_set (depth=5) | 6 queries | Tag + chain + base + **bulk patch fetch** |
| patch_set (depth=20) | 6 queries | **Constant** due to bulk optimization |

## Troubleshooting

### Issue: Connection Refused
```
curl: (7) Failed to connect to localhost port 8080
```
**Solution:** Ensure the orchestrator is running:
```bash
../../bin/orchestrator
```

### Issue: Tag Not Found
```json
{"error": "workflow not found"}
```
**Solution:** Create the workflow first using POST /workflows

### Issue: jq Command Not Found
```
-bash: jq: command not found
```
**Solution:** Install jq (see Prerequisites)

## Next Steps (Phase 2)

When patch materialization is implemented:
1. Test patch application with `patch_*.json` files
2. Verify depth tracking
3. Test compaction triggers (depth > 20)
4. Measure materialization performance

## Contributing

When adding new test data:
1. Follow the naming convention: `workflow_*.json` or `patch_*.json`
2. Include realistic node configurations
3. Update this README with descriptions
4. Add test cases to `test_api.sh`
