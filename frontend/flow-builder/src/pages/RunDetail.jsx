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
              <RunExecutionGraph
                workflowIR={details.workflow_ir}
                nodeExecutions={details.node_executions}
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
      if (status === 'completed') bgColor = '#90EE90'; // light green
      if (status === 'failed') bgColor = '#FFB6C1'; // light red
      if (status === 'running') bgColor = '#ADD8E6'; // light blue

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
          border: '2px solid #333',
          cursor: 'pointer',
        },
      };
    });

    // Convert IR edges to ReactFlow edges
    const flowEdges = [];
    Object.entries(workflowIR.nodes).forEach(([id, node]) => {
      if (node.dependents) {
        node.dependents.forEach((target) => {
          flowEdges.push({
            id: `${id}-${target}`,
            source: id,
            target: target,
            animated: nodeExecutions?.[id]?.status === 'completed',
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
