import {
  Box,
  VStack,
  Text,
  Select,
  Divider,
  Badge,
  HStack,
  Code,
  Accordion,
  AccordionItem,
  AccordionButton,
  AccordionPanel,
  AccordionIcon,
} from '@chakra-ui/react';
import { useState } from 'react';

export default function VersionDiff({ versions = [], selectedVersionIndex = 0, onVersionChange }) {
  const [compareVersionIndex, setCompareVersionIndex] = useState(
    selectedVersionIndex > 0 ? selectedVersionIndex - 1 : 0
  );

  if (versions.length === 0) {
    return (
      <Box p={4}>
        <Text fontSize="sm" color="gray.500">
          No versions available
        </Text>
      </Box>
    );
  }

  const currentVersion = versions[selectedVersionIndex];
  const compareVersion = versions[compareVersionIndex];

  // Calculate differences
  const calculateDiff = () => {
    if (!currentVersion || !compareVersion) return { added: [], removed: [], modified: [] };

    const currentNodes = new Map(currentVersion.nodes.map(n => [n.id, n]));
    const compareNodes = new Map(compareVersion.nodes.map(n => [n.id, n]));

    const added = [];
    const removed = [];
    const modified = [];

    // Find added and modified nodes
    currentNodes.forEach((node, id) => {
      if (!compareNodes.has(id)) {
        added.push(node);
      } else {
        const compareNode = compareNodes.get(id);
        if (JSON.stringify(node) !== JSON.stringify(compareNode)) {
          modified.push({ current: node, previous: compareNode });
        }
      }
    });

    // Find removed nodes
    compareNodes.forEach((node, id) => {
      if (!currentNodes.has(id)) {
        removed.push(node);
      }
    });

    return { added, removed, modified };
  };

  const diff = calculateDiff();

  const formatNodeInfo = (node) => {
    return `${node.config?.name || node.id} (${node.type})`;
  };

  return (
    <VStack align="stretch" spacing={4} w="100%">
      <Text fontSize="md" fontWeight="semibold" color="gray.700">
        Version Comparison
      </Text>

      {/* Version Selectors */}
      <VStack align="stretch" spacing={2}>
        <Box>
          <Text fontSize="xs" color="gray.600" mb={1}>
            Current Version
          </Text>
          <Select
            size="sm"
            value={selectedVersionIndex}
            onChange={(e) => onVersionChange && onVersionChange(parseInt(e.target.value))}
            bg="white"
          >
            {versions.map((version, idx) => (
              <option key={idx} value={idx}>
                {version.version} - {new Date(version.timestamp).toLocaleDateString()}
              </option>
            ))}
          </Select>
        </Box>

        <Box>
          <Text fontSize="xs" color="gray.600" mb={1}>
            Compare With
          </Text>
          <Select
            size="sm"
            value={compareVersionIndex}
            onChange={(e) => setCompareVersionIndex(parseInt(e.target.value))}
            bg="white"
          >
            {versions.map((version, idx) => (
              <option key={idx} value={idx}>
                {version.version} - {new Date(version.timestamp).toLocaleDateString()}
              </option>
            ))}
          </Select>
        </Box>
      </VStack>

      <Divider />

      {/* Diff Summary */}
      <HStack spacing={3}>
        <Badge colorScheme="green" fontSize="xs">
          +{diff.added.length} Added
        </Badge>
        <Badge colorScheme="red" fontSize="xs">
          -{diff.removed.length} Removed
        </Badge>
        <Badge colorScheme="yellow" fontSize="xs">
          ~{diff.modified.length} Modified
        </Badge>
      </HStack>

      {/* Detailed Changes */}
      <Accordion allowMultiple size="sm">
        {/* Added Nodes */}
        {diff.added.length > 0 && (
          <AccordionItem>
            <AccordionButton>
              <Box flex="1" textAlign="left">
                <HStack>
                  <Badge colorScheme="green" fontSize="xs">
                    Added
                  </Badge>
                  <Text fontSize="sm">{diff.added.length} nodes</Text>
                </HStack>
              </Box>
              <AccordionIcon />
            </AccordionButton>
            <AccordionPanel pb={4}>
              <VStack align="stretch" spacing={2}>
                {diff.added.map((node, idx) => (
                  <Box
                    key={idx}
                    p={2}
                    bg="green.50"
                    borderLeft="3px solid"
                    borderColor="green.500"
                    borderRadius="md"
                  >
                    <Text fontSize="xs" fontWeight="medium" color="green.800">
                      {formatNodeInfo(node)}
                    </Text>
                    <Code
                      fontSize="xs"
                      bg="green.100"
                      p={1}
                      borderRadius="sm"
                      display="block"
                      mt={1}
                    >
                      {JSON.stringify(node.config, null, 2)}
                    </Code>
                  </Box>
                ))}
              </VStack>
            </AccordionPanel>
          </AccordionItem>
        )}

        {/* Removed Nodes */}
        {diff.removed.length > 0 && (
          <AccordionItem>
            <AccordionButton>
              <Box flex="1" textAlign="left">
                <HStack>
                  <Badge colorScheme="red" fontSize="xs">
                    Removed
                  </Badge>
                  <Text fontSize="sm">{diff.removed.length} nodes</Text>
                </HStack>
              </Box>
              <AccordionIcon />
            </AccordionButton>
            <AccordionPanel pb={4}>
              <VStack align="stretch" spacing={2}>
                {diff.removed.map((node, idx) => (
                  <Box
                    key={idx}
                    p={2}
                    bg="red.50"
                    borderLeft="3px solid"
                    borderColor="red.500"
                    borderRadius="md"
                  >
                    <Text fontSize="xs" fontWeight="medium" color="red.800">
                      {formatNodeInfo(node)}
                    </Text>
                    <Code
                      fontSize="xs"
                      bg="red.100"
                      p={1}
                      borderRadius="sm"
                      display="block"
                      mt={1}
                    >
                      {JSON.stringify(node.config, null, 2)}
                    </Code>
                  </Box>
                ))}
              </VStack>
            </AccordionPanel>
          </AccordionItem>
        )}

        {/* Modified Nodes */}
        {diff.modified.length > 0 && (
          <AccordionItem>
            <AccordionButton>
              <Box flex="1" textAlign="left">
                <HStack>
                  <Badge colorScheme="yellow" fontSize="xs">
                    Modified
                  </Badge>
                  <Text fontSize="sm">{diff.modified.length} nodes</Text>
                </HStack>
              </Box>
              <AccordionIcon />
            </AccordionButton>
            <AccordionPanel pb={4}>
              <VStack align="stretch" spacing={3}>
                {diff.modified.map((change, idx) => (
                  <Box key={idx}>
                    <Text fontSize="xs" fontWeight="medium" color="yellow.800" mb={2}>
                      {formatNodeInfo(change.current)}
                    </Text>

                    {/* Before */}
                    <Box mb={2}>
                      <Text fontSize="xs" color="red.600" fontWeight="medium">
                        Before:
                      </Text>
                      <Code
                        fontSize="xs"
                        bg="red.50"
                        p={1}
                        borderRadius="sm"
                        display="block"
                        borderLeft="2px solid"
                        borderColor="red.300"
                      >
                        {JSON.stringify(change.previous.config, null, 2)}
                      </Code>
                    </Box>

                    {/* After */}
                    <Box>
                      <Text fontSize="xs" color="green.600" fontWeight="medium">
                        After:
                      </Text>
                      <Code
                        fontSize="xs"
                        bg="green.50"
                        p={1}
                        borderRadius="sm"
                        display="block"
                        borderLeft="2px solid"
                        borderColor="green.300"
                      >
                        {JSON.stringify(change.current.config, null, 2)}
                      </Code>
                    </Box>
                  </Box>
                ))}
              </VStack>
            </AccordionPanel>
          </AccordionItem>
        )}
      </Accordion>

      {/* No changes message */}
      {diff.added.length === 0 && diff.removed.length === 0 && diff.modified.length === 0 && (
        <Box p={4} bg="gray.50" borderRadius="md" textAlign="center">
          <Text fontSize="sm" color="gray.600">
            No differences between selected versions
          </Text>
        </Box>
      )}
    </VStack>
  );
}
