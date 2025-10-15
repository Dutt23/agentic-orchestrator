# Patch Workflow Guide

## Overview

The `patch_workflow` tool allows agents to modify the workflow during execution. This enables self-modifying workflows where agents can add, remove, or update nodes and edges dynamically.

## Important: Connecting New Nodes

**When adding nodes, you MUST also add edges to connect them!**

New nodes that are not connected to the workflow graph will never execute. The LLM must generate patch operations that include both:

1. **Nodes** - The new workflow nodes
2. **Edges** - Connections from the current node to the new nodes

## Context Available to LLM

The LLM has access to:

- `context.current_workflow` - The full workflow structure (nodes and edges)
- `context.current_node_id` - The ID of the agent node making the patch
- Workflow schema documentation

## Example: Agent Self-Patching

### User Request
"Add a branch that checks if the value is > 5, then make an HTTP call to /api/notify"

### LLM Tool Call
```json
{
  "name": "patch_workflow",
  "arguments": {
    "patch_spec": {
      "operations": [
        {
          "op": "add",
          "path": "/nodes/-",
          "value": {
            "id": "branch_check_value",
            "type": "branch",
            "config": {
              "rules": [
                {
                  "condition": {
                    "field": "value",
                    "operator": ">",
                    "value": 5
                  },
                  "next_nodes": ["http_notify"]
                }
              ]
            }
          }
        },
        {
          "op": "add",
          "path": "/nodes/-",
          "value": {
            "id": "http_notify",
            "type": "http",
            "config": {
              "url": "https://api.example.com/notify",
              "method": "POST",
              "payload": {"event": "value_exceeded"}
            }
          }
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {
            "from": "agent_node_1",
            "to": "branch_check_value"
          }
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {
            "from": "branch_check_value",
            "to": "http_notify",
            "condition": "true"
          }
        }
      ],
      "description": "Added branch to check value and HTTP notification"
    }
  }
}
```

### Key Points

1. **Added 2 nodes**: `branch_check_value` and `http_notify`
2. **Added 2 edges**:
   - `agent_node_1` → `branch_check_value` (connects current node to branch)
   - `branch_check_value` → `http_notify` (connects branch to HTTP node when condition is true)
3. **Edge `from` field**: Uses `context.current_node_id` (agent_node_1)

## Workflow Execution After Patch

1. Agent node completes → signals coordinator
2. Patch is stored in `run_patches` table with seq number
3. Coordinator detects agent completion and checks for patches
4. Coordinator fetches patched workflow from orchestrator (base + all run patches applied)
5. Coordinator updates IR in Redis with patched workflow
6. Coordinator finds edges from agent node → routes to new nodes
7. New nodes execute with patched workflow

## Common Patterns

### Adding a Single Node

```json
{
  "operations": [
    {
      "op": "add",
      "path": "/nodes/-",
      "value": {
        "id": "new_node",
        "type": "http",
        "config": {"url": "..."}
      }
    },
    {
      "op": "add",
      "path": "/edges/-",
      "value": {
        "from": "current_node_id",
        "to": "new_node"
      }
    }
  ]
}
```

### Adding a Branch with Multiple Paths

```json
{
  "operations": [
    {
      "op": "add",
      "path": "/nodes/-",
      "value": {
        "id": "branch_node",
        "type": "branch",
        "config": {
          "rules": [
            {"condition": {...}, "next_nodes": ["path_a"]},
            {"condition": {...}, "next_nodes": ["path_b"]}
          ]
        }
      }
    },
    {
      "op": "add",
      "path": "/nodes/-",
      "value": {"id": "path_a", "type": "http", "config": {...}}
    },
    {
      "op": "add",
      "path": "/nodes/-",
      "value": {"id": "path_b", "type": "http", "config": {...}}
    },
    {
      "op": "add",
      "path": "/edges/-",
      "value": {"from": "current_node_id", "to": "branch_node"}
    }
  ]
}
```

Note: Branch nodes use `config.rules[].next_nodes` instead of edges for conditional routing.

### Adding a Sequence of Nodes

```json
{
  "operations": [
    {"op": "add", "path": "/nodes/-", "value": {"id": "node1", ...}},
    {"op": "add", "path": "/nodes/-", "value": {"id": "node2", ...}},
    {"op": "add", "path": "/nodes/-", "value": {"id": "node3", ...}},
    {"op": "add", "path": "/edges/-", "value": {"from": "current_node_id", "to": "node1"}},
    {"op": "add", "path": "/edges/-", "value": {"from": "node1", "to": "node2"}},
    {"op": "add", "path": "/edges/-", "value": {"from": "node2", "to": "node3"}}
  ]
}
```

## Debugging

If new nodes don't execute after a patch:

1. **Check the patch was stored**: `GET /api/v1/runs/{run_id}/patches`
2. **Check patch operations**: `GET /api/v1/runs/{run_id}/patches/{cas_id}/operations`
3. **Verify edges were added**: Operations should include both nodes AND edges
4. **Check edges have correct `from` field**: Should match `current_node_id`

## System Prompt Guidance

The system prompt should emphasize:

```
When using patch_workflow:
1. ALWAYS add edges to connect new nodes to the current node
2. Use context.current_node_id as the 'from' field in edges
3. For branch nodes, use config.rules[].next_nodes for conditional paths
4. Ensure all nodes are reachable from an existing node
```

## Future Enhancements

- [x] Auto-reload and apply patches to IR in coordinator
- [ ] Validation: Check all nodes are reachable
- [ ] UI: Visual diff showing what was patched
- [ ] Rollback: Ability to undo run patches
- [ ] Optimize: Cache materialized workflows to avoid recomputation
