/**
 * Workflow Diff Utility
 * Compares two workflows and returns added/removed/modified nodes and edges
 */

/**
 * Compute differences between two workflows
 * @param {Object} workflowA - First workflow (base for comparison)
 * @param {Object} workflowB - Second workflow (compare against base)
 * @returns {Object} Diff result with added/removed/modified nodes and edges
 */
export function computeWorkflowDiff(workflowA, workflowB) {
  if (!workflowA || !workflowB) {
    return {
      added_nodes: [],
      removed_nodes: [],
      modified_nodes: [],
      added_edges: [],
      removed_edges: [],
      modified_edges: [],
      unchanged_nodes: [],
      unchanged_edges: []
    };
  }

  const nodesA = workflowA.nodes || [];
  const nodesB = workflowB.nodes || [];
  const edgesA = workflowA.edges || [];
  const edgesB = workflowB.edges || [];

  // Create maps for quick lookup
  const nodeMapA = new Map(nodesA.map(n => [n.id, n]));
  const nodeMapB = new Map(nodesB.map(n => [n.id, n]));

  // Compare nodes
  const added_nodes = nodesB.filter(n => !nodeMapA.has(n.id));
  const removed_nodes = nodesA.filter(n => !nodeMapB.has(n.id));

  const modified_nodes = [];
  const unchanged_nodes = [];

  nodesA.forEach(nodeA => {
    if (!nodeMapB.has(nodeA.id)) return; // Removed, already handled

    const nodeB = nodeMapB.get(nodeA.id);

    // Deep comparison of node properties
    if (isNodeModified(nodeA, nodeB)) {
      modified_nodes.push({
        id: nodeA.id,
        before: nodeA,
        after: nodeB,
        changes: getNodeChanges(nodeA, nodeB)
      });
    } else {
      unchanged_nodes.push(nodeA);
    }
  });

  // Compare edges
  const { added_edges, removed_edges, modified_edges, unchanged_edges } = compareEdges(edgesA, edgesB);

  return {
    added_nodes,
    removed_nodes,
    modified_nodes,
    unchanged_nodes,
    added_edges,
    removed_edges,
    modified_edges,
    unchanged_edges,
    summary: {
      nodes: {
        added: added_nodes.length,
        removed: removed_nodes.length,
        modified: modified_nodes.length,
        unchanged: unchanged_nodes.length,
        total_a: nodesA.length,
        total_b: nodesB.length
      },
      edges: {
        added: added_edges.length,
        removed: removed_edges.length,
        modified: modified_edges.length,
        unchanged: unchanged_edges.length,
        total_a: edgesA.length,
        total_b: edgesB.length
      }
    }
  };
}

/**
 * Check if a node has been modified
 */
function isNodeModified(nodeA, nodeB) {
  // Compare node type
  if (nodeA.type !== nodeB.type) return true;

  // Compare data/config
  const dataA = nodeA.data || {};
  const dataB = nodeB.data || {};

  // Compare critical fields
  if (dataA.type !== dataB.type) return true;
  if (dataA.label !== dataB.label) return true;

  // Deep compare config
  const configA = dataA.config || {};
  const configB = dataB.config || {};

  return JSON.stringify(configA) !== JSON.stringify(configB);
}

/**
 * Get detailed changes between two nodes
 */
function getNodeChanges(nodeA, nodeB) {
  const changes = [];

  const dataA = nodeA.data || {};
  const dataB = nodeB.data || {};

  if (dataA.type !== dataB.type) {
    changes.push({
      field: 'type',
      before: dataA.type,
      after: dataB.type
    });
  }

  if (dataA.label !== dataB.label) {
    changes.push({
      field: 'label',
      before: dataA.label,
      after: dataB.label
    });
  }

  const configA = dataA.config || {};
  const configB = dataB.config || {};

  // Compare config fields
  const allConfigKeys = new Set([...Object.keys(configA), ...Object.keys(configB)]);

  allConfigKeys.forEach(key => {
    const valueA = configA[key];
    const valueB = configB[key];

    if (JSON.stringify(valueA) !== JSON.stringify(valueB)) {
      changes.push({
        field: `config.${key}`,
        before: valueA,
        after: valueB
      });
    }
  });

  return changes;
}

/**
 * Compare edges between two workflows
 */
function compareEdges(edgesA, edgesB) {
  // Create edge keys for comparison (source + target + sourceHandle + targetHandle)
  const getEdgeKey = (edge) => {
    return `${edge.source || edge.from}::${edge.target || edge.to}::${edge.sourceHandle || ''}::${edge.targetHandle || ''}`;
  };

  const edgeMapA = new Map(edgesA.map(e => [getEdgeKey(e), e]));
  const edgeMapB = new Map(edgesB.map(e => [getEdgeKey(e), e]));

  const added_edges = edgesB.filter(e => !edgeMapA.has(getEdgeKey(e)));
  const removed_edges = edgesA.filter(e => !edgeMapB.has(getEdgeKey(e)));

  const modified_edges = [];
  const unchanged_edges = [];

  edgesA.forEach(edgeA => {
    const key = getEdgeKey(edgeA);
    if (!edgeMapB.has(key)) return; // Removed, already handled

    const edgeB = edgeMapB.get(key);

    // Compare edge properties (label, condition, type, etc.)
    if (isEdgeModified(edgeA, edgeB)) {
      modified_edges.push({
        key,
        before: edgeA,
        after: edgeB
      });
    } else {
      unchanged_edges.push(edgeA);
    }
  });

  return { added_edges, removed_edges, modified_edges, unchanged_edges };
}

/**
 * Check if an edge has been modified
 */
function isEdgeModified(edgeA, edgeB) {
  // Compare label
  if (edgeA.label !== edgeB.label) return true;

  // Compare condition
  if (edgeA.condition !== edgeB.condition) return true;

  // Compare type
  if (edgeA.type !== edgeB.type) return true;

  // Compare animated
  if (edgeA.animated !== edgeB.animated) return true;

  return false;
}

/**
 * Get human-readable summary of diff
 */
export function getDiffSummaryText(diff) {
  const { summary } = diff;
  const parts = [];

  if (summary.nodes.added > 0) {
    parts.push(`${summary.nodes.added} node${summary.nodes.added > 1 ? 's' : ''} added`);
  }

  if (summary.nodes.removed > 0) {
    parts.push(`${summary.nodes.removed} node${summary.nodes.removed > 1 ? 's' : ''} removed`);
  }

  if (summary.nodes.modified > 0) {
    parts.push(`${summary.nodes.modified} node${summary.nodes.modified > 1 ? 's' : ''} modified`);
  }

  if (summary.edges.added > 0) {
    parts.push(`${summary.edges.added} edge${summary.edges.added > 1 ? 's' : ''} added`);
  }

  if (summary.edges.removed > 0) {
    parts.push(`${summary.edges.removed} edge${summary.edges.removed > 1 ? 's' : ''} removed`);
  }

  if (summary.edges.modified > 0) {
    parts.push(`${summary.edges.modified} edge${summary.edges.modified > 1 ? 's' : ''} modified`);
  }

  if (parts.length === 0) {
    return 'No differences';
  }

  return parts.join(', ');
}

/**
 * Apply diff coloring to ReactFlow nodes
 */
export function applyDiffColorsToNodes(nodes, diff, side = 'after') {
  if (!diff) return nodes;

  return nodes.map(node => {
    let diffStatus = 'unchanged';
    let diffColor = 'gray';

    if (side === 'before') {
      // In "before" view, show what was removed or will be modified
      if (diff.removed_nodes.some(n => n.id === node.id)) {
        diffStatus = 'removed';
        diffColor = 'red';
      } else if (diff.modified_nodes.some(m => m.id === node.id)) {
        diffStatus = 'modified';
        diffColor = 'yellow';
      }
    } else {
      // In "after" view, show what was added or was modified
      if (diff.added_nodes.some(n => n.id === node.id)) {
        diffStatus = 'added';
        diffColor = 'green';
      } else if (diff.modified_nodes.some(m => m.id === node.id)) {
        diffStatus = 'modified';
        diffColor = 'yellow';
      }
    }

    return {
      ...node,
      data: {
        ...node.data,
        diffStatus,
        diffColor
      }
    };
  });
}

/**
 * Apply diff coloring to ReactFlow edges
 */
export function applyDiffColorsToEdges(edges, diff, side = 'after') {
  if (!diff) return edges;

  const getEdgeKey = (edge) => {
    return `${edge.source || edge.from}::${edge.target || edge.to}`;
  };

  return edges.map(edge => {
    let diffStatus = 'unchanged';
    let style = {};

    const edgeKey = getEdgeKey(edge);

    if (side === 'before') {
      if (diff.removed_edges.some(e => getEdgeKey(e) === edgeKey)) {
        diffStatus = 'removed';
        style = { stroke: '#ef4444', strokeWidth: 2 };
      } else if (diff.modified_edges.some(m => getEdgeKey(m.before) === edgeKey)) {
        diffStatus = 'modified';
        style = { stroke: '#eab308', strokeWidth: 2 };
      }
    } else {
      if (diff.added_edges.some(e => getEdgeKey(e) === edgeKey)) {
        diffStatus = 'added';
        style = { stroke: '#22c55e', strokeWidth: 2 };
      } else if (diff.modified_edges.some(m => getEdgeKey(m.after) === edgeKey)) {
        diffStatus = 'modified';
        style = { stroke: '#eab308', strokeWidth: 2 };
      }
    }

    return {
      ...edge,
      style: { ...edge.style, ...style },
      data: {
        ...edge.data,
        diffStatus
      }
    };
  });
}
