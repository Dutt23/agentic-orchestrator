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
  Tabs,
  TabList,
  TabPanels,
  Tab,
  TabPanel,
  Divider,
  Collapse,
  IconButton,
} from '@chakra-ui/react';
import { ArrowBackIcon, ChevronDownIcon, ChevronUpIcon } from '@chakra-ui/icons';
import { getRunDetails } from '../services/api';
import { ReactFlow, Background, Controls } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import BranchDiffOverlay from '../components/BranchDiffOverlay';
import PatchTimeline from '../components/PatchTimeline';
import MetricsSidebar from '../components/MetricsSidebar';
import StatusBadge from '../components/run/StatusBadge';
import NodeSelectorBadges from '../components/run/NodeSelectorBadges';
import NodeMetricsDisplay from '../components/run/NodeMetricsDisplay';
import {
  AlertMessage,
  Card,
  JsonDisplay,
  LoadingState,
  KeyValueList,
  ToggleButtons,
} from '../components/common';
import { computeWorkflowDiff, applyDiffColorsToNodes, applyDiffColorsToEdges } from '../utils/workflowDiff';
import { applyPatchesUpToSeq } from '../utils/workflowPatcher';
import { formatDate } from '../utils/dateUtils';
import { createExecutionEdge } from '../utils/workflowEdgeUtils';

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


  if (loading) {
    return <LoadingState text="Loading run details..." />;
  }

  if (error) {
    return (
      <Container maxW="container.2xl" py={8} px={8}>
        <AlertMessage status="error" message={`Failed to load run details: ${error}`} mb={4} />
        <Button leftIcon={<ArrowBackIcon />} onClick={() => navigate(-1)}>
          Go Back
        </Button>
      </Container>
    );
  }

  if (!details) {
    return (
      <Container maxW="container.2xl" py={8} px={8}>
        <AlertMessage status="warning" message="Run not found" mb={4} />
        <Button leftIcon={<ArrowBackIcon />} onClick={() => navigate(-1)}>
          Go Back
        </Button>
      </Container>
    );
  }

  return (
    <Box w="100%" display="flex" justifyContent="center" bg="gray.50" minH="100vh" position="relative">
      {/* Metrics Sidebar */}
      <MetricsSidebar nodeExecutions={details.node_executions} workflowIR={details.workflow_ir} />

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
          {details.run && <StatusBadge status={details.run.status} />}
        </HStack>

        {/* Run Info */}
        {details.run && (
          <Card>
            <KeyValueList
              items={[
                { label: 'Run ID', value: details.run.run_id, code: true },
                { label: 'Submitted By', value: details.run.submitted_by || 'N/A' },
                { label: 'Submitted At', value: formatDate(details.run.submitted_at) },
                { label: 'Artifact ID', value: details.run.base_ref, code: true },
              ]}
            />
          </Card>
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
          <ToggleButtons
            options={[
              { value: false, label: 'Execution View' },
              { value: true, label: 'Patch Overlay' },
            ]}
            value={showPatchOverlay}
            onChange={setShowPatchOverlay}
            colorScheme={showPatchOverlay ? 'purple' : 'blue'}
          />
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
    return <AlertMessage status="info" message="No patches were applied during this run" />;
  }

  // Check if baseWorkflowIR is available
  if (!baseWorkflowIR) {
    return (
      <AlertMessage
        status="warning"
        message="Base workflow data is not available. Cannot show patch overlay."
      />
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
    return <AlertMessage status="error" message={`Failed to apply patches: ${error.message}`} />;
  }

  if (!workflowAtSeq) {
    return (
      <AlertMessage
        status="error"
        message="Failed to compute workflow state at selected patch"
      />
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
          <ToggleButtons
            options={[
              { value: false, label: 'Single View' },
              { value: true, label: 'Compare Before/After' },
            ]}
            value={showComparison}
            onChange={setShowComparison}
            colorScheme={showComparison ? 'orange' : 'blue'}
          />
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
      let bgColor = '#f0f0f0'; // default
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
      if (status === 'not_executed') {
        bgColor = '#e2e8f0'; // lighter grey
        borderColor = '#a0aec0'; // grey
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
    if (workflowIR.edges && Array.isArray(workflowIR.edges) && workflowIR.edges.length > 0) {
      console.log('Found edges array in workflowIR:', workflowIR.edges);
      workflowIR.edges.forEach((edge) => {
        const sourceId = edge.from || edge.source;
        const targetId = edge.to || edge.target;

        if (sourceId && targetId) {
          flowEdges.push(createExecutionEdge(sourceId, targetId, nodeExecutions));
        }
      });
    }

    // Method 2: Check node.dependents (if edges array was empty or missing)
    if (flowEdges.length === 0) {
      console.log('No edges array found, checking node.dependents and branch rules');
      nodesArray.forEach((node) => {
        const id = node.id;
        console.log(`Node ${id}:`, { dependents: node.dependents, dependencies: node.dependencies, branch: node.branch });

        // Check regular dependents
        if (node.dependents && node.dependents.length > 0) {
          node.dependents.forEach((target) => {
            flowEdges.push(createExecutionEdge(id, target, nodeExecutions));
          });
        }

        // Check branch rules for conditional nodes
        if (node.branch && node.branch.enabled && node.branch.rules) {
          node.branch.rules.forEach((rule) => {
            if (rule.next_nodes && Array.isArray(rule.next_nodes)) {
              rule.next_nodes.forEach((target) => {
                flowEdges.push(createExecutionEdge(id, target, nodeExecutions));
              });
            }
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
              flowEdges.push(createExecutionEdge(sourceId, targetId, nodeExecutions));
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
      <AlertMessage
        status="info"
        message="No workflow graph available (IR may have expired after 24 hours)"
      />
    );
  }

  return (
    <>
      {/* Debug info */}
      {edges.length === 0 && nodes.length > 0 && (
        <AlertMessage
          status="warning"
          message={`Warning: ${nodes.length} nodes found but 0 edges. Check console for details.`}
          mb={4}
        />
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
  const [localSelectedNode, setLocalSelectedNode] = useState(null);

  if (!nodeExecutions || Object.keys(nodeExecutions).length === 0) {
    return <AlertMessage status="info" message="No node execution data available" />;
  }

  // Use local selection first, then prop selection, then first node
  const displayNode = localSelectedNode || selectedNode || Object.keys(nodeExecutions)[0];
  const execution = nodeExecutions[displayNode];

  if (!execution) {
    return (
      <AlertMessage
        status="warning"
        message={`No execution data for node: ${displayNode}`}
      />
    );
  }

  return (
    <VStack align="stretch" spacing={4}>
      {/* Node Selector */}
      <Box>
        <Text fontWeight="bold" mb={2}>
          Select Node:
        </Text>
        <NodeSelectorBadges
          nodeExecutions={nodeExecutions}
          selectedNode={displayNode}
          onNodeSelect={setLocalSelectedNode}
        />
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
            <StatusBadge status={execution.status} />
          </Box>

          {execution.input && <JsonDisplay label="Input" data={execution.input} />}

          {execution.output && <JsonDisplay label="Output" data={execution.output} />}

          {execution.error && (
            <Box>
              <Text fontWeight="bold" mb={2}>
                Error:
              </Text>
              <AlertMessage status="error" message={execution.error} />
            </Box>
          )}

          {execution.metrics && (
            <Box>
              <Text fontWeight="bold" mb={2}>
                Performance Metrics:
              </Text>
              <NodeMetricsDisplay metrics={execution.metrics} />
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
    return <AlertMessage status="info" message="No patches were applied during this run" />;
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
          <Card
            key={index}
            variant={isExpanded ? 'purple' : 'default'}
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
                <JsonDisplay
                  label="Operations"
                  data={patch.operations}
                  maxHeight="400px"
                  fontSize="sm"
                />
              </Box>
            </Collapse>
          </Card>
        );
      })}
    </VStack>
  );
}
