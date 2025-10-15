import { useEffect, useState, useCallback } from 'react';
import { Box, HStack, VStack, Text, Collapse, IconButton } from '@chakra-ui/react';
import { ChevronDownIcon, ChevronUpIcon } from '@chakra-ui/icons';
import { ReactFlow, Background, Controls, useNodesState, useEdgesState } from '@xyflow/react';
import { createExecutionEdge } from '../../utils/workflowEdgeUtils';
import { AlertMessage } from '../common';

// Node status color configuration (centralized styling)
const STATUS_COLORS = {
  completed: { bg: '#90EE90', border: '#38a169' },
  failed: { bg: '#FFB6C1', border: '#e53e3e' },
  error: { bg: '#FFB6C1', border: '#e53e3e' }, // Treat error same as failed
  running: { bg: '#ADD8E6', border: '#3182ce' },
  waiting_for_approval: { bg: '#FFF9C4', border: '#F59E0B' },
  skipped: { bg: '#E2E8F0', border: '#718096' },
  not_executed: { bg: '#e2e8f0', border: '#a0aec0' },
  default: { bg: '#f0f0f0', border: '#333' }
};

// Get colors for a given status
const getNodeColors = (status) => STATUS_COLORS[status] || STATUS_COLORS.default;

export default function RunExecutionGraph({ workflowIR, nodeExecutions, onNodeClick }) {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges] = useEdgesState([]);
  const [showLegend, setShowLegend] = useState(true);

  useEffect(() => {
    if (!workflowIR || !workflowIR.nodes) {
      return;
    }

    // Convert IR nodes to ReactFlow nodes
    let nodesArray = Array.isArray(workflowIR.nodes)
      ? workflowIR.nodes
      : Object.entries(workflowIR.nodes).map(([id, node]) => ({ ...node, id }));

    const flowNodes = nodesArray.map((node, index) => {
      const nodeId = node.id;
      const execution = nodeExecutions?.[nodeId];
      const status = execution?.status || 'pending';
      const colors = getNodeColors(status);

      return {
        id: nodeId,
        data: {
          label: (
            <div>
              <div style={{ fontWeight: 'bold' }}>{nodeId}</div>
              <div style={{ fontSize: '10px' }}>{node.type || 'unknown'}</div>
            </div>
          ),
        },
        position: { x: (index % 3) * 250, y: Math.floor(index / 3) * 150 },
        style: {
          background: colors.bg,
          padding: 10,
          borderRadius: 5,
          border: `2px solid ${colors.border}`,
          cursor: 'pointer',
        },
      };
    });

    // Build edges from dependents
    const flowEdges = [];
    nodesArray.forEach((node) => {
      if (node.dependents?.length > 0) {
        node.dependents.forEach((target) => {
          flowEdges.push(createExecutionEdge(node.id, target, nodeExecutions));
        });
      }
      if (node.branch?.enabled && node.branch.rules) {
        node.branch.rules.forEach((rule) => {
          rule.next_nodes?.forEach((target) => {
            flowEdges.push(createExecutionEdge(node.id, target, nodeExecutions));
          });
        });
      }
    });

    setNodes(flowNodes);
    setEdges(flowEdges);
  }, [workflowIR, nodeExecutions, setNodes, setEdges]);

  const handleNodeClick = (event, node) => {
    onNodeClick?.(node.id);
  };

  if (nodes.length === 0) {
    return <AlertMessage status="info" message="No workflow graph available" />;
  }

  return (
    <Box position="relative">
      {/* Legend */}
      <Box position="absolute" top={2} left={2} zIndex={10} bg="white" borderRadius="md" boxShadow="md" border="1px solid" borderColor="gray.200">
        <HStack justify="space-between" p={2} cursor="pointer" onClick={() => setShowLegend(!showLegend)} _hover={{ bg: 'gray.50' }}>
          <Text fontSize="sm" fontWeight="bold">Legend</Text>
          <IconButton icon={showLegend ? <ChevronUpIcon /> : <ChevronDownIcon />} size="xs" variant="ghost" aria-label="Toggle legend" />
        </HStack>
        <Collapse in={showLegend} animateOpacity>
          <VStack align="stretch" spacing={2} p={3} pt={0}>
            <HStack spacing={2}><Box w="30px" h="4px" bg="#48bb78" /><Text fontSize="xs">Execution Path</Text></HStack>
            <HStack spacing={2}><Box w="20px" h="20px" bg="#90EE90" border="2px solid #38a169" borderRadius="md" /><Text fontSize="xs">Completed</Text></HStack>
            <HStack spacing={2}><Box w="20px" h="20px" bg="#FFB6C1" border="2px solid #e53e3e" borderRadius="md" /><Text fontSize="xs">Failed</Text></HStack>
            <HStack spacing={2}><Box w="20px" h="20px" bg="#E2E8F0" border="2px solid #718096" borderRadius="md" /><Text fontSize="xs">Skipped</Text></HStack>
          </VStack>
        </Collapse>
      </Box>

      <Box height="600px" border="1px solid" borderColor="gray.200" borderRadius="md" bg="white">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onNodeClick={handleNodeClick}
          nodesDraggable={true}
          nodesConnectable={false}
          fitView
        >
          <Background />
          <Controls />
        </ReactFlow>
      </Box>
    </Box>
  );
}
