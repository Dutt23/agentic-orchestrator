import {
  VStack,
  Text,
  Box,
  HStack,
  Badge,
  Accordion,
  AccordionItem,
  AccordionButton,
  AccordionPanel,
  AccordionIcon,
  Code,
  Divider,
} from '@chakra-ui/react';
import { FiPlus, FiMinus, FiEdit } from 'react-icons/fi';

export default function DiffDetailsPanel({ diff, branchA, branchB }) {
  if (!diff) {
    return (
      <VStack align="stretch" spacing={4} p={4}>
        <Text fontSize="sm" color="gray.500">
          No comparison results yet. Click "Compare" to see differences.
        </Text>
      </VStack>
    );
  }

  const { added_nodes, removed_nodes, modified_nodes, summary } = diff;

  const hasChanges =
    added_nodes.length > 0 || removed_nodes.length > 0 || modified_nodes.length > 0;

  if (!hasChanges) {
    return (
      <VStack align="stretch" spacing={4} p={4}>
        <Text fontSize="sm" fontWeight="medium" color="green.600">
          ✓ No differences found
        </Text>
        <Text fontSize="xs" color="gray.500">
          Branches "{branchA}" and "{branchB}" are identical.
        </Text>
      </VStack>
    );
  }

  return (
    <VStack align="stretch" spacing={4} w="100%">
      {/* Summary */}
      <Box>
        <Text fontSize="sm" fontWeight="semibold" mb={2}>
          Comparison: {branchA} → {branchB}
        </Text>
        <HStack spacing={2} flexWrap="wrap">
          {summary.nodes.added > 0 && (
            <Badge colorScheme="green" fontSize="xs">
              +{summary.nodes.added}
            </Badge>
          )}
          {summary.nodes.removed > 0 && (
            <Badge colorScheme="red" fontSize="xs">
              -{summary.nodes.removed}
            </Badge>
          )}
          {summary.nodes.modified > 0 && (
            <Badge colorScheme="yellow" fontSize="xs">
              ~{summary.nodes.modified}
            </Badge>
          )}
        </HStack>
      </Box>

      <Divider />

      {/* Detailed Changes */}
      <Accordion allowMultiple size="sm">
        {/* Added Nodes */}
        {added_nodes.length > 0 && (
          <AccordionItem>
            <AccordionButton>
              <Box flex="1" textAlign="left">
                <HStack>
                  <FiPlus color="var(--chakra-colors-green-600)" />
                  <Text fontSize="sm" fontWeight="medium">
                    Added Nodes ({added_nodes.length})
                  </Text>
                </HStack>
              </Box>
              <AccordionIcon />
            </AccordionButton>
            <AccordionPanel pb={4}>
              <VStack align="stretch" spacing={2}>
                {added_nodes.map((node, idx) => (
                  <Box
                    key={idx}
                    p={2}
                    bg="green.50"
                    borderLeft="3px solid"
                    borderColor="green.500"
                    borderRadius="md"
                  >
                    <HStack justify="space-between" mb={1}>
                      <Text fontSize="xs" fontWeight="medium" color="green.800">
                        {node.data?.config?.name || node.id}
                      </Text>
                      <Badge colorScheme="green" fontSize="xs">
                        {node.data?.type || node.type}
                      </Badge>
                    </HStack>
                    <Code
                      fontSize="xs"
                      bg="green.100"
                      p={1}
                      borderRadius="sm"
                      display="block"
                    >
                      {JSON.stringify(node.data?.config || {}, null, 2)}
                    </Code>
                  </Box>
                ))}
              </VStack>
            </AccordionPanel>
          </AccordionItem>
        )}

        {/* Removed Nodes */}
        {removed_nodes.length > 0 && (
          <AccordionItem>
            <AccordionButton>
              <Box flex="1" textAlign="left">
                <HStack>
                  <FiMinus color="var(--chakra-colors-red-600)" />
                  <Text fontSize="sm" fontWeight="medium">
                    Removed Nodes ({removed_nodes.length})
                  </Text>
                </HStack>
              </Box>
              <AccordionIcon />
            </AccordionButton>
            <AccordionPanel pb={4}>
              <VStack align="stretch" spacing={2}>
                {removed_nodes.map((node, idx) => (
                  <Box
                    key={idx}
                    p={2}
                    bg="red.50"
                    borderLeft="3px solid"
                    borderColor="red.500"
                    borderRadius="md"
                  >
                    <HStack justify="space-between" mb={1}>
                      <Text fontSize="xs" fontWeight="medium" color="red.800">
                        {node.data?.config?.name || node.id}
                      </Text>
                      <Badge colorScheme="red" fontSize="xs">
                        {node.data?.type || node.type}
                      </Badge>
                    </HStack>
                    <Code
                      fontSize="xs"
                      bg="red.100"
                      p={1}
                      borderRadius="sm"
                      display="block"
                    >
                      {JSON.stringify(node.data?.config || {}, null, 2)}
                    </Code>
                  </Box>
                ))}
              </VStack>
            </AccordionPanel>
          </AccordionItem>
        )}

        {/* Modified Nodes */}
        {modified_nodes.length > 0 && (
          <AccordionItem>
            <AccordionButton>
              <Box flex="1" textAlign="left">
                <HStack>
                  <FiEdit color="var(--chakra-colors-yellow-600)" />
                  <Text fontSize="sm" fontWeight="medium">
                    Modified Nodes ({modified_nodes.length})
                  </Text>
                </HStack>
              </Box>
              <AccordionIcon />
            </AccordionButton>
            <AccordionPanel pb={4}>
              <VStack align="stretch" spacing={3}>
                {modified_nodes.map((change, idx) => (
                  <Box key={idx}>
                    <Text fontSize="xs" fontWeight="medium" color="yellow.800" mb={2}>
                      {change.before.data?.config?.name || change.id}
                    </Text>

                    {/* Show field-level changes */}
                    {change.changes && change.changes.length > 0 && (
                      <VStack align="stretch" spacing={1} mb={2}>
                        {change.changes.map((fieldChange, fieldIdx) => (
                          <HStack
                            key={fieldIdx}
                            fontSize="xs"
                            p={1}
                            bg="yellow.50"
                            borderRadius="sm"
                          >
                            <Text fontWeight="medium" color="gray.700">
                              {fieldChange.field}:
                            </Text>
                            <Text color="red.600" textDecoration="line-through">
                              {JSON.stringify(fieldChange.before)}
                            </Text>
                            <Text>→</Text>
                            <Text color="green.600">
                              {JSON.stringify(fieldChange.after)}
                            </Text>
                          </HStack>
                        ))}
                      </VStack>
                    )}

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
                        {JSON.stringify(change.before.data?.config || {}, null, 2)}
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
                        {JSON.stringify(change.after.data?.config || {}, null, 2)}
                      </Code>
                    </Box>
                  </Box>
                ))}
              </VStack>
            </AccordionPanel>
          </AccordionItem>
        )}
      </Accordion>
    </VStack>
  );
}
