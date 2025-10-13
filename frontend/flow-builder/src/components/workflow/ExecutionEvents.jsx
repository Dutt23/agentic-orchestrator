import { useEffect, useRef } from 'react';
import {
  Box,
  VStack,
  HStack,
  Text,
  Badge,
  Icon,
  Divider,
} from '@chakra-ui/react';
import {
  FiPlay,
  FiCheckCircle,
  FiAlertCircle,
  FiClock,
} from 'react-icons/fi';

/**
 * Display real-time workflow execution events
 * Shows event stream with type, timestamp, and details
 */
export default function ExecutionEvents({ events = [] }) {
  const bottomRef = useRef(null);

  // Auto-scroll to bottom when new events arrive
  useEffect(() => {
    if (bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [events]);

  const getEventIcon = (type) => {
    switch (type) {
      case 'workflow_started':
        return { icon: FiPlay, color: 'blue.500' };
      case 'node_completed':
        return { icon: FiCheckCircle, color: 'green.500' };
      case 'workflow_completed':
        return { icon: FiCheckCircle, color: 'green.500' };
      case 'node_failed':
        return { icon: FiAlertCircle, color: 'red.500' };
      default:
        return { icon: FiClock, color: 'gray.500' };
    }
  };

  const getEventColor = (type) => {
    switch (type) {
      case 'workflow_started':
        return 'blue';
      case 'node_completed':
        return 'green';
      case 'workflow_completed':
        return 'green';
      case 'node_failed':
        return 'red';
      default:
        return 'gray';
    }
  };

  const formatEventDetails = (event) => {
    const details = [];

    if (event.node_id) {
      details.push(`Node: ${event.node_id}`);
    }

    if (event.status) {
      details.push(`Status: ${event.status}`);
    }

    if (event.counter !== undefined) {
      details.push(`In-flight: ${event.counter}`);
    }

    if (event.nodes !== undefined) {
      details.push(`Total Nodes: ${event.nodes}`);
    }

    if (event.entry_nodes !== undefined) {
      details.push(`Entry Nodes: ${event.entry_nodes}`);
    }

    return details.join(' • ');
  };

  const formatTime = (timestamp) => {
    if (!timestamp) return '';
    const date = new Date(timestamp * 1000);
    return date.toLocaleTimeString();
  };

  if (events.length === 0) {
    return (
      <Box
        p={8}
        textAlign="center"
        color="gray.500"
        borderWidth={1}
        borderRadius="md"
        borderStyle="dashed"
      >
        <Icon as={FiClock} boxSize={8} mb={2} />
        <Text>No events yet. Start a workflow to see execution updates.</Text>
      </Box>
    );
  }

  return (
    <Box
      maxH="400px"
      overflowY="auto"
      borderWidth={1}
      borderRadius="md"
      bg="gray.50"
      p={4}
    >
      <VStack spacing={3} align="stretch">
        {events.map((event, index) => {
          const { icon, color } = getEventIcon(event.type);
          const badgeColor = getEventColor(event.type);

          return (
            <Box key={index}>
              <HStack spacing={3} align="start">
                <Icon as={icon} color={color} boxSize={5} mt={1} />
                <VStack spacing={1} align="start" flex={1}>
                  <HStack spacing={2}>
                    <Badge colorScheme={badgeColor} fontSize="xs">
                      {event.type.replace(/_/g, ' ').toUpperCase()}
                    </Badge>
                    <Text fontSize="xs" color="gray.500">
                      {formatTime(event.timestamp)}
                    </Text>
                  </HStack>
                  {formatEventDetails(event) && (
                    <Text fontSize="sm" color="gray.700">
                      {formatEventDetails(event)}
                    </Text>
                  )}
                  {event.run_id && (
                    <Text fontSize="xs" color="gray.400" fontFamily="monospace">
                      Run ID: {event.run_id.substring(0, 8)}...
                    </Text>
                  )}
                </VStack>
              </HStack>
              {index < events.length - 1 && <Divider mt={3} />}
            </Box>
          );
        })}
        <div ref={bottomRef} />
      </VStack>
    </Box>
  );
}
