import React, { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Flex,
  Box,
  useToast,
  IconButton,
  useMediaQuery,
  Modal,
  ModalOverlay,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalCloseButton,
  Button,
  HStack,
} from '@chakra-ui/react';
import { ChevronLeftIcon, ChevronRightIcon } from '@chakra-ui/icons';
import { FiArrowLeft } from 'react-icons/fi';
import { applyNodeChanges, applyEdgeChanges, addEdge } from '@xyflow/react';
import FlowCanvas from '../components/flow/FlowCanvas';
import NodesPanel from '../components/NodesPanel';
import Header from '../components/ui/header/Header';
import BranchComparison from '../components/BranchComparison';
import BranchDiffCanvas from '../components/BranchDiffCanvas';
import BranchDiffOverlay from '../components/BranchDiffOverlay';
import ExecutionDrawer from '../components/workflow/ExecutionDrawer';
import { LoadingState } from '../components/common';
import { mockWorkflows, getAllWorkflows, getBranches, getLatestVersion } from '../data/mockWorkflows';
import { applyDiffColorsToNodes, applyDiffColorsToEdges, computeWorkflowDiff } from '../utils/workflowDiff';
import { getWorkflow, getWorkflowVersion, updateWorkflow, runWorkflow } from '../services/api';
import { useWorkflowWebSocket } from '../hooks/useWorkflowWebSocket';

// Function to validate the flow
const validateFlow = (nodes, edges) => {
  // Must have at least one node
  if (nodes.length === 0) {
    return { isValid: false, message: 'Flow must have at least one node' };
  }

  // Must have at least one terminal node (node with no outgoing edges)
  // Note: cycles are allowed (for loops)
  const sourceNodeIds = new Set(edges.map(edge => edge.source));
  const terminalNodes = nodes.filter(node => !sourceNodeIds.has(node.id));

  if (terminalNodes.length === 0) {
    return {
      isValid: false,
      message: 'Flow must have at least one terminal node (node with no outgoing edges)'
    };
  }

  return { isValid: true };
};

// Helper function to add outputs based on node type
const getNodeOutputs = (nodeType) => {
  const outputsMap = {
    'conditional': ['if', 'else'],
    'hitl': ['approve', 'reject']
  };
  return outputsMap[nodeType] || undefined;
};

// Helper function to convert workflow nodes to ReactFlow format
const convertToReactFlowNodes = (workflowNodes) => {
  return workflowNodes.map((node, index) => {
    const outputs = getNodeOutputs(node.type);
    const data = {
      type: node.type,
      config: node.config,
      id: node.id
    };

    // Add outputs array if this node type has multiple outputs
    if (outputs) {
      data.outputs = outputs;
    }

    return {
      id: node.id,
      type: 'workflowNode',
      position: { x: 250 * index, y: 150 },
      data
    };
  });
};

// Helper function to convert workflow edges to ReactFlow format
const convertToReactFlowEdges = (workflowEdges) => {
  return workflowEdges.map((edge, index) => ({
    id: `e${edge.from}-${edge.to}-${index}`,
    source: edge.from,
    target: edge.to,
    label: edge.condition || '',
    type: 'smoothstep'
  }));
};

// Helper function to convert ReactFlow nodes back to workflow format
const convertFromReactFlowNodes = (reactFlowNodes) => {
  return reactFlowNodes.map(node => ({
    id: node.id,
    type: node.data.type || 'task',
    config: node.data.config || {}
  }));
};

// Helper function to convert ReactFlow edges back to workflow format
const convertFromReactFlowEdges = (reactFlowEdges) => {
  return reactFlowEdges.map(edge => ({
    from: edge.source,
    to: edge.target,
    condition: edge.label || undefined
  }));
};

// Generate JSON Patch operations from workflow diff
const generatePatchOperations = (original, current) => {
  const operations = [];

  // Track which original nodes/edges still exist
  const originalNodeIds = new Set(original.nodes.map(n => n.id));
  const currentNodeIds = new Set(current.nodes.map(n => n.id));

  // Added nodes
  current.nodes.forEach((node, index) => {
    if (!originalNodeIds.has(node.id)) {
      operations.push({
        op: 'add',
        path: '/nodes/-',
        value: node
      });
    }
  });

  // Removed nodes - collect indices first, then remove in reverse order
  const removedNodeIndices = [];
  original.nodes.forEach((node, index) => {
    if (!currentNodeIds.has(node.id)) {
      removedNodeIndices.push(index);
    }
  });
  // Sort in descending order and create remove operations
  removedNodeIndices.sort((a, b) => b - a).forEach(index => {
    operations.push({
      op: 'remove',
      path: `/nodes/${index}`
    });
  });

  // Modified nodes
  current.nodes.forEach((currentNode, currentIndex) => {
    const originalNode = original.nodes.find(n => n.id === currentNode.id);
    if (originalNode && JSON.stringify(originalNode) !== JSON.stringify(currentNode)) {
      const originalIndex = original.nodes.findIndex(n => n.id === currentNode.id);
      operations.push({
        op: 'replace',
        path: `/nodes/${originalIndex}`,
        value: currentNode
      });
    }
  });

  // Similar for edges
  const originalEdgeKeys = new Set(original.edges.map(e => `${e.from}-${e.to}`));
  const currentEdgeKeys = new Set(current.edges.map(e => `${e.from}-${e.to}`));

  // Added edges
  current.edges.forEach(edge => {
    const key = `${edge.from}-${edge.to}`;
    if (!originalEdgeKeys.has(key)) {
      operations.push({
        op: 'add',
        path: '/edges/-',
        value: edge
      });
    }
  });

  // Removed edges - collect indices first, then remove in reverse order
  const removedEdgeIndices = [];
  original.edges.forEach((edge, index) => {
    const key = `${edge.from}-${edge.to}`;
    if (!currentEdgeKeys.has(key)) {
      removedEdgeIndices.push(index);
    }
  });
  // Sort in descending order and create remove operations
  removedEdgeIndices.sort((a, b) => b - a).forEach(index => {
    operations.push({
      op: 'remove',
      path: `/edges/${index}`
    });
  });

  return operations;
};

export default function App() {
  // Routing - owner comes from X-User-ID header, not URL
  // React Router automatically decodes URL parameters
  const { tag } = useParams();
  const navigate = useNavigate();

  // Component state
  const [selectedNode, setSelectedNode] = useState(null);
  const [flowData, setFlowData] = useState({ nodes: [], edges: [] });
  const [isLoading, setIsLoading] = useState(true);
  const [isPanelOpen, setIsPanelOpen] = useState(true);
  const [isMobile] = useMediaQuery('(max-width: 768px)');
  const toast = useToast();

  // Workflow state
  const [selectedWorkflowId, setSelectedWorkflowId] = useState('document-analysis');
  const [selectedBranch, setSelectedBranch] = useState('main');
  const [selectedVersionIndex, setSelectedVersionIndex] = useState(0);
  const [currentWorkflow, setCurrentWorkflow] = useState(null);
  const [workflowVersions, setWorkflowVersions] = useState([]);
  const [workflowMetadata, setWorkflowMetadata] = useState(null); // Store API metadata (kind, depth, patch_count, etc.)
  const [patchChain, setPatchChain] = useState([]); // Store patch history from API
  const [useMockData, setUseMockData] = useState(false); // Fallback to mock data if API fails
  const [originalWorkflow, setOriginalWorkflow] = useState(null); // Store original workflow for diff

  // Compare mode state
  const [isComparing, setIsComparing] = useState(false);
  const [showCompareModal, setShowCompareModal] = useState(false);
  const [comparisonData, setComparisonData] = useState(null);

  // Workflow execution state
  const [isExecutionDrawerOpen, setIsExecutionDrawerOpen] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const [executionEvents, setExecutionEvents] = useState([]);
  const [executionError, setExecutionError] = useState(null);
  const username = import.meta.env.VITE_DEV_USERNAME || 'test-user';

  // Memoized WebSocket event handler to prevent reconnection loop
  // No dependencies - all state updates are functional
  const handleWebSocketEvent = useCallback((event) => {
    console.log('[Flow] WebSocket event received:', event);

    // Use functional state update to avoid dependency on executionEvents
    setExecutionEvents((prev) => [...prev, event]);

    // Check if workflow completed
    if (event.type === 'workflow_completed') {
      setIsRunning(false);
      // Show toast notification (toast function should be stable from Chakra)
      // If toast is unstable, we accept it as this is a rare event
      toast({
        title: 'Workflow completed',
        status: 'success',
        duration: 3000,
        isClosable: true,
        position: 'top-right',
      });
    }
  }, []); // No dependencies - prevents callback from changing

  // WebSocket hook for real-time execution events
  const { isConnected, connectionError, reconnect, disconnect } = useWorkflowWebSocket(
    username,
    handleWebSocketEvent
  );

  // Sidebar fixed width for desktop
  const sidebarDesktopWidth = 320; // px

  // Auto-open panel on mobile when a node is selected
  useEffect(() => {
    if (isMobile && selectedNode) {
      setIsPanelOpen(true);
    }
  }, [selectedNode, isMobile]);

  // Load workflow from API when URL params change
  useEffect(() => {
    // If we have tag in URL, try to load from API
    if (tag) {
      const loadFromAPI = async () => {
        setIsLoading(true);
        try {
          const data = await getWorkflow(tag, true);

          // Set workflow data from API response
          if (data.workflow) {
            const workflow = data.workflow;
            setSelectedWorkflowId(tag);
            setSelectedBranch(tag);
            setCurrentWorkflow(workflow);

            // Store metadata
            setWorkflowMetadata({
              kind: data.kind,
              depth: data.depth,
              patch_count: data.patch_count,
              artifact_id: data.artifact_id,
              created_at: data.created_at,
              created_by: data.created_by
            });

            // Store patch chain if available
            if (data.components && data.components.patches) {
              setPatchChain(data.components.patches);
            } else {
              setPatchChain([]);
            }

            // Convert to ReactFlow format
            const reactFlowNodes = convertToReactFlowNodes(workflow.nodes || []);
            const reactFlowEdges = convertToReactFlowEdges(workflow.edges || []);

            setFlowData({
              nodes: reactFlowNodes,
              edges: reactFlowEdges
            });

            // Store original workflow for diff calculation
            // Convert back to workflow format to ensure symmetric comparison
            const originalNodes = convertFromReactFlowNodes(reactFlowNodes);
            const originalEdges = convertFromReactFlowEdges(reactFlowEdges);
            setOriginalWorkflow({
              nodes: originalNodes,
              edges: originalEdges
            });

            setUseMockData(false);
          }
        } catch (error) {
          console.error('Failed to load workflow from API:', error);

          // Fallback to mock data
          toast({
            title: 'API Unavailable',
            description: 'Using mock data instead',
            status: 'warning',
            duration: 3000,
            isClosable: true,
          });

          setUseMockData(true);
        } finally {
          setIsLoading(false);
        }
      };

      loadFromAPI();
      return; // Don't execute the mock data loading below
    } else {
      // No URL params, use mock data
      setUseMockData(true);
      setIsLoading(false);
    }
  }, [tag, toast]);

  // Load workflow when selection changes (mock data only)
  useEffect(() => {
    if (!useMockData) return; // Skip if using API data

    if (!selectedWorkflowId) {
      setFlowData({ nodes: [], edges: [] });
      setCurrentWorkflow(null);
      setWorkflowVersions([]);
      return;
    }

    const workflow = mockWorkflows[selectedWorkflowId];
    if (!workflow || !workflow.branches[selectedBranch]) {
      return;
    }

    const branchData = workflow.branches[selectedBranch];
    const versions = branchData.versions;
    setWorkflowVersions(versions);

    // Load latest version by default
    const versionIndex = selectedVersionIndex < versions.length ? selectedVersionIndex : versions.length - 1;
    setSelectedVersionIndex(versionIndex);

    const version = versions[versionIndex];
    setCurrentWorkflow(version);

    // Convert to ReactFlow format
    const reactFlowNodes = convertToReactFlowNodes(version.nodes);
    const reactFlowEdges = convertToReactFlowEdges(version.edges);

    setFlowData({
      nodes: reactFlowNodes,
      edges: reactFlowEdges
    });
  }, [useMockData, selectedWorkflowId, selectedBranch, selectedVersionIndex]);

  const handleNodeDeselect = () => {
    setSelectedNode(null);
  };

  const handleNodesChange = useCallback((changes) => {
    setFlowData(prev => ({
      ...prev,
      nodes: applyNodeChanges(changes, prev.nodes),
    }));
  }, []);

  const handleEdgesChange = useCallback((changes) => {
    setFlowData(prev => ({
      ...prev,
      edges: applyEdgeChanges(changes, prev.edges),
    }));
  }, []);

  const handleConnect = useCallback((connection) => {
    setFlowData(prev => ({
      ...prev,
      edges: addEdge(connection, prev.edges),
    }));
  }, []);

  const handleNodeUpdate = useCallback(
    (updatedNode) => {
      setFlowData(prev => ({
        ...prev,
        nodes: prev.nodes.map(node =>
          node.id === updatedNode.id
            ? { ...node, data: { ...updatedNode.data } }
            : node
        ),
      }));

      if (selectedNode && selectedNode.id === updatedNode.id) {
        setSelectedNode(prev => ({
          ...prev,
          data: { ...updatedNode.data },
        }));
      }
    },
    [selectedNode]
  );

  const handleSave = useCallback(async () => {
    // Validate the flow first
    const validation = validateFlow(flowData.nodes, flowData.edges);

    if (!validation.isValid) {
      toast({
        title: 'Cannot save flow',
        description: validation.message,
        status: 'error',
        duration: 5000,
        isClosable: true,
        position: 'top-right',
      });
      return;
    }

    // If using API data and we have a tag, generate patches and update
    if (!useMockData && tag && originalWorkflow) {
      try {
        // Convert current ReactFlow data to workflow format
        const currentWorkflowNodes = convertFromReactFlowNodes(flowData.nodes);
        const currentWorkflowEdges = convertFromReactFlowEdges(flowData.edges);

        const currentWorkflow = {
          nodes: currentWorkflowNodes,
          edges: currentWorkflowEdges,
        };

        // Generate JSON Patch operations
        const operations = generatePatchOperations(originalWorkflow, currentWorkflow);

        // Check if there are any changes
        if (operations.length === 0) {
          toast({
            title: 'No changes to save',
            status: 'info',
            duration: 3000,
            isClosable: true,
            position: 'top-right',
          });
          return;
        }

        // Validate that patches can be applied (basic check)
        // This is a simple validation - the backend will do more thorough validation
        const hasValidOps = operations.every(op =>
          ['add', 'remove', 'replace'].includes(op.op) && op.path
        );

        if (!hasValidOps) {
          toast({
            title: 'Invalid patch operations',
            description: 'Generated patches contain invalid operations',
            status: 'error',
            duration: 5000,
            isClosable: true,
            position: 'top-right',
          });
          return;
        }

        // Send patches to backend
        const result = await updateWorkflow(tag, operations, 'Manual workflow update');

        toast({
          title: 'Flow saved successfully',
          description: `Applied ${operations.length} changes`,
          status: 'success',
          duration: 3000,
          isClosable: true,
          position: 'top-right',
        });

        console.log('Workflow updated:', result);

        // Reload the workflow to get updated patch chain and metadata
        try {
          const data = await getWorkflow(tag, true);

          if (data.workflow) {
            const workflow = data.workflow;

            // Update metadata
            setWorkflowMetadata({
              kind: data.kind,
              depth: data.depth,
              patch_count: data.patch_count,
              artifact_id: data.artifact_id,
              created_at: data.created_at,
              created_by: data.created_by
            });

            // Update patch chain
            if (data.components && data.components.patches) {
              setPatchChain(data.components.patches);
            } else {
              setPatchChain([]);
            }

            // Update original workflow for future diffs
            const originalNodes = convertFromReactFlowNodes(flowData.nodes);
            const originalEdges = convertFromReactFlowEdges(flowData.edges);
            setOriginalWorkflow({
              nodes: originalNodes,
              edges: originalEdges
            });
          }
        } catch (reloadError) {
          console.warn('Failed to reload workflow after save:', reloadError);
          // Still update original workflow even if reload fails
          setOriginalWorkflow(currentWorkflow);
        }

      } catch (error) {
        console.error('Failed to save workflow:', error);
        toast({
          title: 'Failed to save flow',
          description: error.message || 'An error occurred while saving',
          status: 'error',
          duration: 5000,
          isClosable: true,
          position: 'top-right',
        });
      }
    } else {
      // Mock data or no tag - just show success message
      toast({
        title: 'Flow saved successfully',
        status: 'success',
        duration: 3000,
        isClosable: true,
        position: 'top-right',
      });
    }
  }, [flowData, toast, useMockData, tag, originalWorkflow]);

  // Handle workflow change
  const handleWorkflowChange = useCallback((workflowId) => {
    setSelectedWorkflowId(workflowId);
    if (workflowId) {
      const workflow = mockWorkflows[workflowId];
      const firstBranch = Object.keys(workflow.branches)[0];
      setSelectedBranch(firstBranch);
      setSelectedVersionIndex(0);
    }
  }, []);

  // Handle branch change
  const handleBranchChange = useCallback((branch) => {
    setSelectedBranch(branch);
    setSelectedVersionIndex(0);
  }, []);

  // Handle version change
  const handleVersionChange = useCallback((versionIndex) => {
    setSelectedVersionIndex(versionIndex);
  }, []);

  // Helper function to get workflow for a specific branch
  const getWorkflowForBranch = useCallback((workflowId, branchTag) => {
    const workflow = mockWorkflows[workflowId];
    if (!workflow || !workflow.branches[branchTag]) return null;

    const branchData = workflow.branches[branchTag];
    const latestVersion = branchData.versions[branchData.versions.length - 1];
    return latestVersion;
  }, []);

  // Handle compare button click
  const handleCompare = useCallback(() => {
    setShowCompareModal(true);
  }, []);

  // Handle comparison result
  const handleComparisonResult = useCallback((result) => {
    const { branchA, branchB, workflowA, workflowB, diff, viewMode } = result;

    // Convert workflows to ReactFlow format
    let nodesA = convertToReactFlowNodes(workflowA.nodes);
    let edgesA = convertToReactFlowEdges(workflowA.edges);
    let nodesB = convertToReactFlowNodes(workflowB.nodes);
    let edgesB = convertToReactFlowEdges(workflowB.edges);

    // Apply diff colors to nodes and edges
    nodesA = applyDiffColorsToNodes(nodesA, diff, 'before');
    edgesA = applyDiffColorsToEdges(edgesA, diff, 'before');
    nodesB = applyDiffColorsToNodes(nodesB, diff, 'after');
    edgesB = applyDiffColorsToEdges(edgesB, diff, 'after');

    setComparisonData({
      branchA,
      branchB,
      workflowA,
      workflowB,
      nodesA,
      edgesA,
      nodesB,
      edgesB,
      diff,
      viewMode // Store view mode
    });

    setIsComparing(true);
    setShowCompareModal(false);
  }, []);

  // Exit compare mode
  const handleExitCompare = useCallback(() => {
    setIsComparing(false);
    setComparisonData(null);
  }, []);

  // Handle version comparison - reuses handleComparisonResult logic
  const handleCompareVersions = useCallback(async (version1, version2, viewMode = 'sidebyside') => {
    if (!tag) return;

    try {
      // Helper to fetch a specific version (0 = base, >0 = patch version)
      const fetchVersion = async (seq) => {
        if (seq === 0) {
          // For version 0, we need to get the base dag_version
          // First get the current workflow to find the base
          const data = await getWorkflow(tag, true);

          // If we have patches, we need to reconstruct the base workflow
          // by subtracting all patches from the materialized workflow
          // For now, we'll try to get version 1 and work backwards
          // A better solution would be to have a dedicated API endpoint

          // Temporary workaround: Get the unmaterialized workflow which should be the base
          const baseData = await getWorkflow(tag, false);

          // The workflow in baseData should be the base (before any patches)
          if (baseData && baseData.workflow) {
            return baseData;
          }

          throw new Error('Unable to fetch base version (version 0)');
        } else {
          // For versions >= 1, use the versions endpoint
          return await getWorkflowVersion(tag, seq, true);
        }
      };

      // Fetch both versions from API
      const [data1, data2] = await Promise.all([
        fetchVersion(version1),
        fetchVersion(version2)
      ]);

      const workflowA = data1.workflow;
      const workflowB = data2.workflow;

      // Calculate diff using workflowDiff utility
      const diff = computeWorkflowDiff(workflowA, workflowB);

      // Reuse existing comparison result handler
      handleComparisonResult({
        branchA: `Version ${version1}`,
        branchB: `Version ${version2}`,
        workflowA,
        workflowB,
        diff,
        viewMode
      });

      toast({
        title: 'Comparing versions',
        description: `Version ${version1} vs Version ${version2}`,
        status: 'info',
        duration: 2000,
        isClosable: true,
      });
    } catch (error) {
      console.error('Failed to compare versions:', error);
      toast({
        title: 'Comparison failed',
        description: error.message,
        status: 'error',
        duration: 3000,
        isClosable: true,
      });
    }
  }, [tag, toast, handleComparisonResult]);

  const handleBackToList = () => {
    navigate('/');
  };

  // Handle run button click - opens execution drawer
  const handleRun = useCallback(() => {
    // Reset execution state
    setExecutionEvents([]);
    setExecutionError(null);
    setIsExecutionDrawerOpen(true);
  }, []);

  // Handle workflow execution with inputs
  const handleRunWorkflow = useCallback(async (inputs) => {
    if (!tag) {
      toast({
        title: 'No workflow selected',
        description: 'Please select a workflow to run',
        status: 'error',
        duration: 3000,
        isClosable: true,
        position: 'top-right',
      });
      return;
    }

    setIsRunning(true);
    setExecutionError(null);

    try {
      const result = await runWorkflow(tag, inputs);
      console.log('[Flow] Workflow started:', result);

      toast({
        title: 'Workflow started',
        description: `Run ID: ${result.run_id?.substring(0, 8)}...`,
        status: 'success',
        duration: 3000,
        isClosable: true,
        position: 'top-right',
      });
    } catch (error) {
      console.error('[Flow] Failed to start workflow:', error);
      setIsRunning(false);
      setExecutionError(error.message || 'Failed to start workflow');

      toast({
        title: 'Failed to start workflow',
        description: error.message,
        status: 'error',
        duration: 5000,
        isClosable: true,
        position: 'top-right',
      });
    }
  }, [tag, toast]);

  // Get all workflows for selector (mock data only)
  const allWorkflows = useMockData ? getAllWorkflows() : [];

  // Get branches for current workflow (mock data only)
  const branches = useMockData && selectedWorkflowId ? getBranches(selectedWorkflowId) : [];

  // Early return after all hooks have been called
  if (isLoading) {
    return <LoadingState fullScreen size="xl" />;
  }

  return (
    <Flex direction="column" height="100vh" width="100vw" bg="#f7f8fa" overflow="hidden">
      {/* Back button - only show if we have URL params (came from list) */}
      {tag && (
        <Box
          position="absolute"
          top="8px"
          left="8px"
          zIndex={20}
        >
          <Button
            size="sm"
            leftIcon={<FiArrowLeft />}
            onClick={handleBackToList}
            variant="ghost"
          >
            Back to List
          </Button>
        </Box>
      )}

      <Header
        onSave={handleSave}
        workflows={allWorkflows}
        selectedWorkflowId={selectedWorkflowId}
        onWorkflowChange={handleWorkflowChange}
        branches={branches}
        selectedBranch={selectedBranch}
        onBranchChange={handleBranchChange}
        workflowName={currentWorkflow?.metadata?.name}
        onCompare={isComparing ? handleExitCompare : handleCompare}
        isComparing={isComparing}
        onRun={handleRun}
        isRunning={isRunning}
      />

      {/* Branch Comparison Modal */}
      <Modal isOpen={showCompareModal} onClose={() => setShowCompareModal(false)} size="md">
        <ModalOverlay />
        <ModalContent>
          <ModalHeader>Compare Branches</ModalHeader>
          <ModalCloseButton />
          <ModalBody pb={6}>
            <BranchComparison
              branches={branches}
              workflowId={selectedWorkflowId}
              onCompare={handleComparisonResult}
              onClose={() => setShowCompareModal(false)}
              getWorkflowForBranch={getWorkflowForBranch}
            />
          </ModalBody>
        </ModalContent>
      </Modal>

      <Flex flex="1" mt="48px">
        {/* Main Canvas Area */}
        <Box
          flex="1"
          position="relative"
          onClick={handleNodeDeselect}
          style={{ cursor: 'default' }}
        >
          {!isComparing ? (
            <FlowCanvas
              nodes={flowData.nodes}
              edges={flowData.edges}
              selectedNode={selectedNode}
              setSelectedNode={setSelectedNode}
              onNodesChange={handleNodesChange}
              onEdgesChange={handleEdgesChange}
              onConnect={handleConnect}
            />
          ) : comparisonData?.viewMode === 'overlay' ? (
            <BranchDiffOverlay
              branchA={comparisonData?.branchA}
              branchB={comparisonData?.branchB}
              nodesA={comparisonData?.nodesA}
              edgesA={comparisonData?.edgesA}
              nodesB={comparisonData?.nodesB}
              edgesB={comparisonData?.edgesB}
              diff={comparisonData?.diff}
            />
          ) : (
            <BranchDiffCanvas
              branchA={comparisonData?.branchA}
              branchB={comparisonData?.branchB}
              nodesA={comparisonData?.nodesA}
              edgesA={comparisonData?.edgesA}
              nodesB={comparisonData?.nodesB}
              edgesB={comparisonData?.edgesB}
              diff={comparisonData?.diff}
            />
          )}
        </Box>

        {/* Overlay for mobile when sidebar is open */}
        {isMobile && isPanelOpen && (
          <Box
            position="fixed"
            top="48px"
            left={0}
            right={0}
            bottom={0}
            bg="blackAlpha.400"
            zIndex={14}
            onClick={() => setIsPanelOpen(false)}
          />
        )}

        {/* Toggle Button */}
        <IconButton
          aria-label={isPanelOpen ? 'Collapse panel' : 'Expand panel'}
          icon={
            isPanelOpen
              ? (isMobile ? <ChevronRightIcon /> : <ChevronRightIcon />)
              : (isMobile ? <ChevronLeftIcon /> : <ChevronLeftIcon />)
          }
          onClick={() => setIsPanelOpen(!isPanelOpen)}
          position="fixed"
          // On mobile: right is 20vw (sidebar width) + 8px; on desktop: right is 320px + 8px
          right={{
            base: isPanelOpen ? 'calc(20vw + 8px)' : '8px',
            md: isPanelOpen ? (sidebarDesktopWidth + 8) + 'px' : '8px',
          }}
          top="50%"
          transform="translateY(-50%)"
          zIndex={20}
          bg="white"
          border="1px solid"
          borderColor="gray.200"
          borderRadius="md 0 0 md"
          boxShadow="md"
          _hover={{ bg: 'gray.50' }}
          transition="right 0.3s"
          width="32px"
          height="48px"
          display="flex"
          alignItems="center"
          justifyContent="center"
          p={0}
        />

        {/* Sidebar */}
        <Box
          width={{ base: '20vw', md: sidebarDesktopWidth + 'px' }}
          maxW={{ base: '96vw', md: sidebarDesktopWidth + 'px' }}
          minW="140px"
          bg="white"
          borderLeft="1px solid #e3e6ea"
          height="calc(100vh - 48px)"
          overflowY="auto"
          position="fixed"
          right={{
            base: isPanelOpen ? 0 : '-20vw',
            md: isPanelOpen ? 0 : `-${sidebarDesktopWidth}px`,
          }}
          top="48px"
          bottom={0}
          transition="right 0.3s cubic-bezier(.4,0,.2,1)"
          zIndex={15}
          boxShadow={
            isPanelOpen
              ? { base: '-4px 0 15px rgba(0,0,0,0.1)', md: '-2px 0 10px rgba(0,0,0,0.05)' }
              : 'none'
          }
          display="block"
        >
          <Box p={4}>
            <Box mt={4}>
              <NodesPanel
                selectedNode={selectedNode}
                onNodeDeselect={() => {
                  handleNodeDeselect();
                  if (isMobile) setIsPanelOpen(false);
                }}
                onNodeUpdate={handleNodeUpdate}
                currentWorkflow={currentWorkflow}
                workflowVersions={workflowVersions}
                selectedVersionIndex={selectedVersionIndex}
                onVersionChange={handleVersionChange}
                isComparing={isComparing}
                comparisonData={comparisonData}
                patchChain={patchChain}
                workflowMetadata={workflowMetadata}
                onCompareVersions={handleCompareVersions}
              />
            </Box>
          </Box>
        </Box>
      </Flex>

      {/* Execution Drawer */}
      <ExecutionDrawer
        isOpen={isExecutionDrawerOpen}
        onClose={() => setIsExecutionDrawerOpen(false)}
        workflowTag={tag || selectedBranch}
        onRunWorkflow={handleRunWorkflow}
        isRunning={isRunning}
        events={executionEvents}
        connectionStatus={{ isConnected, error: connectionError }}
        error={executionError}
      />
    </Flex>
  );
}
