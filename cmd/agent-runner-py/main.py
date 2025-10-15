"""Agent Runner Service - Main entry point.

This service processes LLM-powered agent tasks during workflow execution.
It is a Redis worker that picks jobs from Redis queues, calls LLM with tools,
executes the tools, stores results in DB, and publishes results back to Redis.
"""
import os
import sys
import signal
import logging
import json
import yaml
import time
import psutil
import threading
from datetime import datetime, timezone
from threading import Thread
from concurrent.futures import ThreadPoolExecutor
from typing import Dict, Any, Optional
from dotenv import load_dotenv

# FastAPI for HTTP server
from fastapi import FastAPI, HTTPException
import uvicorn

# Import our modules
from agent.llm_client import LLMClient
from agent.workflow_schema import WorkflowSchema
from agent.intent_classifier import IntentClassifier
from storage.memory import MemoryStorage
from storage.redis_client import RedisClient
from pipeline.executor import execute_pipeline_tool
from workflow.patch_client import patch_workflow_tool
from patch_validator import validate_patch_operations
from metrics import RuntimeMetrics, SystemInfo, create_metrics

# Load environment variables
load_dotenv()

# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class AgentService:
    """Agent service that processes jobs from Redis queue."""

    def __init__(self, config: Dict[str, Any]):
        """Initialize agent service.

        Args:
            config: Service configuration dictionary
        """
        self.config = config
        self.running = False

        # Initialize clients
        logger.info("Initializing agent service...")

        # Load workflow schema
        self.workflow_schema = WorkflowSchema()
        schema_summary = self.workflow_schema.get_schema_summary()

        # Initialize intent classifier
        self.intent_classifier = IntentClassifier()

        self.redis = RedisClient(config['redis'])
        self.storage = MemoryStorage()
        self.llm = LLMClient(config['llm'], workflow_schema_summary=schema_summary)

        # Worker pool
        self.num_workers = config['service'].get('workers', 4)
        self.worker_pool = ThreadPoolExecutor(max_workers=self.num_workers)

        # Orchestrator URL for patch forwarding
        self.orchestrator_url = config['orchestrator']['api_url']

        # Storage config
        self.storage_config = config['storage']

        logger.info(f"Agent service initialized with {self.num_workers} workers")

    def start(self):
        """Start the agent service (HTTP server + worker pool)."""
        logger.info("Starting agent service...")
        self.running = True

        # Start HTTP server in background thread
        http_thread = Thread(target=self._run_http_server, daemon=True)
        http_thread.start()
        logger.info("HTTP server started in background")

        # Start worker pool
        logger.info(f"Starting {self.num_workers} workers...")
        futures = []
        for i in range(self.num_workers):
            future = self.worker_pool.submit(self._worker_loop, worker_id=i)
            futures.append(future)

        # Wait for all workers (blocks until shutdown)
        try:
            for future in futures:
                future.result()
        except KeyboardInterrupt:
            logger.info("Received interrupt signal")
            self.shutdown()

    def _worker_loop(self, worker_id: int):
        """Worker loop that processes jobs from Redis.

        Args:
            worker_id: Worker identifier for logging
        """
        logger.info(f"Worker {worker_id} started")

        while self.running:
            try:
                # Blocking pop from Redis (timeout configured in redis config)
                job = self.redis.pop_job()

                if job:
                    logger.info(f"Worker {worker_id} processing job {job.get('job_id')}")
                    self._process_job(job)
                else:
                    # Timeout, continue loop
                    pass

            except Exception as e:
                logger.error(f"Worker {worker_id} error: {e}", exc_info=True)
                time.sleep(1)  # Back off on error

        logger.info(f"Worker {worker_id} stopped")

    def _process_job(self, job: Dict[str, Any]):
        """Process a single job.

        Args:
            job: Job dictionary from Redis queue
        """
        # Track execution metrics
        sent_at = job.get('sent_at')  # From coordinator
        start_time = time.time()  # Use Unix timestamp

        # Capture runtime metrics at start
        runtime_metrics = RuntimeMetrics()

        # Log the received job for debugging
        logger.info(f"Received job data: {json.dumps(job, indent=2)}")

        job_id = job.get('job_id')
        run_id = job.get('run_id')
        node_id = job.get('node_id')
        task = job.get('task')
        context = job.get('context', {})

        # Validate required fields and report which ones are missing
        missing_fields = []
        if not job_id:
            missing_fields.append('job_id')
        if not run_id:
            missing_fields.append('run_id')
        if not node_id:
            missing_fields.append('node_id')
        if not task:
            missing_fields.append('task')

        if missing_fields:
            logger.error(f"Invalid job: missing required fields: {', '.join(missing_fields)}")
            return

        try:
            # Enhance context with current workflow if provided in job
            enhanced_context = context.copy() if context else {}

            # Check if job includes current workflow
            if job.get('current_workflow'):
                enhanced_context['current_workflow'] = job['current_workflow']
                logger.info(f"Job includes workflow with {len(job['current_workflow'].get('nodes', []))} nodes")

            # Add current node_id to context (needed for patch_workflow to connect edges)
            if node_id:
                enhanced_context['current_node_id'] = node_id
                logger.info(f"Added current_node_id to context: {node_id}")

            # Classify intent before calling LLM
            intent_result = self.intent_classifier.classify(task, enhanced_context)
            logger.info(f"Intent classified: {intent_result['intent']} "
                       f"(confidence: {intent_result['confidence']:.2f}) - "
                       f"{intent_result['reasoning']}")

            # Store intent classification in context for potential use
            enhanced_context['intent_classification'] = intent_result

            # Call LLM with tools
            logger.info(f"Calling LLM for job {job_id}")
            llm_result = self.llm.chat(task, enhanced_context)

            tool_calls = llm_result.get('tool_calls', [])
            logger.info(f"LLM returned {len(tool_calls)} tool calls")

            # Handle cases with or without tool calls
            if not tool_calls:
                # No tool calls - LLM responded without needing to execute tools
                logger.info("LLM did not call any tools - storing text response")
                result_data = {
                    "status": "completed",
                    "type": "text_response",
                    "message": llm_result.get('message', 'No response message'),
                    "finish_reason": llm_result.get('finish_reason'),
                    "note": "LLM completed without calling any tools"
                }
            else:
                # Validate tool choice matches intent (log warning if mismatch)
                chosen_tool = tool_calls[0].get('function', {}).get('name')
                expected_intent = intent_result['intent']

                if expected_intent == 'patch' and chosen_tool != 'patch_workflow':
                    logger.warning(f"Intent mismatch: classified as 'patch' but LLM chose '{chosen_tool}'")
                elif expected_intent == 'execute' and chosen_tool != 'execute_pipeline':
                    logger.warning(f"Intent mismatch: classified as 'execute' but LLM chose '{chosen_tool}'")

                # For MVP, we'll execute the first tool call
                # In production, we might need to handle multiple tool calls in sequence
                tool_call = tool_calls[0]
                result_data = self._execute_tool(job, tool_call)

            # Finalize runtime metrics
            end_time = time.time()
            runtime_metrics.finalize()

            # Create comprehensive metrics using metrics module
            metrics_dict = create_metrics(sent_at, start_time, end_time, runtime_metrics)

            # Embed metrics in result_data
            result_data["metrics"] = metrics_dict

            logger.info(f"Job {job_id} execution metrics: "
                       f"queue={metrics_dict['queue_time_ms']}ms, exec={metrics_dict['execution_time_ms']}ms, "
                       f"mem={metrics_dict['memory_start_mb']:.1f}->{metrics_dict['memory_peak_mb']:.1f}->{metrics_dict['memory_end_mb']:.1f}MB, "
                       f"cpu={metrics_dict['cpu_percent']:.1f}%, threads={metrics_dict['thread_count']}")

            # Store result in database
            result_ref = self._store_result(
                job_id=job_id,
                run_id=run_id,
                node_id=node_id,
                result_data=result_data,
                tool_calls=tool_calls,
                llm_metadata=llm_result
            )

            # Signal completion to coordinator (new architecture)
            # Send the full result_data so coordinator can store it in CAS
            completion_signal = {
                "version": "1.0",
                "job_id": job_id,
                "run_id": run_id,
                "node_id": node_id,
                "status": "completed",
                "result_data": result_data,  # Send full data, not just ref
                "metadata": {
                    "tool_calls": [tc.get('function', {}).get('name') for tc in tool_calls],
                    "tokens_used": llm_result.get('tokens_used'),
                    "cache_hit": llm_result.get('cache_hit'),
                    "execution_time_ms": llm_result.get('execution_time_ms'),
                    "llm_model": llm_result.get('model')
                }
            }
            self.redis.signal_completion(completion_signal)

            # Publish result to job-specific queue (backward compatibility)
            self.redis.publish_result(job_id, {
                "version": "1.0",
                "job_id": job_id,
                "status": "completed",
                "result_ref": result_ref,
                "result_preview": self._create_preview(result_data),
                "metadata": {
                    "tool_calls": tool_calls,
                    "tokens_used": llm_result.get('tokens_used'),
                    "cache_hit": llm_result.get('cache_hit'),
                    "execution_time_ms": llm_result.get('execution_time_ms'),
                    "llm_model": llm_result.get('model')
                }
            })

            logger.info(f"Job {job_id} completed successfully")

            # ACK message from stream
            if job.get('message_id'):
                self.redis.ack_message(job['message_id'])

        except Exception as e:
            logger.error(f"Job {job_id} failed: {e}", exc_info=True)

            # Finalize metrics even on failure
            end_time = time.time()
            runtime_metrics.finalize()

            # Create comprehensive metrics using metrics module
            failure_metrics = create_metrics(sent_at, start_time, end_time, runtime_metrics)

            # ACK message even on failure to remove from pending
            if job.get('message_id'):
                self.redis.ack_message(job['message_id'])

            # Signal failure to coordinator (new architecture)
            failure_signal = {
                "version": "1.0",
                "job_id": job_id,
                "run_id": run_id,
                "node_id": node_id,
                "status": "failed",
                "result_ref": "",  # No result on failure
                "metadata": {
                    "error_type": type(e).__name__,
                    "error_message": str(e),
                    "retryable": self._is_retryable(e),
                    "metrics": failure_metrics
                }
            }
            self.redis.signal_completion(failure_signal)

            # Publish failure result to job-specific queue (backward compatibility)
            self.redis.publish_result(job_id, {
                "version": "1.0",
                "job_id": job_id,
                "status": "failed",
                "error": {
                    "type": type(e).__name__,
                    "message": str(e),
                    "retryable": self._is_retryable(e)
                }
            })

    def _execute_tool(self, job: Dict[str, Any], tool_call: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a tool call.

        Args:
            job: Original job dictionary
            tool_call: Tool call from LLM

        Returns:
            Result data from tool execution
        """
        function = tool_call.get('function', {})
        tool_name = function.get('name')
        arguments_str = function.get('arguments')

        if not tool_name or not arguments_str:
            raise ValueError(f"Invalid tool call: {tool_call}")

        # Parse arguments
        try:
            arguments = json.loads(arguments_str)
        except json.JSONDecodeError as e:
            raise ValueError(f"Failed to parse tool arguments: {e}")

        logger.info(f"Executing tool: {tool_name}")

        # Execute tool based on name
        if tool_name == 'execute_pipeline':
            return execute_pipeline_tool(arguments)

        elif tool_name == 'patch_workflow':
            # Validate patch operations before forwarding
            patch_spec = arguments.get('patch_spec', {})
            operations = patch_spec.get('operations', [])

            try:
                validate_patch_operations(operations)
                logger.info(f"Patch validation passed: {len(operations)} operations")
            except ValueError as e:
                logger.error(f"Patch validation failed: {e}")
                # Return error instead of forwarding bad patch
                return {
                    'status': 'error',
                    'error': str(e),
                    'error_type': 'PatchValidationError',
                    'message': f"Patch rejected: {e}"
                }

            # Add workflow info from job if not in arguments
            if 'workflow_owner' not in arguments and job.get('workflow_owner'):
                arguments['workflow_owner'] = job['workflow_owner']
            # Pass run_id and node_id for run-specific patches
            return patch_workflow_tool(
                arguments,
                self.orchestrator_url,
                run_id=job.get('run_id'),
                node_id=job.get('node_id')
            )

        else:
            raise ValueError(f"Unknown tool: {tool_name}")

    def _store_result(
        self,
        job_id: str,
        run_id: str,
        node_id: str,
        result_data: Dict[str, Any],
        tool_calls: list,
        llm_metadata: Dict[str, Any]
    ) -> str:
        """Store result in memory storage.

        Args:
            job_id: Job identifier
            run_id: Run identifier
            node_id: Node identifier
            result_data: Result data to store
            tool_calls: Tool calls executed
            llm_metadata: LLM metadata (tokens, cache hit, etc.)

        Returns:
            result_ref: Reference to stored result (artifact://uuid)

        Note: Actual result_data is sent to coordinator in completion signal.
        Coordinator stores it in Redis CAS. This memory storage is for backward compatibility.
        """
        result_id = self.storage.store_result(
            job_id=job_id,
            run_id=run_id,
            node_id=node_id,
            result_data=result_data,
            status="completed",
            tool_calls=tool_calls,
            tokens_used=llm_metadata.get('tokens_used'),
            cache_hit=llm_metadata.get('cache_hit'),
            execution_time_ms=llm_metadata.get('execution_time_ms'),
            llm_model=llm_metadata.get('model')
        )

        return f"artifact://{result_id}"

    def _create_preview(self, result_data: Dict[str, Any]) -> Dict[str, Any]:
        """Create a preview of result data for the result message.

        Args:
            result_data: Full result data

        Returns:
            Preview dictionary
        """
        preview = {"type": "unknown"}

        # Check if it's a pipeline result
        if isinstance(result_data, dict):
            if result_data.get('status') == 'success':
                data = result_data.get('data')
                if isinstance(data, list):
                    preview = {
                        "type": "dataset",
                        "row_count": len(data),
                        "sample": data[:3] if data else []
                    }
                else:
                    preview = {
                        "type": "object",
                        "keys": list(data.keys()) if isinstance(data, dict) else []
                    }
            elif result_data.get('patch_id'):
                preview = {
                    "type": "patch",
                    "patch_id": result_data.get('patch_id')
                }

        return preview

    def _is_retryable(self, error: Exception) -> bool:
        """Determine if an error is retryable.

        Args:
            error: Exception that occurred

        Returns:
            True if retryable, False otherwise
        """
        # Network errors, timeouts, rate limits are retryable
        retryable_types = [
            'ConnectionError',
            'Timeout',
            'TimeoutError',
            'RateLimitError',
            'ServiceUnavailable'
        ]

        return type(error).__name__ in retryable_types

    def _run_http_server(self):
        """Run HTTP server for health checks and testing."""
        app = FastAPI(title="Agent Runner Service")

        @app.get("/health")
        def health():
            """Health check endpoint."""
            return {
                "status": "ok",
                "service": "agent-runner",
                "workers": self.num_workers,
                "running": self.running
            }

        @app.get("/metrics")
        def metrics():
            """Metrics endpoint (placeholder for now)."""
            return {
                "workers": self.num_workers,
                "status": "running" if self.running else "stopped"
            }

        @app.post("/test/chat")
        def test_chat(task: str):
            """Test chat endpoint for manual testing.

            Args:
                task: Test task to send to LLM

            Returns:
                LLM response with tool calls
            """
            if not self.running:
                raise HTTPException(status_code=503, message="Service not running")

            try:
                result = self.llm.chat(task, context={"session_id": "test"})
                return {
                    "status": "success",
                    "result": result
                }
            except Exception as e:
                logger.error(f"Test chat failed: {e}")
                raise HTTPException(status_code=500, detail=str(e))

        # Run server
        port = self.config['service'].get('port', 8082)
        uvicorn.run(app, host="0.0.0.0", port=port, log_level="info")

    def shutdown(self):
        """Gracefully shutdown the service."""
        logger.info("Shutting down agent service...")
        self.running = False

        # Wait for workers to finish current jobs (max 30 seconds)
        logger.info("Waiting for workers to finish...")
        self.worker_pool.shutdown(wait=True, timeout=30)

        # Close connections
        logger.info("Closing connections...")
        self.redis.close()
        self.storage.close()

        logger.info("Shutdown complete")


def load_config(config_path: str = 'config.yaml') -> Dict[str, Any]:
    """Load configuration from YAML file.

    Args:
        config_path: Path to config file

    Returns:
        Configuration dictionary
    """
    with open(config_path, 'r') as f:
        config = yaml.safe_load(f)

    # Replace environment variables
    def replace_env_vars(obj):
        if isinstance(obj, dict):
            return {k: replace_env_vars(v) for k, v in obj.items()}
        elif isinstance(obj, list):
            return [replace_env_vars(item) for item in obj]
        elif isinstance(obj, str) and obj.startswith('${') and obj.endswith('}'):
            env_var = obj[2:-1]
            return os.getenv(env_var, obj)
        return obj

    config = replace_env_vars(config)
    return config


def main():
    """Main entry point."""
    logger.info("=" * 60)
    logger.info("Agent Runner Service")
    logger.info("=" * 60)

    # Load config
    config_path = os.getenv('CONFIG_PATH', 'config.yaml')
    logger.info(f"Loading config from {config_path}")

    try:
        config = load_config(config_path)
    except Exception as e:
        logger.error(f"Failed to load config: {e}")
        sys.exit(1)

    # Create and start service
    service = AgentService(config)

    # Setup signal handlers for graceful shutdown
    def signal_handler(signum, frame):
        logger.info(f"Received signal {signum}")
        service.shutdown()
        sys.exit(0)

    signal.signal(signal.SIGTERM, signal_handler)
    signal.signal(signal.SIGINT, signal_handler)

    # Start service
    try:
        service.start()
    except Exception as e:
        logger.error(f"Service failed: {e}", exc_info=True)
        service.shutdown()
        sys.exit(1)


if __name__ == "__main__":
    main()
