import { Box, HStack, VStack, Text, Badge, Flex } from '@chakra-ui/react';
import { ReactFlow, Background, Controls, MiniMap } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import WorkflowNode from './nodes/WorkflowNode';
import { applyDiffColorsToNodes, applyDiffColorsToEdges } from '../utils/workflowDiff';

const nodeTypes = {
  workflowNode: (props) => <WorkflowNode {...props} />
};

// Enhanced node component that shows diff status
function DiffWorkflowNode({ data, selected }) {
  const { diffStatus, diffColor } = data;

  // Determine border and background based on diff status
  let borderColor = 'gray.200';
  let bgColor = 'white';
  let badgeColorScheme = 'gray';
  let statusLabel = '';

  if (diffStatus === 'added') {
    borderColor = 'green.400';
    bgColor = 'green.50';
    badgeColorScheme = 'green';
    statusLabel = 'Added';
  } else if (diffStatus === 'removed') {
    borderColor = 'red.400';
    bgColor = 'red.50';
    badgeColorScheme = 'red';
    statusLabel = 'Removed';
  } else if (diffStatus === 'modified') {
    borderColor = 'yellow.400';
    bgColor = 'yellow.50';
    badgeColorScheme = 'yellow';
    statusLabel = 'Modified';
  }

  return (
    <Box
      minW="180px"
      bg={bgColor}
      borderRadius="lg"
      border="2px solid"
      borderColor={selected ? borderColor : borderColor}
      boxShadow={selected ? 'lg' : 'md'}
      opacity={diffStatus === 'removed' ? 0.6 : 1}
    >
      <WorkflowNode data={data} selected={selected} />
      {diffStatus && diffStatus !== 'unchanged' && (
        <Badge
          position="absolute"
          top={-2}
          right={-2}
          colorScheme={badgeColorScheme}
          fontSize="xs"
          textTransform="uppercase"
        >
          {statusLabel}
        </Badge>
      )}
    </Box>
  );
}

const diffNodeTypes = {
  workflowNode: DiffWorkflowNode
};

export default function BranchDiffCanvas({
  branchA,
  branchB,
  nodesA = [],
  edgesA = [],
  nodesB = [],
  edgesB = [],
  diff,
  showOnlyDifferences = false
}) {
  // Apply diff coloring to nodes and edges
  const coloredNodesA = applyDiffColorsToNodes(nodesA, diff, 'before');
  const coloredNodesB = applyDiffColorsToNodes(nodesB, diff, 'after');
  const coloredEdgesA = applyDiffColorsToEdges(edgesA, diff, 'before');
  const coloredEdgesB = applyDiffColorsToEdges(edgesB, diff, 'after');

  // Filter nodes if showing only differences
  const displayNodesA = showOnlyDifferences
    ? coloredNodesA.filter(n => n.data.diffStatus !== 'unchanged')
    : coloredNodesA;

  const displayNodesB = showOnlyDifferences
    ? coloredNodesB.filter(n => n.data.diffStatus !== 'unchanged')
    : coloredNodesB;

  // Filter edges if showing only differences
  const displayEdgesA = showOnlyDifferences
    ? coloredEdgesA.filter(e => e.data?.diffStatus !== 'unchanged')
    : coloredEdgesA;

  const displayEdgesB = showOnlyDifferences
    ? coloredEdgesB.filter(e => e.data?.diffStatus !== 'unchanged')
    : coloredEdgesB;

  return (
    <HStack spacing={0} w="100%" h="100%" align="stretch">
      {/* Left Panel - Branch A */}
      <Box flex="1" h="100%" position="relative" borderRight="2px solid" borderColor="gray.200">
        {/* Header */}
        <Flex
          position="absolute"
          top={2}
          left={2}
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
            <HStack>
              <Badge colorScheme="blue" fontSize="sm">
                {branchA}
              </Badge>
              <Text fontSize="xs" color="gray.500">
                Base
              </Text>
            </HStack>
            {diff && (
              <HStack spacing={2} fontSize="xs">
                {diff.summary.nodes.removed > 0 && (
                  <Text color="red.600" fontWeight="medium">
                    -{diff.summary.nodes.removed} nodes
                  </Text>
                )}
                {diff.summary.nodes.modified > 0 && (
                  <Text color="yellow.600" fontWeight="medium">
                    ~{diff.summary.nodes.modified} modified
                  </Text>
                )}
              </HStack>
            )}
          </VStack>
        </Flex>

        {/* ReactFlow Canvas */}
        <ReactFlow
          nodes={displayNodesA}
          edges={displayEdgesA}
          nodeTypes={diffNodeTypes}
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

      {/* Right Panel - Branch B */}
      <Box flex="1" h="100%" position="relative">
        {/* Header */}
        <Flex
          position="absolute"
          top={2}
          left={2}
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
            <HStack>
              <Badge colorScheme="purple" fontSize="sm">
                {branchB}
              </Badge>
              <Text fontSize="xs" color="gray.500">
                Compare
              </Text>
            </HStack>
            {diff && (
              <HStack spacing={2} fontSize="xs">
                {diff.summary.nodes.added > 0 && (
                  <Text color="green.600" fontWeight="medium">
                    +{diff.summary.nodes.added} nodes
                  </Text>
                )}
                {diff.summary.nodes.modified > 0 && (
                  <Text color="yellow.600" fontWeight="medium">
                    ~{diff.summary.nodes.modified} modified
                  </Text>
                )}
              </HStack>
            )}
          </VStack>
        </Flex>

        {/* ReactFlow Canvas */}
        <ReactFlow
          nodes={displayNodesB}
          edges={displayEdgesB}
          nodeTypes={diffNodeTypes}
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
    </HStack>
  );
}
