import React, { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Flex,
  Box,
  Spinner,
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
import { mockWorkflows, getAllWorkflows, getBranches, getLatestVersion } from '../data/mockWorkflows';
import { applyDiffColorsToNodes, applyDiffColorsToEdges } from '../utils/workflowDiff';
import { getWorkflow } from '../services/api';

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
  const [useMockData, setUseMockData] = useState(false); // Fallback to mock data if API fails

  // Compare mode state
  const [isComparing, setIsComparing] = useState(false);
  const [showCompareModal, setShowCompareModal] = useState(false);
  const [comparisonData, setComparisonData] = useState(null);

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

            // Convert to ReactFlow format
            const reactFlowNodes = convertToReactFlowNodes(workflow.nodes || []);
            const reactFlowEdges = convertToReactFlowEdges(workflow.edges || []);

            setFlowData({
              nodes: reactFlowNodes,
              edges: reactFlowEdges
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

  const handleBackToList = () => {
    navigate('/');
  };

  // Get all workflows for selector (mock data only)
  const allWorkflows = useMockData ? getAllWorkflows() : [];

  // Get branches for current workflow (mock data only)
  const branches = useMockData && selectedWorkflowId ? getBranches(selectedWorkflowId) : [];

  // Early return after all hooks have been called
  if (isLoading) {
    return (
      <Flex justify="center" align="center" height="100vh">
        <Spinner size="xl" />
      </Flex>
    );
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
              />
            </Box>
          </Box>
        </Box>
      </Flex>
    </Flex>
  );
}
