"""OpenAI LLM client with prompt caching and connection pooling."""
from openai import OpenAI
from typing import Dict, Any, List, Optional
import logging
import time
import httpx
import os

from agent.tools import get_tool_schemas
from agent.system_prompt import get_system_prompt

logger = logging.getLogger(__name__)


class LLMClient:
    """OpenAI client with function calling, prompt caching, and connection pooling."""

    def __init__(self, config: Dict[str, Any], workflow_schema_summary: Optional[str] = None,
                 current_workflow: Optional[Dict[str, Any]] = None):
        """Initialize OpenAI client with optimizations.

        Args:
            config: LLM configuration
            workflow_schema_summary: Optional workflow schema summary
            current_workflow: Current workflow structure (for prompt caching)
        """
        self.config = config
        self.model = 'gpt-5-mini'
        self.temperature = 1
        self.max_tokens = config.get('max_tokens', 4000)
        self.timeout = config.get('timeout_sec', 30)

        # Configure HTTP client with connection pooling
        # Reuses TCP connections to reduce latency (saves 100-300ms per request)
        #
        # TODO: FIX SSL VERIFICATION FOR PRODUCTION!
        # Temporary workaround: SSL verification disabled for development
        # This is needed because certifi bundle path is incorrect in Docker
        # For production, must fix certificate path or use proper SSL context
        logger.warning("SSL verification is DISABLED - this is a temporary workaround for development")

        http_client = httpx.Client(
            limits=httpx.Limits(
                max_connections=100,  # Max concurrent connections
                max_keepalive_connections=20,  # Keep 20 connections warm
                keepalive_expiry=300  # Keep alive for 5 minutes
            ),
            timeout=self.timeout,
            verify=False  # TEMPORARY: Disable SSL verification
        )

        # Initialize OpenAI client with connection pooling
        self.client = OpenAI(http_client=http_client)

        # System prompt with workflow context (cached by OpenAI if >1024 tokens)
        # OpenAI automatically caches the prefix when same prompt is reused
        self.system_prompt = get_system_prompt(workflow_schema_summary, current_workflow)

        # Tool schemas
        self.tools = get_tool_schemas()

        logger.info(f"LLM client initialized with model: {self.model}, connection pooling enabled")

    def chat(self, user_prompt: str, context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Send chat request to LLM with function calling.

        Args:
            user_prompt: User's natural language instruction
            context: Optional context (previous results, session info, current_workflow)

        Returns:
            Dictionary with tool calls and metadata
        """
        start_time = time.time()

        # Rebuild system prompt with current workflow if provided (for caching)
        # OpenAI caches the system prompt prefix - if workflow doesn't change, cache hit!
        system_prompt = self.system_prompt
        if context and context.get('current_workflow'):
            system_prompt = get_system_prompt(
                workflow_schema_summary=None,  # Already in base prompt
                current_workflow=context['current_workflow']
            )
            # Prepend the base prompt (without workflow schema to avoid duplication)
            base = self.system_prompt.split('\n\n## Current Workflow Structure')[0]
            system_prompt = base + "\n\n## Current Workflow Structure\n" + system_prompt.split('\n\n## Current Workflow Structure\n')[-1]

        # Build messages
        messages = [
            {"role": "system", "content": system_prompt},
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

        Note: Workflow structure is now in system prompt for caching.
        This message should only contain dynamic, per-request information.

        Args:
            prompt: User's prompt
            context: Optional context information

        Returns:
            Formatted user message (task-focused, minimal)
        """
        message_parts = [prompt]

        if context:
            # Add previous results if available (dynamic per-run)
            if context.get('previous_results'):
                message_parts.append("\n\n## Available Data")
                for result in context['previous_results']:
                    node_id = result.get('node_id', 'unknown')
                    result_ref = result.get('result_ref', 'N/A')
                    preview = result.get('preview', {})
                    message_parts.append(f"- From {node_id}: {result_ref} (preview: {preview})")

            # Add session info (dynamic per-session)
            if context.get('session_id'):
                message_parts.append(f"\n\n## Session: {context['session_id']}")

            # Add workflow state (dynamic per-step)
            if context.get('workflow_state'):
                state = context['workflow_state']
                message_parts.append(f"\n\n## Workflow State")
                message_parts.append(f"Step {state.get('current_step', '?')} of {state.get('total_steps', '?')}")

        return "\n".join(message_parts)
