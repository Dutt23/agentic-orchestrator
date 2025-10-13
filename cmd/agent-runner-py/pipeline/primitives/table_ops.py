"""Table operation primitives (sort, filter, select, top_k)."""
from typing import Dict, Any, List
import logging

logger = logging.getLogger(__name__)


def table_sort(step: Dict[str, Any], data: Any) -> Any:
    """Sort records by field.

    Args:
        step: Step configuration with field and order
        data: Input data (list of dicts)

    Returns:
        Sorted data
    """
    if not isinstance(data, list):
        raise ValueError("table_sort requires list input")

    field = step.get('field')
    order = step.get('order', 'asc')

    if not field:
        raise ValueError("table_sort requires 'field' parameter")

    logger.info(f"Sorting by {field} ({order})")

    reverse = (order == 'desc')
    sorted_data = sorted(data, key=lambda x: x.get(field, 0), reverse=reverse)

    logger.info(f"Sorted {len(sorted_data)} records")
    return sorted_data


def table_filter(step: Dict[str, Any], data: Any) -> Any:
    """Filter records by condition.

    Args:
        step: Step configuration with condition
        data: Input data (list of dicts)

    Returns:
        Filtered data
    """
    if not isinstance(data, list):
        raise ValueError("table_filter requires list input")

    condition = step.get('condition')
    if not condition:
        raise ValueError("table_filter requires 'condition' parameter")

    field = condition.get('field')
    op = condition.get('op')
    value = condition.get('value')

    if not all([field, op, value is not None]):
        raise ValueError("condition requires 'field', 'op', and 'value'")

    logger.info(f"Filtering by {field} {op} {value}")

    # Define operators
    ops = {
        '<': lambda a, b: a < b,
        '>': lambda a, b: a > b,
        '<=': lambda a, b: a <= b,
        '>=': lambda a, b: a >= b,
        '==': lambda a, b: a == b,
        '!=': lambda a, b: a != b
    }

    if op not in ops:
        raise ValueError(f"Unsupported operator: {op}")

    filtered_data = [
        record for record in data
        if field in record and ops[op](record[field], value)
    ]

    logger.info(f"Filtered to {len(filtered_data)} records (from {len(data)})")
    return filtered_data


def table_select(step: Dict[str, Any], data: Any) -> Any:
    """Select specific fields from records.

    Args:
        step: Step configuration with fields
        data: Input data (list of dicts)

    Returns:
        Data with only selected fields
    """
    if not isinstance(data, list):
        raise ValueError("table_select requires list input")

    fields = step.get('fields')
    if not fields:
        raise ValueError("table_select requires 'fields' parameter")

    logger.info(f"Selecting fields: {fields}")

    selected_data = [
        {field: record.get(field) for field in fields}
        for record in data
    ]

    logger.info(f"Selected {len(fields)} fields from {len(selected_data)} records")
    return selected_data


def top_k(step: Dict[str, Any], data: Any) -> Any:
    """Take first K records.

    Args:
        step: Step configuration with k
        data: Input data (list)

    Returns:
        First K records
    """
    if not isinstance(data, list):
        raise ValueError("top_k requires list input")

    k = step.get('k')
    if not k or k < 1:
        raise ValueError("top_k requires 'k' parameter (positive integer)")

    logger.info(f"Taking top {k} records")

    result = data[:k]

    logger.info(f"Returned {len(result)} records")
    return result


def execute(step: Dict[str, Any], data: Any) -> Any:
    """Execute table operation based on step type.

    Args:
        step: Step configuration
        data: Input data

    Returns:
        Processed data
    """
    step_type = step.get('step')

    if step_type == 'table_sort':
        return table_sort(step, data)
    elif step_type == 'table_filter':
        return table_filter(step, data)
    elif step_type == 'table_select':
        return table_select(step, data)
    elif step_type == 'top_k':
        return top_k(step, data)
    else:
        raise ValueError(f"Unknown table operation: {step_type}")
