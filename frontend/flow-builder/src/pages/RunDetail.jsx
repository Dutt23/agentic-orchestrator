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
  Collapse,
  IconButton,
} from '@chakra-ui/react';
import { ArrowBackIcon, ChevronDownIcon, ChevronUpIcon } from '@chakra-ui/icons';
import { getRunDetails } from '../services/api';
import { ReactFlow, Background, Controls } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import BranchDiffOverlay from '../components/BranchDiffOverlay';
import PatchTimeline from '../components/PatchTimeline';
import { computeWorkflowDiff, applyDiffColorsToNodes, applyDiffColorsToEdges } from '../utils/workflowDiff';
import { applyPatchesUpToSeq } from '../utils/workflowPatcher';

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
      <Container maxW="container.2xl" py={8} px={8}>
        <Spinner size="lg" />
        <Text mt={4}>Loading run details...</Text>
      </Container>
    );
  }

  if (error) {
    return (
      <Container maxW="container.2xl" py={8} px={8}>
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
      <Container maxW="container.2xl" py={8} px={8}>
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
    <Box w="100%" display="flex" justifyContent="center" bg="gray.50" minH="100vh">
      <Container maxW="container.2xl" py={8} px={8}>
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
                baseWorkflowIR={details.base_workflow_ir}
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
    </Box>
  );
}

/**
 * Wrapper component that handles patch overlay toggle
 */
function RunExecutionGraphWithPatchOverlay({ baseWorkflowIR, workflowIR, nodeExecutions, patches, onNodeClick }) {
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
          baseWorkflowIR={baseWorkflowIR}
          workflowIR={workflowIR}
          patches={patches}
          nodeExecutions={nodeExecutions}
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
 * RunPatchOverlay shows workflow evolution through patch timeline
 * Allows users to scrub through different patch states
 */
function RunPatchOverlay({ baseWorkflowIR, workflowIR, patches, nodeExecutions }) {
  const [selectedSeq, setSelectedSeq] = useState(patches && patches.length > 0 ? patches[patches.length - 1].seq : 0);
  const [showComparison, setShowComparison] = useState(false);

  if (!patches || patches.length === 0) {
    return (
      <Alert status="info">
        <AlertIcon />
        No patches were applied during this run
      </Alert>
    );
  }

  // Check if baseWorkflowIR is available
  if (!baseWorkflowIR) {
    return (
      <Alert status="warning">
        <AlertIcon />
        Base workflow data is not available. Cannot show patch overlay.
      </Alert>
    );
  }

  // Compute workflow at selected seq using client-side patching
  let workflowAtSeq;
  let workflowAtPrevSeq;

  try {
    workflowAtSeq = applyPatchesUpToSeq(baseWorkflowIR, patches, selectedSeq);

    // Compute previous seq for comparison
    const prevSeq = selectedSeq > 0 ? selectedSeq - 1 : 0;
    workflowAtPrevSeq = applyPatchesUpToSeq(baseWorkflowIR, patches, prevSeq);
  } catch (error) {
    console.error('Error applying patches:', error);
    return (
      <Alert status="error">
        <AlertIcon />
        Failed to apply patches: {error.message}
      </Alert>
    );
  }

  if (!workflowAtSeq) {
    return (
      <Alert status="error">
        <AlertIcon />
        Failed to compute workflow state at selected patch
      </Alert>
    );
  }

  return (
    <VStack align="stretch" spacing={6}>
      {/* Patch Timeline Selector */}
      <PatchTimeline
        patches={patches}
        selectedSeq={selectedSeq}
        onSeqChange={setSelectedSeq}
      />

      {/* Toggle: Single View vs Comparison */}
      {selectedSeq > 0 && (
        <HStack justify="flex-end">
          <ButtonGroup size="sm" isAttached variant="outline">
            <Button
              onClick={() => setShowComparison(false)}
              colorScheme={!showComparison ? 'blue' : 'gray'}
              variant={!showComparison ? 'solid' : 'outline'}
            >
              Single View
            </Button>
            <Button
              onClick={() => setShowComparison(true)}
              colorScheme={showComparison ? 'orange' : 'gray'}
              variant={showComparison ? 'solid' : 'outline'}
            >
              Compare Before/After
            </Button>
          </ButtonGroup>
        </HStack>
      )}

      {/* Workflow Visualization */}
      {showComparison && selectedSeq > 0 ? (
        <BranchDiffOverlay
          baseWorkflow={workflowAtPrevSeq}
          branchWorkflow={workflowAtSeq}
        />
      ) : (
        <RunExecutionGraph
          workflowIR={workflowAtSeq}
          nodeExecutions={nodeExecutions}
          onNodeClick={() => {}}
        />
      )}
    </VStack>
  );
}

/**
 * RunExecutionGraph visualizes the workflow execution path
 */
function RunExecutionGraph({ workflowIR, nodeExecutions, onNodeClick }) {
  const [nodes, setNodes] = useState([]);
  const [edges, setEdges] = useState([]);
  const [showLegend, setShowLegend] = useState(true);

  useEffect(() => {
    if (!workflowIR || !workflowIR.nodes) {
      console.log('No workflow IR or nodes available');
      return;
    }

    console.log('WorkflowIR:', workflowIR);
    console.log('NodeExecutions:', nodeExecutions);
    console.log('workflowIR.nodes type:', Array.isArray(workflowIR.nodes) ? 'Array' : 'Object');

    // Convert IR nodes to ReactFlow nodes
    // Handle both array and object formats
    let nodesArray;
    if (Array.isArray(workflowIR.nodes)) {
      // If nodes is an array, use it directly
      nodesArray = workflowIR.nodes;
      console.log('Nodes is an array, using directly');
    } else {
      // If nodes is an object, convert to array of [id, node] pairs
      nodesArray = Object.entries(workflowIR.nodes).map(([id, node]) => ({ ...node, id }));
      console.log('Nodes is an object, converted to array');
    }

    const flowNodes = nodesArray.map((node, index) => {
      const nodeId = node.id;  // Get ID from node object
      const execution = nodeExecutions?.[nodeId];
      const status = execution?.status || 'pending';

      console.log(`Node ${nodeId}: status=${status}, execution=`, execution);

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
        id: nodeId, // Use the actual node ID, not the index
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

    // Method 1: Check for edges array at the top level
    if (workflowIR.edges && Array.isArray(workflowIR.edges)) {
      console.log('Found edges array in workflowIR:', workflowIR.edges);
      workflowIR.edges.forEach((edge) => {
        const sourceId = edge.from || edge.source;
        const targetId = edge.to || edge.target;

        if (sourceId && targetId) {
          const sourceExec = nodeExecutions?.[sourceId];
          const targetExec = nodeExecutions?.[targetId];

          const sourceCompleted = sourceExec?.status === 'completed';
          const targetExecuted = targetExec?.status === 'completed' || targetExec?.status === 'failed' || targetExec?.status === 'running';
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

          flowEdges.push({
            id: `${sourceId}-${targetId}`,
            source: sourceId,
            target: targetId,
            animated: isExecutionPath,
            style: edgeStyle,
            data: { isExecutionPath },
          });
        }
      });
    }
    // Method 2: Check node.dependents
    else {
      console.log('No edges array found, checking node.dependents');
      nodesArray.forEach((node) => {
        const id = node.id;
        console.log(`Node ${id}:`, { dependents: node.dependents, dependencies: node.dependencies });

        if (node.dependents && node.dependents.length > 0) {
          node.dependents.forEach((target) => {
            const sourceExec = nodeExecutions?.[id];
            const targetExec = nodeExecutions?.[target];

            const sourceCompleted = sourceExec?.status === 'completed';
            const targetExecuted = targetExec?.status === 'completed' || targetExec?.status === 'failed' || targetExec?.status === 'running';
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

      // Method 3: If still no edges, create them from dependencies (reverse direction)
      if (flowEdges.length === 0) {
        console.log('Still no edges, trying to create from dependencies');
        nodesArray.forEach((node) => {
          const targetId = node.id;
          if (node.dependencies && Array.isArray(node.dependencies) && node.dependencies.length > 0) {
            console.log(`Node ${targetId} has dependencies:`, node.dependencies);
            node.dependencies.forEach((sourceId) => {
              const sourceExec = nodeExecutions?.[sourceId];
              const targetExec = nodeExecutions?.[targetId];

              const sourceCompleted = sourceExec?.status === 'completed';
              const targetExecuted = targetExec?.status === 'completed' || targetExec?.status === 'failed' || targetExec?.status === 'running';
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

              flowEdges.push({
                id: `${sourceId}-${targetId}`,
                source: sourceId,
                target: targetId,
                animated: isExecutionPath,
                style: edgeStyle,
                data: { isExecutionPath },
              });
            });
          }
        });
      }
    }

    console.log('Generated flowNodes:', flowNodes.length, flowNodes);
    console.log('Generated flowEdges:', flowEdges.length, flowEdges);

    // Debug: Check if node IDs match edge source/targets
    const nodeIds = new Set(flowNodes.map(n => n.id));
    console.log('Node IDs:', Array.from(nodeIds));

    flowEdges.forEach(edge => {
      const sourceExists = nodeIds.has(edge.source);
      const targetExists = nodeIds.has(edge.target);
      console.log(`Edge ${edge.id}: source="${edge.source}" (exists: ${sourceExists}), target="${edge.target}" (exists: ${targetExists})`);
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
    <>
      {/* Debug info */}
      {edges.length === 0 && nodes.length > 0 && (
        <Alert status="warning" mb={4}>
          <AlertIcon />
          Warning: {nodes.length} nodes found but 0 edges. Check console for details.
        </Alert>
      )}

      <Box position="relative">
      {/* Legend */}
      <Box
        position="absolute"
        top={2}
        left={2}
        zIndex={10}
        bg="white"
        borderRadius="md"
        boxShadow="md"
        border="1px solid"
        borderColor="gray.200"
      >
        <HStack
          justify="space-between"
          p={2}
          cursor="pointer"
          onClick={() => setShowLegend(!showLegend)}
          _hover={{ bg: 'gray.50' }}
        >
          <Text fontSize="sm" fontWeight="bold">Legend</Text>
          <IconButton
            icon={showLegend ? <ChevronUpIcon /> : <ChevronDownIcon />}
            size="xs"
            variant="ghost"
            aria-label={showLegend ? 'Collapse legend' : 'Expand legend'}
          />
        </HStack>
        <Collapse in={showLegend} animateOpacity>
          <VStack align="stretch" spacing={2} p={3} pt={0}>
            <HStack spacing={2}>
              <Box w="30px" h="4px" bg="#48bb78" />
              <Text fontSize="xs">Execution Path</Text>
            </HStack>
            <HStack spacing={2}>
              <Box w="30px" h="3px" bg="#a0aec0" style={{ borderTop: '3px dashed #a0aec0' }} />
              <Text fontSize="xs">Path Not Taken</Text>
            </HStack>
            <HStack spacing={2}>
              <Box w="30px" h="3px" bg="#718096" />
              <Text fontSize="xs">Default Edge</Text>
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
            <HStack spacing={2}>
              <Box w="20px" h="20px" bg="#f0f0f0" border="2px solid #333" borderRadius="md" />
              <Text fontSize="xs">Pending</Text>
            </HStack>
          </VStack>
        </Collapse>
      </Box>

      <Box height="600px" border="1px solid" borderColor="gray.200" borderRadius="md" bg="white">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodeClick={handleNodeClick}
          fitView
          attributionPosition="bottom-left"
        >
          <Background />
          <Controls />
        </ReactFlow>
      </Box>
      </Box>
    </>
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
  const [expandedPatches, setExpandedPatches] = useState({});

  if (!patches || patches.length === 0) {
    return (
      <Alert status="info">
        <AlertIcon />
        No patches were applied during this run
      </Alert>
    );
  }

  const togglePatch = (index) => {
    setExpandedPatches(prev => ({
      ...prev,
      [index]: !prev[index]
    }));
  };

  return (
    <VStack align="stretch" spacing={4}>
      {patches.map((patch, index) => {
        const isExpanded = expandedPatches[index] || false;
        const operationsCount = patch.operations?.length || 0;

        return (
          <Box
            key={index}
            p={4}
            border="1px solid"
            borderColor="gray.200"
            borderRadius="md"
            bg={isExpanded ? 'purple.50' : 'white'}
            transition="background 0.2s"
          >
            <HStack justify="space-between" mb={2}>
              <HStack spacing={3}>
                <Heading size="sm">Patch #{patch.seq}</Heading>
                {patch.node_id && (
                  <Badge colorScheme="purple" fontSize="xs">
                    {patch.node_id}
                  </Badge>
                )}
                <Badge colorScheme="gray" fontSize="xs">
                  {operationsCount} operation{operationsCount !== 1 ? 's' : ''}
                </Badge>
              </HStack>
              <IconButton
                icon={isExpanded ? <ChevronUpIcon /> : <ChevronDownIcon />}
                onClick={() => togglePatch(index)}
                size="sm"
                variant="ghost"
                colorScheme="purple"
                aria-label={isExpanded ? 'Collapse operations' : 'Expand operations'}
              />
            </HStack>

            <Text fontSize="sm" color="gray.600" mb={2}>
              {patch.description || 'No description provided'}
            </Text>

            <Collapse in={isExpanded} animateOpacity>
              <Box mt={4}>
                <Text fontSize="xs" fontWeight="bold" color="gray.700" mb={2}>
                  Operations:
                </Text>
                <Code
                  display="block"
                  whiteSpace="pre"
                  p={4}
                  borderRadius="md"
                  overflowX="auto"
                  maxH="400px"
                  overflowY="auto"
                  fontSize="sm"
                >
                  {JSON.stringify(patch.operations, null, 2)}
                </Code>
              </Box>
            </Collapse>
          </Box>
        );
      })}
    </VStack>
  );
}
