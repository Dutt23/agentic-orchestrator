import { useState } from 'react';
import {
  Box,
  Button,
  FormControl,
  FormLabel,
  FormHelperText,
  Textarea,
  VStack,
  Alert,
  AlertIcon,
  AlertDescription,
} from '@chakra-ui/react';

/**
 * Form for entering workflow input parameters
 * Accepts JSON input and validates before submission
 */
export default function WorkflowInputsForm({ onSubmit, isSubmitting = false }) {
  const [inputJson, setInputJson] = useState('{}');
  const [parseError, setParseError] = useState(null);

  const handleInputChange = (e) => {
    const value = e.target.value;
    setInputJson(value);

    // Validate JSON as user types
    try {
      JSON.parse(value);
      setParseError(null);
    } catch (error) {
      setParseError(error.message);
    }
  };

  const handleSubmit = (e) => {
    e.preventDefault();

    // Final validation before submission
    try {
      const inputs = JSON.parse(inputJson);
      onSubmit(inputs);
    } catch (error) {
      setParseError(error.message);
    }
  };

  const isValid = !parseError && inputJson.trim() !== '';

  return (
    <Box as="form" onSubmit={handleSubmit}>
      <VStack spacing={4} align="stretch">
        <FormControl isInvalid={!!parseError}>
          <FormLabel>Workflow Inputs (JSON)</FormLabel>
          <Textarea
            value={inputJson}
            onChange={handleInputChange}
            placeholder='{"key": "value"}'
            rows={8}
            fontFamily="monospace"
            fontSize="sm"
            isDisabled={isSubmitting}
          />
          <FormHelperText>
            Enter workflow inputs as a JSON object. Leave as {`{}`} for no inputs.
          </FormHelperText>
        </FormControl>

        {parseError && (
          <Alert status="error" borderRadius="md">
            <AlertIcon />
            <AlertDescription fontSize="sm">
              Invalid JSON: {parseError}
            </AlertDescription>
          </Alert>
        )}

        <Button
          type="submit"
          colorScheme="blue"
          isDisabled={!isValid || isSubmitting}
          isLoading={isSubmitting}
          loadingText="Starting Workflow..."
          size="md"
          width="full"
        >
          Start Workflow
        </Button>
      </VStack>
    </Box>
  );
}
