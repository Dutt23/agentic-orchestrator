import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box,
  Container,
  Heading,
  HStack,
  VStack,
  Text,
  Badge,
  Button,
  ButtonGroup,
  Spinner,
  Alert,
  AlertIcon,
  Tabs,
  TabList,
  TabPanels,
  Tab,
  TabPanel,
  Divider,
  Code,
} from '@chakra-ui/react';
import { ArrowBackIcon } from '@chakra-ui/icons';
import { getRunDetails } from '../services/api';
import { ReactFlow, Background, Controls } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import BranchDiffOverlay from '../components/BranchDiffOverlay';
import { computeWorkflowDiff, applyDiffColorsToNodes, applyDiffColorsToEdges } from '../utils/workflowDiff';

/**
 * RunDetail page displays comprehensive information about a workflow run
 */
export default function RunDetail() {
  const { runId } = useParams();
  const navigate = useNavigate();
  const [details, setDetails] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selectedNode, setSelectedNode] = useState(null);

  useEffect(() => {
    const fetchDetails = async () => {
      try {
        setLoading(true);
        const data = await getRunDetails(runId);
        setDetails(data);
      } catch (err) {
        console.error('Failed to fetch run details:', err);
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchDetails();
  }, [runId]);

  const getStatusBadge = (status) => {
    const statusLower = status?.toLowerCase() || '';
    const colorScheme =
      statusLower === 'completed'
        ? 'green'
        : statusLower === 'failed'
        ? 'red'
        : statusLower === 'running'
        ? 'blue'
        : 'gray';
    return <Badge colorScheme={colorScheme}>{status}</Badge>;
  };

  const formatDate = (dateString) => {
    if (!dateString) return 'N/A';
    return new Date(dateString).toLocaleString();
  };

  if (loading) {
    return (
      <Container maxW="container.xl" py={8}>
        <Spinner size="lg" />
        <Text mt={4}>Loading run details...</Text>
      </Container>
    );
  }

  if (error) {
    return (
      <Container maxW="container.xl" py={8}>
        <Alert status="error">
          <AlertIcon />
          Failed to load run details: {error}
        </Alert>
        <Button mt={4} leftIcon={<ArrowBackIcon />} onClick={() => navigate(-1)}>
          Go Back
        </Button>
      </Container>
    );
  }

  if (!details) {
    return (
      <Container maxW="container.xl" py={8}>
        <Alert status="warning">
          <AlertIcon />
          Run not found
        </Alert>
        <Button mt={4} leftIcon={<ArrowBackIcon />} onClick={() => navigate(-1)}>
          Go Back
        </Button>
      </Container>
    );
  }

  return (
    <Container maxW="container.xl" py={8}>
      {/* Header */}
      <VStack align="stretch" spacing={6}>
        <HStack justify="space-between">
          <HStack spacing={4}>
            <Button
              leftIcon={<ArrowBackIcon />}
              variant="ghost"
              onClick={() => navigate(-1)}
            >
              Back
            </Button>
            <Heading size="lg">Run Details</Heading>
          </HStack>
          {details.run && getStatusBadge(details.run.status)}
        </HStack>

        {/* Run Info */}
        {details.run && (
          <Box p={4} border="1px solid" borderColor="gray.200" borderRadius="md">
            <VStack align="stretch" spacing={2}>
              <HStack>
                <Text fontWeight="bold">Run ID:</Text>
                <Code fontSize="sm">{details.run.run_id}</Code>
              </HStack>
              <HStack>
                <Text fontWeight="bold">Submitted By:</Text>
                <Text>{details.run.submitted_by || 'N/A'}</Text>
              </HStack>
              <HStack>
                <Text fontWeight="bold">Submitted At:</Text>
                <Text>{formatDate(details.run.submitted_at)}</Text>
              </HStack>
              <HStack>
                <Text fontWeight="bold">Artifact ID:</Text>
                <Code fontSize="sm">{details.run.base_ref}</Code>
              </HStack>
            </VStack>
          </Box>
        )}

        <Divider />

        {/* Tabs */}
        <Tabs>
          <TabList>
            <Tab>Execution Graph</Tab>
            <Tab>Node Details</Tab>
            {details.patches && details.patches.length > 0 && <Tab>Patches</Tab>}
          </TabList>

          <TabPanels>
            {/* Execution Graph Tab */}
            <TabPanel>
              <RunExecutionGraphWithPatchOverlay
                workflowIR={details.workflow_ir}
                nodeExecutions={details.node_executions}
                patches={details.patches}
                onNodeClick={setSelectedNode}
              />
            </TabPanel>

            {/* Node Details Tab */}
            <TabPanel>
              <NodeExecutionDetails
                nodeExecutions={details.node_executions}
                selectedNode={selectedNode}
              />
            </TabPanel>

            {/* Patches Tab */}
            {details.patches && details.patches.length > 0 && (
              <TabPanel>
                <RunPatchesList patches={details.patches} />
              </TabPanel>
            )}
          </TabPanels>
        </Tabs>
      </VStack>
    </Container>
  );
}

/**
 * Wrapper component that handles patch overlay toggle
 */
function RunExecutionGraphWithPatchOverlay({ workflowIR, nodeExecutions, patches, onNodeClick }) {
  const [showPatchOverlay, setShowPatchOverlay] = useState(false);

  // Check if patches exist and are not empty
  const hasPatches = patches && patches.length > 0;

  return (
    <VStack align="stretch" spacing={4}>
      {/* Toggle Controls */}
      {hasPatches && (
        <HStack justify="flex-end">
          <ButtonGroup size="sm" isAttached variant="outline">
            <Button
              onClick={() => setShowPatchOverlay(false)}
              colorScheme={!showPatchOverlay ? 'blue' : 'gray'}
              variant={!showPatchOverlay ? 'solid' : 'outline'}
            >
              Execution View
            </Button>
            <Button
              onClick={() => setShowPatchOverlay(true)}
              colorScheme={showPatchOverlay ? 'purple' : 'gray'}
              variant={showPatchOverlay ? 'solid' : 'outline'}
            >
              Patch Overlay
            </Button>
          </ButtonGroup>
        </HStack>
      )}

      {/* Render appropriate view */}
      {showPatchOverlay && hasPatches ? (
        <RunPatchOverlay
          workflowIR={workflowIR}
          patches={patches}
        />
      ) : (
        <RunExecutionGraph
          workflowIR={workflowIR}
          nodeExecutions={nodeExecutions}
          onNodeClick={onNodeClick}
        />
      )}
    </VStack>
  );
}

/**
 * RunPatchOverlay shows before/after workflow when patches were applied
 */
function RunPatchOverlay({ workflowIR, patches }) {
  // For now, show a message explaining what will be shown once backend provides patch data
  // In the future, this will:
  // 1. Fetch base workflow (before patches)
  // 2. Compare with current workflow (after patches)
  // 3. Use BranchDiffOverlay to show differences

  if (!patches || patches.length === 0) {
    return (
      <Alert status="info">
        <AlertIcon />
        No patches were applied during this run
      </Alert>
    );
  }

  return (
    <Box>
      <Alert status="info" mb={4}>
        <AlertIcon />
        <VStack align="start" spacing={2}>
          <Text fontWeight="bold">Patch Overlay Coming Soon</Text>
          <Text fontSize="sm">
            {patches.length} patch{patches.length > 1 ? 'es' : ''} applied during this run.
            Visual diff overlay will be available when backend provides base workflow data.
          </Text>
        </VStack>
      </Alert>

      {/* Show patch list as fallback */}
      <VStack align="stretch" spacing={4}>
        {patches.map((patch, index) => (
          <Box
            key={index}
            p={4}
            border="1px solid"
            borderColor="purple.200"
            borderRadius="md"
            bg="purple.50"
          >
            <HStack justify="space-between" mb={2}>
              <Heading size="sm">Patch #{patch.seq}</Heading>
              <Badge colorScheme="purple">Applied</Badge>
            </HStack>
            <Text fontSize="sm" color="gray.700" mb={3}>
              {patch.description}
            </Text>
            <Text fontSize="xs" fontWeight="bold" mb={2}>Operations:</Text>
            <Code
              display="block"
              whiteSpace="pre"
              p={3}
              borderRadius="md"
              overflowX="auto"
              fontSize="xs"
            >
              {JSON.stringify(patch.operations, null, 2)}
            </Code>
          </Box>
        ))}
      </VStack>
    </Box>
  );
}

/**
 * RunExecutionGraph visualizes the workflow execution path
 */
function RunExecutionGraph({ workflowIR, nodeExecutions, onNodeClick }) {
  const [nodes, setNodes] = useState([]);
  const [edges, setEdges] = useState([]);

  useEffect(() => {
    if (!workflowIR || !workflowIR.nodes) return;

    // Convert IR nodes to ReactFlow nodes
    const flowNodes = Object.entries(workflowIR.nodes).map(([id, node], index) => {
      const execution = nodeExecutions?.[id];
      const status = execution?.status || 'pending';

      // Color based on status
      let bgColor = '#f0f0f0'; // pending
      let borderColor = '#333';
      if (status === 'completed') {
        bgColor = '#90EE90'; // light green
        borderColor = '#38a169'; // darker green
      }
      if (status === 'failed') {
        bgColor = '#FFB6C1'; // light red
        borderColor = '#e53e3e'; // darker red
      }
      if (status === 'running') {
        bgColor = '#ADD8E6'; // light blue
        borderColor = '#3182ce'; // darker blue
      }

      return {
        id,
        data: {
          label: (
            <div>
              <div style={{ fontWeight: 'bold' }}>{id}</div>
              <div style={{ fontSize: '10px' }}>{node.type || 'unknown'}</div>
            </div>
          ),
        },
        position: { x: (index % 3) * 250, y: Math.floor(index / 3) * 150 },
        style: {
          background: bgColor,
          padding: 10,
          borderRadius: 5,
          border: `2px solid ${borderColor}`,
          cursor: 'pointer',
        },
      };
    });

    // Convert IR edges to ReactFlow edges with execution path highlighting
    const flowEdges = [];
    Object.entries(workflowIR.nodes).forEach(([id, node]) => {
      if (node.dependents) {
        node.dependents.forEach((target) => {
          const sourceExec = nodeExecutions?.[id];
          const targetExec = nodeExecutions?.[target];

          // Determine if this edge was part of the execution path
          const sourceCompleted = sourceExec?.status === 'completed';
          const targetExecuted = targetExec?.status === 'completed' || targetExec?.status === 'failed' || targetExec?.status === 'running';
          const isExecutionPath = sourceCompleted && targetExecuted;

          // Style based on execution path
          let edgeStyle = {};
          if (isExecutionPath) {
            // Execution path: thick, bright, solid
            edgeStyle = {
              stroke: '#48bb78', // green
              strokeWidth: 4,
              opacity: 1,
            };
          } else if (sourceCompleted) {
            // Source completed but target not executed: dim, dashed (path not taken)
            edgeStyle = {
              stroke: '#cbd5e0', // gray
              strokeWidth: 2,
              strokeDasharray: '5,5',
              opacity: 0.4,
            };
          } else {
            // Neither executed: very dim
            edgeStyle = {
              stroke: '#e2e8f0', // light gray
              strokeWidth: 2,
              opacity: 0.3,
            };
          }

          flowEdges.push({
            id: `${id}-${target}`,
            source: id,
            target: target,
            animated: isExecutionPath,
            style: edgeStyle,
            data: { isExecutionPath },
          });
        });
      }
    });

    setNodes(flowNodes);
    setEdges(flowEdges);
  }, [workflowIR, nodeExecutions]);

  const handleNodeClick = (event, node) => {
    onNodeClick?.(node.id);
  };

  if (nodes.length === 0) {
    return (
      <Alert status="info">
        <AlertIcon />
        No workflow graph available (IR may have expired after 24 hours)
      </Alert>
    );
  }

  return (
    <Box position="relative">
      {/* Legend */}
      <Box
        position="absolute"
        top={2}
        left={2}
        zIndex={10}
        bg="white"
        p={3}
        borderRadius="md"
        boxShadow="md"
        border="1px solid"
        borderColor="gray.200"
      >
        <VStack align="stretch" spacing={2}>
          <Text fontSize="sm" fontWeight="bold">Legend</Text>
          <HStack spacing={2}>
            <Box w="30px" h="3px" bg="#48bb78" />
            <Text fontSize="xs">Execution Path</Text>
          </HStack>
          <HStack spacing={2}>
            <Box w="30px" h="3px" bg="#cbd5e0" style={{ borderTop: '2px dashed #cbd5e0' }} />
            <Text fontSize="xs">Path Not Taken</Text>
          </HStack>
          <HStack spacing={2}>
            <Box w="20px" h="20px" bg="#90EE90" border="2px solid #38a169" borderRadius="md" />
            <Text fontSize="xs">Completed</Text>
          </HStack>
          <HStack spacing={2}>
            <Box w="20px" h="20px" bg="#FFB6C1" border="2px solid #e53e3e" borderRadius="md" />
            <Text fontSize="xs">Failed</Text>
          </HStack>
          <HStack spacing={2}>
            <Box w="20px" h="20px" bg="#ADD8E6" border="2px solid #3182ce" borderRadius="md" />
            <Text fontSize="xs">Running</Text>
          </HStack>
        </VStack>
      </Box>

      <Box height="600px" border="1px solid" borderColor="gray.200" borderRadius="md">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodeClick={handleNodeClick}
          fitView
        >
          <Background />
          <Controls />
        </ReactFlow>
      </Box>
    </Box>
  );
}

/**
 * NodeExecutionDetails displays input/output for each node
 */
function NodeExecutionDetails({ nodeExecutions, selectedNode }) {
  if (!nodeExecutions || Object.keys(nodeExecutions).length === 0) {
    return (
      <Alert status="info">
        <AlertIcon />
        No node execution data available
      </Alert>
    );
  }

  const displayNode = selectedNode || Object.keys(nodeExecutions)[0];
  const execution = nodeExecutions[displayNode];

  if (!execution) {
    return (
      <Alert status="warning">
        <AlertIcon />
        No execution data for node: {displayNode}
      </Alert>
    );
  }

  return (
    <VStack align="stretch" spacing={4}>
      {/* Node Selector */}
      <Box>
        <Text fontWeight="bold" mb={2}>
          Select Node:
        </Text>
        <HStack spacing={2} flexWrap="wrap">
          {Object.keys(nodeExecutions).map((nodeId) => {
            const exec = nodeExecutions[nodeId];
            return (
              <Badge
                key={nodeId}
                colorScheme={
                  exec.status === 'completed'
                    ? 'green'
                    : exec.status === 'failed'
                    ? 'red'
                    : 'gray'
                }
                cursor="pointer"
                p={2}
                variant={nodeId === displayNode ? 'solid' : 'outline'}
              >
                {nodeId}
              </Badge>
            );
          })}
        </HStack>
      </Box>

      <Divider />

      {/* Execution Details */}
      <Box>
        <Heading size="md" mb={4}>
          {displayNode}
        </Heading>

        <VStack align="stretch" spacing={4}>
          <Box>
            <Text fontWeight="bold">Status:</Text>
            <Badge
              colorScheme={
                execution.status === 'completed'
                  ? 'green'
                  : execution.status === 'failed'
                  ? 'red'
                  : 'gray'
              }
            >
              {execution.status}
            </Badge>
          </Box>

          {execution.input && (
            <Box>
              <Text fontWeight="bold" mb={2}>
                Input:
              </Text>
              <Code
                display="block"
                whiteSpace="pre"
                p={4}
                borderRadius="md"
                overflowX="auto"
              >
                {JSON.stringify(execution.input, null, 2)}
              </Code>
            </Box>
          )}

          {execution.output && (
            <Box>
              <Text fontWeight="bold" mb={2}>
                Output:
              </Text>
              <Code
                display="block"
                whiteSpace="pre"
                p={4}
                borderRadius="md"
                overflowX="auto"
              >
                {JSON.stringify(execution.output, null, 2)}
              </Code>
            </Box>
          )}

          {execution.error && (
            <Box>
              <Text fontWeight="bold" mb={2}>
                Error:
              </Text>
              <Alert status="error">
                <AlertIcon />
                {execution.error}
              </Alert>
            </Box>
          )}
        </VStack>
      </Box>
    </VStack>
  );
}

/**
 * RunPatchesList displays patches applied during execution
 */
function RunPatchesList({ patches }) {
  if (!patches || patches.length === 0) {
    return (
      <Alert status="info">
        <AlertIcon />
        No patches were applied during this run
      </Alert>
    );
  }

  return (
    <VStack align="stretch" spacing={4}>
      {patches.map((patch, index) => (
        <Box
          key={index}
          p={4}
          border="1px solid"
          borderColor="gray.200"
          borderRadius="md"
        >
          <HStack justify="space-between" mb={2}>
            <Heading size="sm">Patch #{patch.seq}</Heading>
          </HStack>
          <Text fontSize="sm" color="gray.600" mb={4}>
            {patch.description}
          </Text>
          <Code
            display="block"
            whiteSpace="pre"
            p={4}
            borderRadius="md"
            overflowX="auto"
          >
            {JSON.stringify(patch.operations, null, 2)}
          </Code>
        </Box>
      ))}
    </VStack>
  );
}
