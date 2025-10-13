import React, { useState, useEffect, useCallback } from 'react';
import {
  Flex,
  Box,
  Spinner,
  useToast,
  IconButton,
  useMediaQuery,
} from '@chakra-ui/react';
import { ChevronLeftIcon, ChevronRightIcon } from '@chakra-ui/icons';
import { applyNodeChanges, applyEdgeChanges, addEdge } from '@xyflow/react';
import FlowCanvas from '../components/flow/FlowCanvas';
import NodesPanel from '../components/NodesPanel';
import Header from '../components/ui/header/Header';
import { mockWorkflows, getAllWorkflows, getBranches, getLatestVersion } from '../data/mockWorkflows';

// Function to validate the flow
const validateFlow = (nodes, edges) => {
  if (nodes.length === 0) return { isValid: false, message: 'No nodes in the flow' };
  if (edges.length === 0) return { isValid: false, message: 'No connecting nodes' };

  // Create a Set of all source node IDs from edges
  const sourceNodeIds = new Set(edges.map(edge => edge.source));
  // Count nodes without outgoing edges
  const nodesWithoutOutgoingEdges = nodes.filter(
    node => !sourceNodeIds.has(node.id)
  );
  if (nodesWithoutOutgoingEdges.length === 0) {
    return { isValid: false, message: 'Flow contains a cycle or all nodes are connected' };
  }
  if (nodesWithoutOutgoingEdges.length > 1) {
    return {
      isValid: false,
      message: `Multiple nodes (${nodesWithoutOutgoingEdges.length}) don't have outgoing edges. Only one node can be the end node.`,
    };
  }
  return { isValid: true };
};

// Helper function to convert workflow nodes to ReactFlow format
const convertToReactFlowNodes = (workflowNodes) => {
  return workflowNodes.map((node, index) => ({
    id: node.id,
    type: 'workflowNode',
    position: { x: 250 * index, y: 150 },
    data: {
      type: node.type,
      config: node.config,
      id: node.id
    }
  }));
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

export default function App() {
  const [selectedNode, setSelectedNode] = useState(null);
  const [flowData, setFlowData] = useState({ nodes: [], edges: [] });
  const [isLoading, setIsLoading] = useState(false);
  const [isPanelOpen, setIsPanelOpen] = useState(true);
  const [isMobile] = useMediaQuery('(max-width: 768px)');
  const toast = useToast();

  // Workflow state
  const [selectedWorkflowId, setSelectedWorkflowId] = useState('flight-booking');
  const [selectedBranch, setSelectedBranch] = useState('main');
  const [selectedVersionIndex, setSelectedVersionIndex] = useState(0);
  const [currentWorkflow, setCurrentWorkflow] = useState(null);
  const [workflowVersions, setWorkflowVersions] = useState([]);

  // Sidebar fixed width for desktop
  const sidebarDesktopWidth = 320; // px

  // Auto-open panel on mobile when a node is selected
  useEffect(() => {
    if (isMobile && selectedNode) {
      setIsPanelOpen(true);
    }
  }, [selectedNode, isMobile]);

  // Load workflow when selection changes
  useEffect(() => {
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
  }, [selectedWorkflowId, selectedBranch, selectedVersionIndex]);

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

  const handleSave = useCallback(() => {
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

    toast({
      title: 'Flow saved successfully',
      status: 'success',
      duration: 3000,
      isClosable: true,
      position: 'top-right',
    });

    // Here you would typically call your API to save the flow
  }, [flowData, toast]);

  if (isLoading) {
    return (
      <Flex justify="center" align="center" height="100vh">
        <Spinner size="xl" />
      </Flex>
    );
  }

  // Get all workflows for selector
  const allWorkflows = getAllWorkflows();

  // Get branches for current workflow
  const branches = selectedWorkflowId ? getBranches(selectedWorkflowId) : [];

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

  return (
    <Flex direction="column" height="100vh" width="100vw" bg="#f7f8fa" overflow="hidden">
      <Header
        onSave={handleSave}
        workflows={allWorkflows}
        selectedWorkflowId={selectedWorkflowId}
        onWorkflowChange={handleWorkflowChange}
        branches={branches}
        selectedBranch={selectedBranch}
        onBranchChange={handleBranchChange}
        workflowName={currentWorkflow?.metadata?.name}
      />

      <Flex flex="1" mt="48px">
        {/* Main Canvas Area */}
        <Box
          flex="1"
          position="relative"
          onClick={handleNodeDeselect}
          style={{ cursor: 'default' }}
        >
          <FlowCanvas
            nodes={flowData.nodes}
            edges={flowData.edges}
            selectedNode={selectedNode}
            setSelectedNode={setSelectedNode}
            onNodesChange={handleNodesChange}
            onEdgesChange={handleEdgesChange}
            onConnect={handleConnect}
          />
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
              />
            </Box>
          </Box>
        </Box>
      </Flex>
    </Flex>
  );
}
