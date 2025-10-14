/**
 * Utility for applying JSON patches to workflows on the client side
 * Uses fast-json-patch library for RFC 6902 JSON Patch operations
 */

import { applyPatch, deepClone } from 'fast-json-patch';

/**
 * Apply patches up to a specific sequence number
 * @param {Object} baseWorkflow - Base workflow IR before any patches
 * @param {Array} patches - Array of patch objects with {seq, operations, description, node_id}
 * @param {number} upToSeq - Apply patches from seq 1 up to and including this seq (0 = base, no patches)
 * @returns {Object} Workflow IR after applying patches
 */
export function applyPatchesUpToSeq(baseWorkflow, patches, upToSeq) {
  // If seq is 0 or no patches, return base workflow
  if (upToSeq === 0 || !patches || patches.length === 0) {
    return deepClone(baseWorkflow);
  }

  // Start with a deep clone of base workflow
  let current = deepClone(baseWorkflow);

  // Filter and sort patches up to the target seq
  const patchesToApply = patches
    .filter(p => p.seq <= upToSeq)
    .sort((a, b) => a.seq - b.seq);

  // Apply each patch sequentially
  for (const patch of patchesToApply) {
    try {
      const result = applyPatch(current, patch.operations, true, false);
      current = result.newDocument;
    } catch (error) {
      console.error(`Failed to apply patch seq ${patch.seq}:`, error);
      console.error('Patch operations:', patch.operations);
      // Continue with the current state (best effort)
    }
  }

  return current;
}

/**
 * Get workflow state at each patch sequence
 * Returns an array of {seq, workflow} objects
 * @param {Object} baseWorkflow - Base workflow IR
 * @param {Array} patches - Array of patches
 * @returns {Array} Array of {seq, workflow, patch} objects
 */
export function getAllWorkflowStates(baseWorkflow, patches) {
  const states = [];

  // Add base state (seq 0)
  states.push({
    seq: 0,
    workflow: deepClone(baseWorkflow),
    patch: null,
    label: 'Base Workflow',
  });

  // If no patches, return just the base state
  if (!patches || patches.length === 0) {
    return states;
  }

  // Sort patches by seq
  const sortedPatches = [...patches].sort((a, b) => a.seq - b.seq);

  // Generate state for each patch
  for (const patch of sortedPatches) {
    const workflow = applyPatchesUpToSeq(baseWorkflow, patches, patch.seq);
    states.push({
      seq: patch.seq,
      workflow,
      patch,
      label: `Patch #${patch.seq}${patch.node_id ? ` (${patch.node_id})` : ''}`,
    });
  }

  return states;
}

/**
 * Group patches by node_id for analytics
 * @param {Array} patches - Array of patches with node_id
 * @returns {Object} Map of node_id to array of patches
 */
export function groupPatchesByNode(patches) {
  if (!patches || patches.length === 0) {
    return {};
  }

  const grouped = {};

  for (const patch of patches) {
    const nodeId = patch.node_id || 'unknown';
    if (!grouped[nodeId]) {
      grouped[nodeId] = [];
    }
    grouped[nodeId].push(patch);
  }

  return grouped;
}

/**
 * Get patch statistics
 * @param {Array} patches - Array of patches
 * @returns {Object} Statistics object
 */
export function getPatchStats(patches) {
  if (!patches || patches.length === 0) {
    return {
      total: 0,
      byNode: {},
      totalOperations: 0,
    };
  }

  const byNode = {};
  let totalOperations = 0;

  for (const patch of patches) {
    const nodeId = patch.node_id || 'unknown';
    if (!byNode[nodeId]) {
      byNode[nodeId] = { count: 0, operations: 0 };
    }
    byNode[nodeId].count++;
    byNode[nodeId].operations += patch.operations?.length || 0;
    totalOperations += patch.operations?.length || 0;
  }

  return {
    total: patches.length,
    byNode,
    totalOperations,
  };
}
