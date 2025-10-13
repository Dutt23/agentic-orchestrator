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
                "strict": True,
                "description": "Execute ephemeral data pipeline using composable primitives. Use this for one-time data operations that don't need to persist in the workflow.",
                "parameters": {
                    "type": "object",
                    "required": ["session_id", "pipeline"],
                    "additionalProperties": False,
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
                                    "params": {"type": "object", "description": "Query parameters or POST body"},
                                    # table_sort params
                                    "field": {"type": "string", "description": "Field name to sort/filter by"},
                                    "order": {"type": "string", "enum": ["asc", "desc"], "description": "Sort order"},
                                    # table_filter params
                                    "condition": {
                                        "type": "object",
                                        "description": "Filter condition",
                                        "properties": {
                                            "field": {"type": "string"},
                                            "op": {"type": "string", "enum": ["<", ">", "<=", ">=", "==", "!="]},
                                            "value": {}
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
                "strict": True,
                "description": "Create persistent workflow modifications. Use this for 'always', 'whenever', or 'schedule' type requests that should permanently change the workflow.",
                "parameters": {
                    "type": "object",
                    "required": ["workflow_tag", "patch_spec"],
                    "additionalProperties": False,
                    "properties": {
                        "workflow_tag": {
                            "type": "string",
                            "description": "Tag of workflow to patch (e.g., 'main')"
                        },
                        "workflow_owner": {
                            "type": "string",
                            "description": "Owner of the workflow"
                        },
                        "patch_spec": {
                            "type": "object",
                            "description": "JSON Patch operations to apply",
                            "properties": {
                                "operations": {
                                    "type": "array",
                                    "items": {
                                        "type": "object",
                                        "required": ["op", "path"],
                                        "properties": {
                                            "op": {
                                                "type": "string",
                                                "enum": ["add", "remove", "replace"],
                                                "description": "Operation type"
                                            },
                                            "path": {
                                                "type": "string",
                                                "description": "JSON Pointer path (e.g., '/nodes/-')"
                                            },
                                            "value": {
                                                "description": "Value for add/replace operations"
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
