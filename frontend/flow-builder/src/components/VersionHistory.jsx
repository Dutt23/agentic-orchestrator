import React, { useState } from 'react';
import {
  Box,
  VStack,
  Text,
  Badge,
  Divider,
  Heading,
  HStack,
  Icon,
  Button,
  Checkbox,
  ButtonGroup,
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverBody,
  PopoverArrow,
} from '@chakra-ui/react';
import { FiClock, FiUser, FiGitCommit, FiGitMerge, FiColumns, FiLayers } from 'react-icons/fi';

/**
 * VersionHistory Component
 * Displays the patch chain history for a workflow
 *
 * @param {Array} patches - Array of patch metadata from API
 * @param {Object} metadata - Workflow metadata (kind, depth, patch_count)
 * @param {Function} onCompareVersions - Callback when user wants to compare versions
 */
export default function VersionHistory({ patches = [], metadata = null, onCompareVersions }) {
  const [selectedVersions, setSelectedVersions] = useState([]);
  const [viewMode, setViewMode] = useState('sidebyside'); // 'sidebyside' or 'overlay'
  // If no patches (dag_version with no edits), show base version only
  if (!patches || patches.length === 0) {
    return (
      <Box>
        <Heading size="sm" mb={3}>Version History</Heading>
        <Box
          p={3}
          bg="gray.50"
          borderRadius="md"
          borderLeft="3px solid"
          borderColor="blue.400"
        >
          <HStack spacing={2} mb={1}>
            <Icon as={FiGitCommit} color="blue.500" />
            <Text fontWeight="semibold" fontSize="sm">
              Version 1 (Base)
            </Text>
          </HStack>
          <Text fontSize="xs" color="gray.600">
            Initial workflow version
          </Text>
          {metadata?.kind === 'dag_version' && (
            <Badge mt={2} colorScheme="blue" fontSize="xs">
              No patches
            </Badge>
          )}
        </Box>
      </Box>
    );
  }

  // Format date helper
  const formatDate = (dateString) => {
    if (!dateString) return 'Unknown';
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  // Handle version selection
  const handleVersionToggle = (seq) => {
    setSelectedVersions(prev => {
      if (prev.includes(seq)) {
        return prev.filter(v => v !== seq);
      }
      if (prev.length >= 2) {
        // Only allow 2 selections
        return [prev[1], seq];
      }
      return [...prev, seq];
    });
  };

  // Handle compare button
  const handleCompare = () => {
    if (selectedVersions.length === 2 && onCompareVersions) {
      const [v1, v2] = selectedVersions.sort((a, b) => a - b);
      onCompareVersions(v1, v2, viewMode);
    }
  };

  return (
    <Box>
      <HStack justify="space-between" mb={3}>
        <Heading size="sm">
          Version History
          {metadata?.patch_count > 0 && (
            <Badge ml={2} colorScheme="purple" fontSize="xs">
              {metadata.patch_count} {metadata.patch_count === 1 ? 'patch' : 'patches'}
            </Badge>
          )}
        </Heading>
        {selectedVersions.length === 2 && (
          <HStack spacing={2}>
            <Popover placement="bottom-end">
              <PopoverTrigger>
                <Button
                  size="xs"
                  variant="outline"
                  colorScheme="gray"
                  leftIcon={<Icon as={viewMode === 'overlay' ? FiLayers : FiColumns} />}
                >
                  {viewMode === 'overlay' ? 'Overlay' : 'Side-by-Side'}
                </Button>
              </PopoverTrigger>
              <PopoverContent width="200px">
                <PopoverArrow />
                <PopoverBody p={2}>
                  <ButtonGroup size="xs" orientation="vertical" width="100%" spacing={1}>
                    <Button
                      leftIcon={<Icon as={FiColumns} />}
                      onClick={() => setViewMode('sidebyside')}
                      colorScheme={viewMode === 'sidebyside' ? 'blue' : 'gray'}
                      variant={viewMode === 'sidebyside' ? 'solid' : 'ghost'}
                      justifyContent="flex-start"
                    >
                      Side-by-Side
                    </Button>
                    <Button
                      leftIcon={<Icon as={FiLayers} />}
                      onClick={() => setViewMode('overlay')}
                      colorScheme={viewMode === 'overlay' ? 'blue' : 'gray'}
                      variant={viewMode === 'overlay' ? 'solid' : 'ghost'}
                      justifyContent="flex-start"
                    >
                      Overlay
                    </Button>
                  </ButtonGroup>
                </PopoverBody>
              </PopoverContent>
            </Popover>
            <Button
              size="xs"
              colorScheme="blue"
              leftIcon={<Icon as={FiGitMerge} />}
              onClick={handleCompare}
            >
              Compare
            </Button>
          </HStack>
        )}
      </HStack>

      <VStack spacing={3} align="stretch">
        {/* Show patches in reverse order (newest first) */}
        {[...patches].reverse().map((patch, index) => {
          const isLatest = index === 0;
          const displaySeq = patches.length - index;
          const isSelected = selectedVersions.includes(displaySeq);

          return (
            <Box
              key={patch.seq || index}
              p={3}
              bg={isSelected ? 'purple.50' : (isLatest ? 'blue.50' : 'gray.50')}
              borderRadius="md"
              borderLeft="3px solid"
              borderColor={isSelected ? 'purple.400' : (isLatest ? 'blue.400' : 'gray.300')}
              _hover={{
                bg: isSelected ? 'purple.100' : (isLatest ? 'blue.100' : 'gray.100'),
                cursor: 'pointer'
              }}
              transition="background 0.2s"
              onClick={() => handleVersionToggle(displaySeq)}
            >
              <HStack justify="space-between" mb={2}>
                <HStack spacing={2}>
                  <Checkbox
                    isChecked={isSelected}
                    onChange={() => handleVersionToggle(displaySeq)}
                    onClick={(e) => e.stopPropagation()}
                    size="sm"
                    colorScheme="purple"
                  />
                  <Icon as={FiGitCommit} color={isLatest ? 'blue.500' : 'gray.500'} />
                  <Text fontWeight="semibold" fontSize="sm">
                    Version {displaySeq}
                  </Text>
                  {isLatest && (
                    <Badge colorScheme="green" fontSize="xs">
                      Current
                    </Badge>
                  )}
                </HStack>
                {patch.depth && (
                  <Text fontSize="xs" color="gray.500">
                    Depth: {patch.depth}
                  </Text>
                )}
              </HStack>

              {/* Operation count */}
              {patch.op_count && (
                <Text fontSize="xs" color="gray.600" mb={1}>
                  {patch.op_count} {patch.op_count === 1 ? 'operation' : 'operations'}
                </Text>
              )}

              <Divider my={2} />

              {/* Metadata */}
              <VStack align="stretch" spacing={1}>
                {patch.created_by && (
                  <HStack spacing={1} fontSize="xs" color="gray.600">
                    <Icon as={FiUser} boxSize={3} />
                    <Text>{patch.created_by}</Text>
                  </HStack>
                )}
                {patch.created_at && (
                  <HStack spacing={1} fontSize="xs" color="gray.600">
                    <Icon as={FiClock} boxSize={3} />
                    <Text>{formatDate(patch.created_at)}</Text>
                  </HStack>
                )}
              </VStack>

              {/* Artifact ID (truncated) */}
              {patch.artifact_id && (
                <Text fontSize="xs" color="gray.400" mt={2} fontFamily="mono">
                  {patch.artifact_id.substring(0, 8)}...
                </Text>
              )}
            </Box>
          );
        })}

        {/* Base version */}
        <Box
          p={3}
          bg="gray.50"
          borderRadius="md"
          borderLeft="3px solid"
          borderColor="gray.300"
        >
          <HStack spacing={2} mb={1}>
            <Icon as={FiGitCommit} color="gray.500" />
            <Text fontWeight="semibold" fontSize="sm">
              Version 0 (Base)
            </Text>
          </HStack>
          <Text fontSize="xs" color="gray.600">
            Initial workflow (versions 1-{patches.length} show changes from here)
          </Text>
        </Box>
      </VStack>
    </Box>
  );
}
