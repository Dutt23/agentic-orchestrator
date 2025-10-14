import {
  Box,
  VStack,
  HStack,
  Text,
  Badge,
  Divider,
} from '@chakra-ui/react';
import { formatTimestamp } from '../../utils/dateUtils';

/**
 * NodeMetricsDisplay shows detailed performance metrics for a node
 */
export default function NodeMetricsDisplay({ metrics }) {
  if (!metrics) {
    return (
      <Text fontSize="sm" color="gray.500">
        No metrics available
      </Text>
    );
  }

  return (
    <Box
      p={4}
      bg="gray.50"
      borderRadius="md"
      border="1px solid"
      borderColor="gray.200"
    >
      <VStack align="stretch" spacing={3}>
        {/* Timing Metrics */}
        <Box>
          <Text fontSize="sm" fontWeight="semibold" color="blue.600" mb={2}>
            Timing
          </Text>
          <VStack align="stretch" spacing={2}>
            <HStack justify="space-between">
              <Text fontSize="sm" color="gray.600">Execution Time:</Text>
              <Badge colorScheme="blue">{metrics.execution_time_ms}ms</Badge>
            </HStack>
            <HStack justify="space-between">
              <Text fontSize="sm" color="gray.600">Queue Time:</Text>
              <Badge colorScheme="orange">{metrics.queue_time_ms}ms</Badge>
            </HStack>
            <HStack justify="space-between">
              <Text fontSize="sm" color="gray.600">Total Duration:</Text>
              <Badge colorScheme="purple">{metrics.total_duration_ms}ms</Badge>
            </HStack>
          </VStack>
        </Box>

        {/* Memory Metrics */}
        {(metrics.memory_peak_mb > 0 || metrics.memory_start_mb > 0) && (
          <>
            <Divider />
            <Box>
              <Text fontSize="sm" fontWeight="semibold" color="green.600" mb={2}>
                Memory Usage
              </Text>
              <VStack align="stretch" spacing={2}>
                {metrics.memory_start_mb > 0 && (
                  <HStack justify="space-between">
                    <Text fontSize="sm" color="gray.600">Start:</Text>
                    <Text fontSize="sm" fontWeight="medium">
                      {metrics.memory_start_mb.toFixed(1)} MB
                    </Text>
                  </HStack>
                )}
                <HStack justify="space-between">
                  <Text fontSize="sm" color="gray.600">Peak:</Text>
                  <Text fontSize="sm" fontWeight="bold" color="green.600">
                    {metrics.memory_peak_mb.toFixed(1)} MB
                  </Text>
                </HStack>
                {metrics.memory_end_mb > 0 && (
                  <HStack justify="space-between">
                    <Text fontSize="sm" color="gray.600">End:</Text>
                    <Text fontSize="sm" fontWeight="medium">
                      {metrics.memory_end_mb.toFixed(1)} MB
                    </Text>
                  </HStack>
                )}
              </VStack>
            </Box>
          </>
        )}

        {/* CPU & Thread Metrics */}
        {(metrics.cpu_percent > 0 || metrics.thread_count > 0) && (
          <>
            <Divider />
            <Box>
              <Text fontSize="sm" fontWeight="semibold" color="orange.600" mb={2}>
                Resource Usage
              </Text>
              <VStack align="stretch" spacing={2}>
                {metrics.cpu_percent > 0 && (
                  <HStack justify="space-between">
                    <Text fontSize="sm" color="gray.600">CPU:</Text>
                    <Text fontSize="sm" fontWeight="medium">
                      {metrics.cpu_percent.toFixed(1)}%
                    </Text>
                  </HStack>
                )}
                {metrics.thread_count > 0 && (
                  <HStack justify="space-between">
                    <Text fontSize="sm" color="gray.600">Threads:</Text>
                    <Text fontSize="sm" fontWeight="medium">
                      {metrics.thread_count}
                    </Text>
                  </HStack>
                )}
              </VStack>
            </Box>
          </>
        )}

        {/* Timestamps */}
        <Divider />
        <Box>
          <Text fontSize="sm" fontWeight="semibold" color="gray.600" mb={2}>
            Timestamps
          </Text>
          <VStack align="stretch" spacing={1}>
            <HStack justify="space-between">
              <Text fontSize="xs" color="gray.500">Sent:</Text>
              <Text fontSize="xs">{formatTimestamp(metrics.sent_at)}</Text>
            </HStack>
            <HStack justify="space-between">
              <Text fontSize="xs" color="gray.500">Started:</Text>
              <Text fontSize="xs">{formatTimestamp(metrics.start_time)}</Text>
            </HStack>
            <HStack justify="space-between">
              <Text fontSize="xs" color="gray.500">Ended:</Text>
              <Text fontSize="xs">{formatTimestamp(metrics.end_time)}</Text>
            </HStack>
          </VStack>
        </Box>
      </VStack>
    </Box>
  );
}
