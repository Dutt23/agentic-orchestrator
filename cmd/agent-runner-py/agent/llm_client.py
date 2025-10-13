"""OpenAI LLM client with prompt caching."""
from openai import OpenAI
from typing import Dict, Any, List, Optional
import logging
import time

from agent.tools import get_tool_schemas
from agent.system_prompt import get_system_prompt

logger = logging.getLogger(__name__)


class LLMClient:
    """OpenAI client with function calling and prompt caching."""

    def __init__(self, config: Dict[str, Any], workflow_schema_summary: Optional[str] = None):
        """Initialize OpenAI client.

        Args:
            config: LLM configuration
            workflow_schema_summary: Optional workflow schema summary to include in system prompt
        """
        self.config = config
        self.client = OpenAI()  # Uses OPENAI_API_KEY env var
        self.model = 'gpt-5-mini'
        self.temperature = 1
        self.max_tokens = config.get('max_tokens', 4000)
        self.timeout = config.get('timeout_sec', 30)

        # Static system prompt (will be cached by OpenAI)
        self.system_prompt = get_system_prompt(workflow_schema_summary)

        # Tool schemas
        self.tools = get_tool_schemas()

        logger.info(f"LLM client initialized with model: {self.model}")

    def chat(self, user_prompt: str, context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Send chat request to LLM with function calling.

        Args:
            user_prompt: User's natural language instruction
            context: Optional context (previous results, session info)

        Returns:
            Dictionary with tool calls and metadata
        """
        start_time = time.time()

        # Build messages
        messages = [
            {"role": "system", "content": self.system_prompt},
            {"role": "user", "content": self._build_user_message(user_prompt, context)}
        ]

        try:
            logger.info(f"Calling OpenAI with prompt: {user_prompt[:100]}...")

            response = self.client.chat.completions.create(
                model='gpt-5-mini',
                messages=messages,
                tools=self.tools,
                tool_choice="auto",
                temperature=self.temperature,
                max_completion_tokens=self.max_tokens,
                timeout=self.timeout
            )

            execution_time = int((time.time() - start_time) * 1000)

            # Extract tool calls
            message = response.choices[0].message
            tool_calls = []

            if message.tool_calls:
                for tool_call in message.tool_calls:
                    tool_calls.append({
                        "id": tool_call.id,
                        "function": {
                            "name": tool_call.function.name,
                            "arguments": tool_call.function.arguments
                        }
                    })

            # Get usage stats
            usage = response.usage
            tokens_used = usage.total_tokens if usage else 0

            # Check if cache was hit (OpenAI doesn't expose this directly yet,
            # but we can infer from prompt_tokens being lower than expected)
            # For now, we'll set to False and update when OpenAI exposes cache metrics
            cache_hit = False

            result = {
                "tool_calls": tool_calls,
                "message": message.content if message.content else "",
                "tokens_used": tokens_used,
                "cache_hit": cache_hit,
                "execution_time_ms": execution_time,
                "model": self.model,
                "finish_reason": response.choices[0].finish_reason
            }

            logger.info(f"LLM response: {len(tool_calls)} tool calls, {tokens_used} tokens, {execution_time}ms")

            return result

        except Exception as e:
            logger.error(f"LLM request failed: {e}")
            raise

    def _build_user_message(self, prompt: str, context: Optional[Dict[str, Any]]) -> str:
        """Build user message with context.

        Args:
            prompt: User's prompt
            context: Optional context information

        Returns:
            Formatted user message
        """
        message_parts = [prompt]

        if context:
            # Add current workflow structure if available
            if context.get('current_workflow'):
                workflow = context['current_workflow']
                message_parts.append("\n\n## Current Workflow Structure")

                # Show nodes
                nodes = workflow.get('nodes', [])
                if nodes:
                    message_parts.append(f"\n**Nodes ({len(nodes)}):**")
                    for node in nodes:
                        node_id = node.get('id', 'unknown')
                        node_type = node.get('type', 'unknown')
                        config_keys = list(node.get('config', {}).keys()) if node.get('config') else []
                        config_summary = f", config: {config_keys}" if config_keys else ""
                        message_parts.append(f"- `{node_id}` (type: {node_type}{config_summary})")

                # Show edges
                edges = workflow.get('edges', [])
                if edges:
                    message_parts.append(f"\n**Edges ({len(edges)}):**")
                    for edge in edges:
                        from_node = edge.get('from', '?')
                        to_node = edge.get('to', '?')
                        condition = edge.get('condition')
                        condition_str = f" [if {condition}]" if condition else ""
                        message_parts.append(f"- {from_node} → {to_node}{condition_str}")

            # Add previous results if available
            if context.get('previous_results'):
                message_parts.append("\n\n## Available Data")
                for result in context['previous_results']:
                    node_id = result.get('node_id', 'unknown')
                    result_ref = result.get('result_ref', 'N/A')
                    preview = result.get('preview', {})
                    message_parts.append(f"- From {node_id}: {result_ref} (preview: {preview})")

            # Add session info
            if context.get('session_id'):
                message_parts.append(f"\n\n## Session: {context['session_id']}")

            # Add workflow state
            if context.get('workflow_state'):
                state = context['workflow_state']
                message_parts.append(f"\n\n## Workflow State")
                message_parts.append(f"Step {state.get('current_step', '?')} of {state.get('total_steps', '?')}")

        return "\n".join(message_parts)
