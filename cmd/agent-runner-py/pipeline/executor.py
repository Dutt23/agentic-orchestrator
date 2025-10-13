"""Pipeline execution engine."""
from typing import Dict, Any, List
import json
import logging

from pipeline.primitives import http_request, table_ops

logger = logging.getLogger(__name__)


class PipelineExecutor:
    """Execute data pipelines composed of primitives."""

    def __init__(self):
        """Initialize executor."""
        self.primitives = {
            'http_request': http_request.execute,
            'table_sort': table_ops.execute,
            'table_filter': table_ops.execute,
            'table_select': table_ops.execute,
            'top_k': table_ops.execute
        }

    def execute(self, pipeline: List[Dict[str, Any]], input_data: Any = None) -> Any:
        """Execute pipeline steps sequentially.

        Args:
            pipeline: List of pipeline steps
            input_data: Optional initial input data

        Returns:
            Final result after all steps
        """
        if not pipeline:
            raise ValueError("Pipeline cannot be empty")

        logger.info(f"Executing pipeline with {len(pipeline)} steps")

        data = input_data

        for i, step in enumerate(pipeline):
            step_type = step.get('step')
            if not step_type:
                raise ValueError(f"Step {i} missing 'step' field")

            logger.info(f"Step {i+1}/{len(pipeline)}: {step_type}")

            if step_type not in self.primitives:
                raise ValueError(f"Unknown primitive: {step_type}")

            try:
                # Execute primitive
                primitive_func = self.primitives[step_type]
                data = primitive_func(step, data)

                logger.info(f"Step {i+1} completed successfully")

            except Exception as e:
                logger.error(f"Step {i+1} failed: {e}")
                raise ValueError(f"Pipeline failed at step {i+1} ({step_type}): {e}")

        logger.info("Pipeline execution completed")
        return data


def execute_pipeline_tool(args: Dict[str, Any]) -> Dict[str, Any]:
    """Execute execute_pipeline tool.

    Args:
        args: Tool arguments with session_id, pipeline, and optional input_ref

    Returns:
        Result dictionary with data and metadata
    """
    session_id = args.get('session_id')
    pipeline = args.get('pipeline')
    input_ref = args.get('input_ref')

    if not session_id or not pipeline:
        raise ValueError("execute_pipeline requires 'session_id' and 'pipeline'")

    logger.info(f"Executing pipeline for session {session_id}")

    # TODO: If input_ref provided, fetch data from CAS
    input_data = None

    # Execute pipeline
    executor = PipelineExecutor()
    result_data = executor.execute(pipeline, input_data)

    return {
        "status": "success",
        "data": result_data,
        "pipeline_steps": len(pipeline)
    }
