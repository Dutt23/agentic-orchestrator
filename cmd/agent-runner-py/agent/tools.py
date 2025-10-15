"""Tool definitions for LLM function calling."""
from typing import List, Dict, Any


def get_tool_schemas() -> List[Dict[str, Any]]:
    """Get tool schemas for OpenAI function calling.

    Returns:
        List of tool definitions
    """
    return [
        {
            "type": "function",
            "function": {
                "name": "execute_pipeline",
                "description": "Execute ephemeral data pipeline using composable primitives. Use this for one-time data operations that don't need to persist in the workflow.",
                "parameters": {
                    "type": "object",
                    "required": ["session_id", "pipeline"],
                    "additionalProperties": True,
                    "properties": {
                        "session_id": {
                            "type": "string",
                            "description": "Session ID for context tracking"
                        },
                        "pipeline": {
                            "type": "array",
                            "description": "Array of pipeline steps to execute in sequence",
                            "items": {
                                "type": "object",
                                "required": ["step"],
                                "additionalProperties": True,
                                "properties": {
                                    "step": {
                                        "type": "string",
                                        "enum": [
                                            "http_request",
                                            "table_sort",
                                            "table_filter",
                                            "table_select",
                                            "top_k"
                                        ],
                                        "description": "Pipeline step type"
                                    },
                                    # http_request params
                                    "url": {"type": "string", "description": "URL for HTTP request"},
                                    "method": {"type": "string", "enum": ["GET", "POST"], "description": "HTTP method"},
                                    "params": {
                                        "type": "object",
                                        "description": "Query parameters or POST body",
                                        "additionalProperties": True,
                                        "properties": {}
                                    },
                                    # table_sort params
                                    "field": {"type": "string", "description": "Field name to sort/filter by"},
                                    "order": {"type": "string", "enum": ["asc", "desc"], "description": "Sort order"},
                                    # table_filter params
                                    "condition": {
                                        "type": "object",
                                        "description": "Filter condition",
                                        "required": ["field", "op", "value"],
                                        "additionalProperties": True,
                                        "properties": {
                                            "field": {"type": "string"},
                                            "op": {"type": "string", "enum": ["<", ">", "<=", ">=", "==", "!="]},
                                            "value": {
                                                "description": "Value to compare against",
                                                "anyOf": [
                                                    {"type": "string"},
                                                    {"type": "number"},
                                                    {"type": "boolean"}
                                                ]
                                            }
                                        }
                                    },
                                    # table_select params
                                    "fields": {
                                        "type": "array",
                                        "items": {"type": "string"},
                                        "description": "Fields to select"
                                    },
                                    # top_k params
                                    "k": {"type": "integer", "minimum": 1, "description": "Number of records to take"}
                                }
                            }
                        },
                        "input_ref": {
                            "type": "string",
                            "description": "Optional CAS reference to input data (e.g., cas://sha256:...)"
                        }
                    }
                }
            }
        },
        {
            "type": "function",
            "function": {
                "name": "patch_workflow",
                "description": """Create persistent workflow modifications during runtime. Use this when the workflow needs to modify itself.

IMPORTANT: When adding nodes, you MUST also add edges to connect them to the current node.
- Use context.current_workflow to get the workflow structure
- Use context.current_node_id to get the ID of the node making this patch
- Add edges from current_node_id to your new nodes to ensure they execute next

Example: If current_node_id is 'agent_1' and you add 'branch_1', you must add:
  {"op": "add", "path": "/edges/-", "value": {"from": "agent_1", "to": "branch_1"}}
""",
                "parameters": {
                    "type": "object",
                    "required": ["patch_spec"],
                    "additionalProperties": True,
                    "properties": {
                        "workflow_tag": {
                            "type": "string",
                            "description": "Tag of workflow to patch (optional, will use current workflow)"
                        },
                        "workflow_owner": {
                            "type": "string",
                            "description": "Owner of the workflow (optional, will use current user)"
                        },
                        "patch_spec": {
                            "type": "object",
                            "description": "JSON Patch operations to apply. MUST include both nodes AND edges to connect them.",
                            "required": ["operations"],
                            "additionalProperties": True,
                            "properties": {
                                "operations": {
                                    "type": "array",
                                    "description": "Array of patch operations. When adding nodes, ALWAYS add corresponding edges to connect them to the current node.",
                                    "items": {
                                        "type": "object",
                                        "required": ["op", "path"],
                                        "additionalProperties": True,
                                        "properties": {
                                            "op": {
                                                "type": "string",
                                                "enum": ["add", "remove", "replace"],
                                                "description": "Operation type"
                                            },
                                            "path": {
                                                "type": "string",
                                                "description": "JSON Pointer path (e.g., '/nodes/-' for nodes, '/edges/-' for edges)"
                                            },
                                            "value": {
                                                "description": "MUST be an object. For nodes: {id: string, type: string, config: object}. For edges: {from: string, to: string, condition?: string}",
                                                "type": "object",
                                                "properties": {
                                                    "id": {
                                                        "type": "string",
                                                        "description": "Unique node ID (for nodes)"
                                                    },
                                                    "type": {
                                                        "type": "string",
                                                        "enum": ["agent", "http", "hitl", "conditional", "loop"],
                                                        "description": "Node type (for nodes)"
                                                    },
                                                    "config": {
                                                        "type": "object",
                                                        "description": "Node configuration as key-value object (NOT array). Example: {\"task\": \"do something\"}",
                                                        "additionalProperties": True
                                                    },
                                                    "from": {
                                                        "type": "string",
                                                        "description": "Source node ID (for edges)"
                                                    },
                                                    "to": {
                                                        "type": "string",
                                                        "description": "Target node ID (for edges)"
                                                    },
                                                    "condition": {
                                                        "type": "string",
                                                        "description": "Conditional expression (for edges, optional)"
                                                    }
                                                },
                                                "additionalProperties": False
                                            }
                                        }
                                    }
                                },
                                "description": {
                                    "type": "string",
                                    "description": "Human-readable description of the patch"
                                }
                            }
                        }
                    }
                }
            }
        }
    ]
