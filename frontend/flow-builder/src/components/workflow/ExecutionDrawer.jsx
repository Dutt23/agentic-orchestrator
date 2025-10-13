import { useState } from 'react';
import {
  Drawer,
  DrawerBody,
  DrawerHeader,
  DrawerOverlay,
  DrawerContent,
  DrawerCloseButton,
  VStack,
  Heading,
  Text,
  Divider,
  Badge,
  HStack,
  Box,
  Alert,
  AlertIcon,
  AlertDescription,
} from '@chakra-ui/react';
import WorkflowInputsForm from './WorkflowInputsForm';
import ExecutionEvents from './ExecutionEvents';

/**
 * Drawer for workflow execution
 * Shows inputs form before execution, then displays real-time events
 */
export default function ExecutionDrawer({
  isOpen,
  onClose,
  workflowTag,
  onRunWorkflow,
  isRunning,
  events,
  connectionStatus,
  error,
}) {
  const [hasStarted, setHasStarted] = useState(false);

  const handleSubmit = async (inputs) => {
    setHasStarted(true);
    await onRunWorkflow(inputs);
  };

  const handleClose = () => {
    setHasStarted(false);
    onClose();
  };

  return (
    <Drawer isOpen={isOpen} placement="right" onClose={handleClose} size="md">
      <DrawerOverlay />
      <DrawerContent>
        <DrawerCloseButton />
        <DrawerHeader borderBottomWidth="1px">
          <VStack align="start" spacing={2}>
            <Heading size="md">Execute Workflow</Heading>
            <HStack spacing={2}>
              <Text fontSize="sm" color="gray.600" fontWeight="normal">
                Tag: <Badge colorScheme="blue">{workflowTag}</Badge>
              </Text>
              {connectionStatus && (
                <Badge
                  colorScheme={connectionStatus.isConnected ? 'green' : 'red'}
                  fontSize="xs"
                >
                  {connectionStatus.isConnected ? 'Connected' : 'Disconnected'}
                </Badge>
              )}
            </HStack>
          </VStack>
        </DrawerHeader>

        <DrawerBody py={6}>
          <VStack spacing={6} align="stretch">
            {/* Connection Error Alert */}
            {connectionStatus?.error && (
              <Alert status="warning" borderRadius="md">
                <AlertIcon />
                <AlertDescription fontSize="sm">
                  WebSocket disconnected: {connectionStatus.error}
                </AlertDescription>
              </Alert>
            )}

            {/* Execution Error Alert */}
            {error && (
              <Alert status="error" borderRadius="md">
                <AlertIcon />
                <AlertDescription fontSize="sm">{error}</AlertDescription>
              </Alert>
            )}

            {/* Inputs Form - Show only before workflow starts */}
            {!hasStarted && (
              <Box>
                <Heading size="sm" mb={4}>
                  Workflow Inputs
                </Heading>
                <WorkflowInputsForm
                  onSubmit={handleSubmit}
                  isSubmitting={isRunning}
                />
              </Box>
            )}

            {/* Execution Events - Show after workflow starts */}
            {hasStarted && (
              <>
                <Divider />
                <Box>
                  <HStack mb={4} justify="space-between">
                    <Heading size="sm">Execution Events</Heading>
                    {isRunning && (
                      <Badge colorScheme="blue" fontSize="sm">
                        Running
                      </Badge>
                    )}
                    {!isRunning && events.length > 0 && (
                      <Badge colorScheme="green" fontSize="sm">
                        Completed
                      </Badge>
                    )}
                  </HStack>
                  <ExecutionEvents events={events} />
                </Box>
              </>
            )}
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  );
}
