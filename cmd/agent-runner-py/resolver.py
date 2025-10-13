"""
Variable Expression Resolver for Agent Workers

Resolves variable expressions in node configs:
- $nodes.node_id - entire node output
- $nodes.node_id.field - specific field access
- ${$nodes.node_id.field} - string interpolation

This mirrors the Go implementation in cmd/workflow-runner/resolver/
"""

import json
import re
from typing import Any, Dict, List, Union
import redis
import logging


class Resolver:
    """Resolves variable expressions in workflow configs"""

    def __init__(self, redis_client: redis.Redis, logger: logging.Logger):
        self.redis = redis_client
        self.logger = logger

    def resolve_config(self, run_id: str, config: Dict[str, Any]) -> Dict[str, Any]:
        """
        Resolve all variable expressions in a config dict

        Args:
            run_id: The workflow run ID
            config: Configuration dict with potential variable references

        Returns:
            Resolved configuration with variables substituted
        """
        resolved = {}
        for key, value in config.items():
            try:
                resolved[key] = self._resolve_value(run_id, value)
            except Exception as e:
                raise ValueError(f"Failed to resolve config key {key}: {e}")
        return resolved

    def _resolve_value(self, run_id: str, value: Any) -> Any:
        """Recursively resolve a value (string, dict, list, etc.)"""
        if isinstance(value, str):
            return self._resolve_string(run_id, value)
        elif isinstance(value, dict):
            return {k: self._resolve_value(run_id, v) for k, v in value.items()}
        elif isinstance(value, list):
            return [self._resolve_value(run_id, item) for item in value]
        else:
            # Primitives pass through
            return value

    def _resolve_string(self, run_id: str, text: str) -> Union[str, Any]:
        """Handle string expressions"""
        # Case 1: Full node reference: "$nodes.node_id" or "$nodes.node_id.field"
        if text.startswith("$nodes."):
            return self._resolve_node_reference(run_id, text)

        # Case 2: String interpolation: "text ${$nodes.node_id} more"
        if "${" in text:
            return self._resolve_interpolation(run_id, text)

        # Case 3: Plain string
        return text

    def _resolve_node_reference(self, run_id: str, expr: str) -> Any:
        """Resolve "$nodes.node_id" or "$nodes.node_id.field.path" """
        # Remove "$nodes." prefix
        expr = expr.replace("$nodes.", "", 1)

        # Split into node_id and path
        parts = expr.split(".", 1)
        node_id = parts[0]

        # Load node output from Redis context
        output = self._load_node_output(run_id, node_id)

        # If no field path, return entire output
        if len(parts) == 1:
            return output

        # Extract specific field using dot notation
        field_path = parts[1]
        return self._get_nested_field(output, field_path)

    def _resolve_interpolation(self, run_id: str, text: str) -> str:
        """Handle string interpolation "${$nodes.node_id.field}" """
        # Pattern: ${$nodes.node_id.field.path}
        pattern = re.compile(r'\$\{([^}]+)\}')

        def replace_match(match):
            expr = match.group(1)  # Inner expression
            value = self._resolve_string(run_id, expr)
            # Convert to string
            if isinstance(value, (dict, list)):
                return json.dumps(value)
            return str(value)

        return pattern.sub(replace_match, text)

    def _load_node_output(self, run_id: str, node_id: str) -> Any:
        """Load a node's output from Redis context"""
        context_key = f"context:{run_id}"
        output_key = f"{node_id}:output"

        # Get CAS reference
        cas_ref = self.redis.hget(context_key, output_key)
        if not cas_ref:
            raise ValueError(f"Node output not found: {node_id}")

        cas_ref = cas_ref.decode('utf-8') if isinstance(cas_ref, bytes) else cas_ref

        # Load from CAS
        cas_key = f"cas:{cas_ref}"
        data = self.redis.get(cas_key)
        if not data:
            raise ValueError(f"CAS data not found: {cas_ref}")

        # Parse JSON
        try:
            return json.loads(data)
        except json.JSONDecodeError:
            # Return raw string if not JSON
            return data.decode('utf-8') if isinstance(data, bytes) else data

    def _get_nested_field(self, data: Any, path: str) -> Any:
        """Extract nested field using dot notation (e.g., 'body.items[0].price')"""
        # Simple implementation - handles dot notation
        # For array access like [0], we'd need more parsing

        parts = path.split(".")
        current = data

        for part in parts:
            # Handle array access like "items[0]"
            if "[" in part and "]" in part:
                field_name = part[:part.index("[")]
                index_str = part[part.index("[")+1:part.index("]")]

                if field_name:
                    current = current.get(field_name) if isinstance(current, dict) else current

                try:
                    index = int(index_str)
                    current = current[index]
                except (ValueError, IndexError, TypeError):
                    raise ValueError(f"Invalid array access: {part}")
            else:
                # Regular field access
                if isinstance(current, dict):
                    current = current.get(part)
                else:
                    raise ValueError(f"Cannot access field '{part}' on non-dict value")

            if current is None:
                raise ValueError(f"Field not found: {path}")

        return current


# Example usage:
"""
resolver = Resolver(redis_client, logger)

config = {
    "url": "https://httpbin.org/post",
    "method": "POST",
    "payload": {
        "flights": "$nodes.fetch_flights",
        "best_price": "$nodes.fetch_flights.body[0].price",
        "message": "Found ${$nodes.fetch_flights.count} flights"
    }
}

resolved_config = resolver.resolve_config(run_id, config)
"""
