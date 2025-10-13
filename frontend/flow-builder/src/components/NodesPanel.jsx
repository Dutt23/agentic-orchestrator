import { Box, VStack, Text, Heading, Tabs, TabList, TabPanels, Tab, TabPanel } from '@chakra-ui/react';
import { useState, useEffect } from 'react';
import SelectedNodeMessage from './SelectedNodeMessage';
import NodeItem from './NodeItem';
import CodeView from './CodeView';
import VersionDiff from './VersionDiff';
import DiffDetailsPanel from './DiffDetailsPanel';
import VersionHistory from './VersionHistory';
import nodeTypesData from '../data/nodeTypes.json';

export default function NodesPanel({
  selectedNode,
  onNodeDeselect,
  onNodeUpdate,
  currentWorkflow,
  workflowVersions = [],
  selectedVersionIndex = 0,
  onVersionChange,
  isComparing = false,
  comparisonData = null,
  patchChain = [],
  workflowMetadata = null,
  onCompareVersions
}) {
  const [nodeTypes, setNodeTypes] = useState([]);

  useEffect(() => {
    // In a real app, you might fetch this from an API
    // Handle both old format (flat nodes array) and new format (categorized)
    if (nodeTypesData.categories) {
      setNodeTypes(nodeTypesData.categories);
    } else if (nodeTypesData.nodes) {
      // Legacy format - wrap in a single category
      setNodeTypes([{ name: 'Nodes', id: 'all', nodes: nodeTypesData.nodes }]);
    }
  }, []);

  const handleDragStart = (event, nodeType) => {
    event.dataTransfer.setData('application/reactflow', JSON.stringify({
      type: nodeType.type,
      data: nodeType.defaultData
    }));
    event.dataTransfer.effectAllowed = 'move';
  };

  if (selectedNode) {
    return (
      <SelectedNodeMessage
        node={selectedNode}
        onBack={onNodeDeselect}
        onNodeUpdate={onNodeUpdate}
      />
    );
  }

  return (
    <Box overflowY="auto" height="100%">
      <Tabs size="sm" variant="enclosed" colorScheme="blue" defaultIndex={isComparing ? 3 : 0}>
        <TabList px={4} pt={2}>
          <Tab fontSize="xs">Nodes</Tab>
          <Tab fontSize="xs">Code</Tab>
          <Tab fontSize="xs">History</Tab>
          {isComparing && <Tab fontSize="xs">Diff</Tab>}
        </TabList>

        <TabPanels>
          {/* Nodes Tab */}
          <TabPanel>
            <Heading size="md" mb={4} color="gray.700">
              Nodes Panel
            </Heading>
            <Text fontSize="sm" color="gray.500" mb={4}>
              Drag and drop nodes onto the canvas
            </Text>

            <VStack spacing={4} align="stretch">
              {nodeTypes.map((category) => (
                <Box key={category.id}>
                  <Text
                    fontSize="xs"
                    fontWeight="bold"
                    textTransform="uppercase"
                    color="gray.600"
                    mb={2}
                    letterSpacing="wide"
                  >
                    {category.name}
                  </Text>
                  {category.description && (
                    <Text fontSize="xs" color="gray.500" mb={2}>
                      {category.description}
                    </Text>
                  )}
                  <VStack spacing={2} align="stretch">
                    {category.nodes.map((nodeType) => (
                      <NodeItem
                        key={nodeType.type}
                        nodeType={nodeType}
                        onDragStart={handleDragStart}
                      />
                    ))}
                  </VStack>
                </Box>
              ))}
            </VStack>
          </TabPanel>

          {/* Code View Tab */}
          <TabPanel>
            <CodeView
              workflow={currentWorkflow}
              selectedNode={selectedNode}
            />
          </TabPanel>

          {/* Version History Tab */}
          <TabPanel>
            <VersionHistory
              patches={patchChain}
              metadata={workflowMetadata}
              onCompareVersions={onCompareVersions}
            />
          </TabPanel>

          {/* Diff Tab (only visible in compare mode) */}
          {isComparing && (
            <TabPanel>
              <DiffDetailsPanel
                diff={comparisonData?.diff}
                branchA={comparisonData?.branchA}
                branchB={comparisonData?.branchB}
              />
            </TabPanel>
          )}
        </TabPanels>
      </Tabs>
    </Box>
  );
}
