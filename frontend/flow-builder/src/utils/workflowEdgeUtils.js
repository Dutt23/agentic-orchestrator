/**
 * Determine edge style based on execution status
 */
export function getEdgeStyle(sourceExec, targetExec) {
  const sourceCompleted =
    sourceExec?.status === 'completed' ||
    sourceExec?.status === 'skipped';
  const targetExecuted =
    targetExec?.status === 'completed' ||
    targetExec?.status === 'failed' ||
    targetExec?.status === 'running' ||
    targetExec?.status === 'skipped';
  const isExecutionPath = sourceCompleted && targetExecuted;

  let edgeStyle = {};

  if (isExecutionPath) {
    edgeStyle = {
      stroke: '#48bb78',
      strokeWidth: 5,
      opacity: 1,
    };
  } else if (sourceCompleted) {
    edgeStyle = {
      stroke: '#a0aec0',
      strokeWidth: 3,
      strokeDasharray: '5,5',
      opacity: 0.7,
    };
  } else {
    edgeStyle = {
      stroke: '#718096',
      strokeWidth: 3,
      opacity: 0.6,
    };
  }

  return { edgeStyle, isExecutionPath };
}

/**
 * Create edge from source and target with execution status
 */
export function createExecutionEdge(sourceId, targetId, nodeExecutions) {
  const sourceExec = nodeExecutions?.[sourceId];
  const targetExec = nodeExecutions?.[targetId];

  const { edgeStyle, isExecutionPath } = getEdgeStyle(sourceExec, targetExec);

  return {
    id: `${sourceId}-${targetId}`,
    source: sourceId,
    target: targetId,
    animated: isExecutionPath,
    style: edgeStyle,
    data: { isExecutionPath },
  };
}
