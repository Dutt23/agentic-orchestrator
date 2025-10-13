import { Box, VStack, HStack, Text, Badge, Button, ButtonGroup } from '@chakra-ui/react';
import { ReactFlow, Background, Controls, MiniMap } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import WorkflowNode from './nodes/WorkflowNode';
import { applyDiffColorsToNodes } from '../utils/workflowDiff';

const nodeTypes = {
  workflowNode: (props) => <WorkflowNode {...props} />
};

// Colors for different node statuses
const PATH_COLORS = {
  base: '#3182ce', // blue
  compare: '#9f7aea', // purple
  added: '#38a169', // green
  removed: '#e53e3e', // red
  modified: '#d69e2e' // yellow
};

// Enhanced node component for overlay mode
function OverlayWorkflowNode({ data, selected }) {
  const { diffStatus, isBaseNode } = data;

  let borderColor = 'gray.300';
  let bgColor = 'white';
  let opacity = 1;
  let borderStyle = 'solid';

  // Color based on diff status
  if (diffStatus === 'unchanged') {
    // Unchanged nodes - neutral, solid
    borderColor = 'blue.300';
    borderStyle = 'solid';
    opacity = 1;
  } else if (diffStatus === 'removed') {
    // Removed nodes - red, dimmed, dashed
    borderColor = 'red.400';
    bgColor = 'red.50';
    borderStyle = 'dashed';
    opacity = 0.5;
  } else if (diffStatus === 'modified') {
    // Modified nodes - yellow, solid
    borderColor = 'yellow.400';
    bgColor = 'yellow.50';
    borderStyle = 'solid';
    opacity = 1;
  } else if (diffStatus === 'added') {
    // Added nodes - green, dashed
    borderColor = 'green.400';
    bgColor = 'green.50';
    borderStyle = 'dashed';
    opacity = 0.85;
  }

  return (
    <Box
      minW="180px"
      bg={bgColor}
      borderRadius="lg"
      border="2px"
      borderStyle={borderStyle}
      borderColor={borderColor}
      boxShadow={selected ? 'lg' : 'md'}
      opacity={opacity}
      position="relative"
    >
      <WorkflowNode data={data} selected={selected} />
      {diffStatus && diffStatus !== 'unchanged' && (
        <Badge
          position="absolute"
          top={-2}
          right={-2}
          colorScheme={
            diffStatus === 'added' ? 'green' :
            diffStatus === 'removed' ? 'red' :
            'yellow'
          }
          fontSize="xs"
        >
          {diffStatus}
        </Badge>
      )}
    </Box>
  );
}

const overlayNodeTypes = {
  workflowNode: OverlayWorkflowNode
};

export default function BranchDiffOverlay({
  branchA,
  branchB,
  nodesA = [],
  edgesA = [],
  nodesB = [],
  edgesB = [],
  diff
}) {
  // Intelligently merge nodes - don't duplicate unchanged nodes
  const mergedNodes = [];
  const nodeMapA = new Map(nodesA.map(n => [n.id, n]));
  const nodeMapB = new Map(nodesB.map(n => [n.id, n]));

  // First, add all nodes from branch A (base)
  nodesA.forEach(nodeA => {
    const diffStatus = nodeA.data?.diffStatus || 'unchanged';

    if (diffStatus === 'removed') {
      // Show removed nodes with reduced opacity
      mergedNodes.push({
        ...nodeA,
        data: {
          ...nodeA.data,
          isBaseNode: true,
          diffStatus: 'removed'
        }
      });
    } else if (diffStatus === 'modified') {
      // Show modified nodes (base version)
      mergedNodes.push({
        ...nodeA,
        data: {
          ...nodeA.data,
          isBaseNode: true,
          diffStatus: 'modified'
        }
      });
    } else {
      // Unchanged nodes - show once
      mergedNodes.push({
        ...nodeA,
        data: {
          ...nodeA.data,
          isBaseNode: true,
          diffStatus: 'unchanged'
        }
      });
    }
  });

  // Then, add only the ADDED nodes from branch B
  // Calculate positions to avoid overlap with base nodes
  const baseNodePositions = new Set(
    nodesA.map(n => `${Math.floor(n.position.x / 50)},${Math.floor(n.position.y / 50)}`)
  );

  nodesB.forEach(nodeB => {
    const diffStatus = nodeB.data?.diffStatus || 'unchanged';

    if (diffStatus === 'added') {
      // Offset added nodes to avoid overlapping with base nodes
      let newPosition = { ...nodeB.position };
      let offsetY = 150; // Initial vertical offset
      let attempts = 0;

      // Keep offsetting until we find a non-overlapping position
      while (attempts < 10) {
        const posKey = `${Math.floor(newPosition.x / 50)},${Math.floor(newPosition.y / 50)}`;
        if (!baseNodePositions.has(posKey)) {
          break;
        }
        newPosition.y += offsetY;
        attempts++;
      }

      // Only add nodes that are actually new
      mergedNodes.push({
        ...nodeB,
        position: newPosition,
        data: {
          ...nodeB.data,
          isBaseNode: false,
          diffStatus: 'added'
        }
      });
    }
  });

  // Intelligently merge edges
  const mergedEdges = [];
  const edgeMapA = new Map(edgesA.map(e => [`${e.source}-${e.target}`, e]));
  const edgeMapB = new Map(edgesB.map(e => [`${e.source}-${e.target}`, e]));

  // Add all edges from branch A
  edgesA.forEach(edgeA => {
    const key = `${edgeA.source}-${edgeA.target}`;
    const existsInB = edgeMapB.has(key);
    const diffStatus = edgeA.data?.diffStatus || 'unchanged';

    if (diffStatus === 'removed' || !existsInB) {
      // Removed edge - show dimmed
      mergedEdges.push({
        ...edgeA,
        style: {
          stroke: PATH_COLORS.removed,
          strokeWidth: 2,
          strokeDasharray: '5 5',
          opacity: 0.4
        },
        data: { ...edgeA.data, diffStatus: 'removed' },
        zIndex: 1
      });
    } else {
      // Unchanged edge - show solid
      mergedEdges.push({
        ...edgeA,
        style: {
          stroke: PATH_COLORS.base,
          strokeWidth: 2,
          strokeDasharray: '0'
        },
        data: { ...edgeA.data, diffStatus: 'unchanged' },
        zIndex: 1
      });
    }
  });

  // Add only NEW edges from branch B (added edges)
  edgesB.forEach(edgeB => {
    const key = `${edgeB.source}-${edgeB.target}`;
    const existsInA = edgeMapA.has(key);

    if (!existsInA) {
      // Added edge - show dashed in green
      mergedEdges.push({
        ...edgeB,
        style: {
          stroke: PATH_COLORS.added,
          strokeWidth: 2,
          strokeDasharray: '5 5',
          opacity: 0.8
        },
        animated: true,
        data: { ...edgeB.data, diffStatus: 'added' },
        zIndex: 2
      });
    }
  });

  const allNodes = mergedNodes;
  const allEdges = mergedEdges;

  return (
    <Box h="100%" w="100%" position="relative">
      {/* Legend */}
      <Box
        position="absolute"
        top={2}
        left={2}
        zIndex={10}
        bg="white"
        p={3}
        borderRadius="md"
        boxShadow="lg"
        border="1px solid"
        borderColor="gray.200"
      >
        <VStack align="stretch" spacing={2}>
          <Text fontSize="sm" fontWeight="bold" mb={1}>
            Overlay Legend
          </Text>

          {/* Base Branch */}
          <HStack spacing={2}>
            <Box
              w="20px"
              h="2px"
              bg={PATH_COLORS.base}
              borderRadius="sm"
            />
            <Badge colorScheme="blue" variant="subtle" fontSize="xs">
              {branchA} (Base)
            </Badge>
          </HStack>

          {/* Compare Branch */}
          <HStack spacing={2}>
            <Box
              w="20px"
              h="2px"
              bg={PATH_COLORS.compare}
              borderRadius="sm"
              style={{ borderTop: '2px dashed', borderColor: PATH_COLORS.compare }}
            />
            <Badge colorScheme="purple" variant="subtle" fontSize="xs">
              {branchB} (Compare)
            </Badge>
          </HStack>

          {/* Changes */}
          {diff && diff.summary.nodes.added > 0 && (
            <HStack spacing={2}>
              <Box w="20px" h="2px" bg={PATH_COLORS.added} borderRadius="sm" />
              <Text fontSize="xs" color="green.600">
                Added ({diff.summary.nodes.added})
              </Text>
            </HStack>
          )}

          {diff && diff.summary.nodes.removed > 0 && (
            <HStack spacing={2}>
              <Box w="20px" h="2px" bg={PATH_COLORS.removed} borderRadius="sm" />
              <Text fontSize="xs" color="red.600">
                Removed ({diff.summary.nodes.removed})
              </Text>
            </HStack>
          )}

          {diff && diff.summary.nodes.modified > 0 && (
            <HStack spacing={2}>
              <Box w="20px" h="2px" bg={PATH_COLORS.modified} borderRadius="sm" />
              <Text fontSize="xs" color="yellow.600">
                Modified ({diff.summary.nodes.modified})
              </Text>
            </HStack>
          )}
        </VStack>
      </Box>

      {/* Branch Info */}
      <Box
        position="absolute"
        top={2}
        right={2}
        zIndex={10}
        bg="white"
        px={3}
        py={2}
        borderRadius="md"
        boxShadow="md"
        border="1px solid"
        borderColor="gray.200"
      >
        <VStack align="start" spacing={1}>
          <Text fontSize="xs" fontWeight="medium" color="gray.600">
            Overlay View
          </Text>
          <HStack spacing={2}>
            <Badge colorScheme="blue" fontSize="xs">
              {branchA}
            </Badge>
            <Text fontSize="xs" color="gray.400">vs</Text>
            <Badge colorScheme="purple" fontSize="xs">
              {branchB}
            </Badge>
          </HStack>
        </VStack>
      </Box>

      {/* ReactFlow Canvas */}
      <ReactFlow
        nodes={allNodes}
        edges={allEdges}
        nodeTypes={overlayNodeTypes}
        fitView
        minZoom={0.1}
        maxZoom={2}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </Box>
  );
}
