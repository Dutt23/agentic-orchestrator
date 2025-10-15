import { useEffect, useState, useCallback, useRef } from 'react';
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
import { useWebSocketEvents } from '../contexts/WebSocketContext';
import MetricsSidebar from '../components/MetricsSidebar';
import StatusBadge from '../components/run/StatusBadge';
import NodeSelectorBadges from '../components/run/NodeSelectorBadges';
import NodeMetricsDisplay from '../components/run/NodeMetricsDisplay';
import RunExecutionGraph from '../components/run/RunExecutionGraph';
import {
  AlertMessage,
  Card,
  JsonDisplay,
  LoadingState,
  KeyValueList,
  ToggleButtons,
} from '../components/common';
import { formatDate } from '../utils/dateUtils';
import { applyPatchesUpToSeq } from '../utils/workflowPatcher';
import BranchDiffOverlay from '../components/BranchDiffOverlay';
import PatchTimeline from '../components/PatchTimeline';

// Event types that should trigger a refetch (defined outside to avoid recreating on every render)
const RELEVANT_EVENT_TYPES = new Set([
  'node_completed',
  'node_failed',
  'node_started',
  'approval_required',
  'workflow_completed',
  'workflow_failed'
]);

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
  const isInitialLoad = useRef(true);

  // Fetch run details
  const fetchDetails = useCallback(async (showLoading = false) => {
    try {
      // Only show loading spinner on initial load, not on refetch
      if (showLoading) {
        setLoading(true);
      }
      const data = await getRunDetails(runId);
      setDetails(data);
      setError(null);
    } catch (err) {
      console.error('Failed to fetch run details:', err);

      // Check if it's a rate limit error
      if (err.response?.status === 429 || err.message?.includes('rate limit')) {
        setError({
          type: 'rate_limit',
          message: 'Rate limit exceeded. Please try again in a moment.',
          details: err.response?.data?.details,
          retryAfter: err.response?.data?.details?.retry_after_seconds || 60
        });
      } else {
        setError({
          type: 'error',
          message: err.message || 'Failed to load run details'
        });
      }
    } finally {
      if (showLoading) {
        setLoading(false);
      }
    }
  }, [runId]);

  // Initial fetch on mount
  useEffect(() => {
    if (isInitialLoad.current) {
      fetchDetails(true); // Show loading on initial load
      isInitialLoad.current = false;
    }
  }, [fetchDetails]);

  // WebSocket event handler - refetch details when events for this run arrive
  const handleWebSocketEvent = useCallback((event) => {
    console.log('[RunDetail] WebSocket event received:', event);
    // Refetch details to show latest status
    fetchDetails();
  }, [fetchDetails]);

  // Subscribe to WebSocket events for this specific run
  // Filter uses constant Set defined outside to avoid array recreation on every render
  useWebSocketEvents(
    handleWebSocketEvent,
    (event) => event.run_id === runId && RELEVANT_EVENT_TYPES.has(event.type)
  );


  if (loading) {
    return <LoadingState text="Loading run details..." />;
  }

  if (error) {
    // Handle different error types
    const isRateLimit = error.type === 'rate_limit';

    return (
      <Container maxW="container.2xl" py={8} px={8}>
        <VStack spacing={4} align="stretch">
          <AlertMessage
            status={isRateLimit ? 'warning' : 'error'}
            message={error.message || 'Failed to load run details'}
          />

          {isRateLimit && error.details && (
            <Card>
              <VStack align="stretch" spacing={2}>
                <Text fontSize="sm" fontWeight="bold">Rate Limit Details:</Text>
                <Text fontSize="sm">Limit: {error.details.limit} requests per {error.details.window}</Text>
                <Text fontSize="sm">Current: {error.details.current_count} requests</Text>
                <Text fontSize="sm">Retry after: {error.retryAfter} seconds</Text>
              </VStack>
            </Card>
          )}

          <HStack>
            <Button leftIcon={<ArrowBackIcon />} onClick={() => navigate(-1)}>
              Go Back
            </Button>
            {isRateLimit && (
              <Button colorScheme="blue" onClick={() => fetchDetails(true)}>
                Retry Now
              </Button>
            )}
          </HStack>
        </VStack>
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

/**
 * NodeExecutionDetails displays input/output for each node
 */
function NodeExecutionDetails({ nodeExecutions, selectedNode }) {
  const [localSelectedNode, setLocalSelectedNode] = useState(null);
  const [isApproving, setIsApproving] = useState(false);
  const { runId } = useParams();

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

  // Handle approval decision
  const handleApproval = async (approved) => {
    setIsApproving(true);
    try {
      const FANOUT_URL = import.meta.env.VITE_FANOUT_URL || 'http://localhost:8084';
      const username = import.meta.env.VITE_DEV_USERNAME || 'test-user';

      const response = await fetch(`${FANOUT_URL}/api/approval`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-User-ID': username,
        },
        body: JSON.stringify({
          run_id: runId,
          node_id: displayNode,
          approved: approved,
          comment: approved ? 'Approved from UI' : 'Rejected from UI',
        }),
      });

      if (!response.ok) {
        throw new Error(`Approval failed: ${response.statusText}`);
      }

      console.log('[Approval] Decision sent:', { approved, node_id: displayNode });
    } catch (error) {
      console.error('[Approval] Failed:', error);
      alert(`Failed to send approval: ${error.message}`);
    } finally {
      setIsApproving(false);
    }
  };

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

          {/* Approval Buttons - Show when waiting for approval */}
          {execution.status === 'waiting_for_approval' && (
            <Box>
              <Text fontWeight="bold" mb={2}>
                Approval Required:
              </Text>
              <HStack spacing={3}>
                <Button
                  colorScheme="green"
                  size="md"
                  onClick={() => handleApproval(true)}
                  isLoading={isApproving}
                  loadingText="Approving..."
                >
                  Approve
                </Button>
                <Button
                  colorScheme="red"
                  size="md"
                  onClick={() => handleApproval(false)}
                  isLoading={isApproving}
                  loadingText="Rejecting..."
                >
                  Reject
                </Button>
              </HStack>
            </Box>
          )}

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

          {execution.metrics?.system && (
            <Box>
              <Text fontWeight="bold" mb={2}>
                System Information:
              </Text>
              <Card variant="info">
                <KeyValueList
                  items={[
                    { label: 'OS', value: execution.metrics.system.os },
                    { label: 'OS Version', value: execution.metrics.system.os_version },
                    { label: 'Architecture', value: execution.metrics.system.arch },
                    { label: 'Hostname', value: execution.metrics.system.hostname, code: true },
                    { label: 'CPU Cores', value: `${execution.metrics.system.cpu_cores} physical / ${execution.metrics.system.cpu_logical} logical` },
                    { label: 'Total Memory', value: `${execution.metrics.system.total_memory_mb} MB` },
                    {
                      label: 'Container',
                      value: execution.metrics.system.in_container
                        ? `Yes (${execution.metrics.system.container_runtime || 'unknown'})`
                        : 'No',
                      badge: execution.metrics.system.in_container,
                      colorScheme: execution.metrics.system.in_container ? 'purple' : 'gray'
                    },
                    {
                      label: 'Runtime Version',
                      value: execution.metrics.system.go_version || execution.metrics.system.python_version,
                      code: true
                    },
                  ]}
                />
              </Card>
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
