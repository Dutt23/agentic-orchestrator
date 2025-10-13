"""Workflow schema loader and utilities."""
import json
import os
import logging
from typing import Dict, Any, List

logger = logging.getLogger(__name__)


class WorkflowSchema:
    """Manages workflow schema information."""

    def __init__(self, schema_path: str = None):
        """Initialize workflow schema.

        Args:
            schema_path: Path to workflow.schema.json, defaults to common/schema location
        """
        if schema_path is None:
            # Default to common schema location (relative to repo root)
            base_dir = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
            schema_path = os.path.join(base_dir, "common", "schema", "workflow.schema.json")

        self.schema_path = schema_path
        self.schema = self._load_schema()
        self.node_types = self._extract_node_types()

        logger.info(f"Workflow schema loaded: {len(self.node_types)} node types")

    def _load_schema(self) -> Dict[str, Any]:
        """Load workflow schema from JSON file.

        Returns:
            Schema dictionary
        """
        try:
            with open(self.schema_path, 'r') as f:
                schema = json.load(f)
            logger.info(f"Loaded workflow schema from {self.schema_path}")
            return schema
        except FileNotFoundError:
            logger.warning(f"Schema file not found: {self.schema_path}, using minimal schema")
            return self._get_minimal_schema()
        except Exception as e:
            logger.error(f"Failed to load schema: {e}, using minimal schema")
            return self._get_minimal_schema()

    def _get_minimal_schema(self) -> Dict[str, Any]:
        """Get minimal schema if file not found.

        Returns:
            Minimal schema dictionary
        """
        return {
            "properties": {
                "nodes": {
                    "items": {
                        "properties": {
                            "type": {
                                "enum": ["function", "http", "conditional", "loop", "parallel", "transform", "aggregate", "filter"]
                            }
                        }
                    }
                }
            }
        }

    def _extract_node_types(self) -> List[str]:
        """Extract valid node types from schema.

        Returns:
            List of valid node type strings
        """
        try:
            node_def = self.schema.get("definitions", {}).get("Node", {})
            type_enum = node_def.get("properties", {}).get("type", {}).get("enum", [])
            return type_enum
        except Exception as e:
            logger.error(f"Failed to extract node types: {e}")
            return ["function", "http", "conditional", "loop", "parallel", "transform", "aggregate", "filter"]

    def get_schema_summary(self) -> str:
        """Get human-readable schema summary for LLM prompt.

        Returns:
            Schema summary string
        """
        summary = f"""## Workflow Schema

**Valid Node Types:**
{self._format_node_types()}

**Node Structure:**
- `id` (required): Unique identifier (alphanumeric, underscores, hyphens)
- `type` (required): One of the node types above
- `config` (optional): Node-specific configuration object
- `timeout_ms` (optional): Execution timeout in milliseconds
- `retry` (optional): Retry policy with max_attempts, backoff_ms, backoff_multiplier

**Edge Structure:**
- `from` (required): Source node ID
- `to` (required): Target node ID
- `condition` (optional): Condition expression for conditional edges

**Node Type Descriptions:**
- `function`: Execute a function/code
- `http`: Make HTTP requests
- `conditional`: Branch based on condition
- `loop`: Iterate over items
- `parallel`: Execute nodes in parallel
- `transform`: Transform data
- `aggregate`: Aggregate/combine data
- `filter`: Filter data based on criteria

**Important Rules:**
- Node IDs must be unique within the workflow
- Edges must reference existing node IDs
- No circular dependencies (DAG structure)
- At least one node is required
"""
        return summary

    def _format_node_types(self) -> str:
        """Format node types as bullet list.

        Returns:
            Formatted string
        """
        return "\n".join([f"- `{node_type}`" for node_type in self.node_types])

    def validate_node_type(self, node_type: str) -> bool:
        """Validate if node type is valid.

        Args:
            node_type: Node type to validate

        Returns:
            True if valid, False otherwise
        """
        return node_type in self.node_types

    def get_node_types(self) -> List[str]:
        """Get list of valid node types.

        Returns:
            List of node type strings
        """
        return self.node_types.copy()
