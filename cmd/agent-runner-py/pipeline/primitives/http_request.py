"""HTTP request primitive."""
import requests
from typing import Dict, Any
import logging

logger = logging.getLogger(__name__)


def execute(step: Dict[str, Any], data: Any) -> Any:
    """Execute HTTP request.

    Args:
        step: Step configuration with url, method, params
        data: Input data (ignored for http_request)

    Returns:
        Response JSON data
    """
    url = step.get('url')
    method = step.get('method', 'GET').upper()
    params = step.get('params', {})

    if not url:
        raise ValueError("http_request requires 'url' parameter")

    logger.info(f"HTTP {method} {url}")

    try:
        if method == 'GET':
            response = requests.get(url, params=params, timeout=30)
        elif method == 'POST':
            response = requests.post(url, json=params, timeout=30)
        else:
            raise ValueError(f"Unsupported HTTP method: {method}")

        response.raise_for_status()
        result = response.json()

        logger.info(f"HTTP request successful, got {len(result) if isinstance(result, list) else 1} items")
        return result

    except requests.exceptions.RequestException as e:
        logger.error(f"HTTP request failed: {e}")
        raise
