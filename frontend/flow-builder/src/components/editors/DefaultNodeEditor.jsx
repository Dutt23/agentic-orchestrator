import { Box, Text, VStack } from '@chakra-ui/react';
import { useState } from 'react';
import { TextDisplay, TextAreaDisplay, NumberDisplay, SelectDisplay, ListDisplay, ConditionalListDisplay } from './fields';

export default function DefaultNodeEditor({ node, onUpdate }) {
  const [config, setConfig] = useState(node.data.config || {});

  const handleChange = (field, value) => {
    const newConfig = { ...config, [field]: value };
    setConfig(newConfig);
    if (onUpdate) {
      onUpdate({
        ...node,
        data: {
          ...node.data,
          config: newConfig,
        },
      });
    }
  };

  const renderConfigField = (key, value) => {
    const fieldName = key.split('_').map(word =>
      word.charAt(0).toUpperCase() + word.slice(1)
    ).join(' ');

    // Determine component type based on key name and value type
    // Special handling for conditions array (If/Else node)
    if (key === 'conditions' && Array.isArray(value)) {
      return (
        <ConditionalListDisplay
          key={key}
          label="If"
          value={value}
          onChange={(newValue) => handleChange(key, newValue)}
        />
      );
    } else if (key === 'method') {
      // HTTP method select
      return (
        <SelectDisplay
          key={key}
          label={fieldName}
          value={value}
          onChange={(newValue) => handleChange(key, newValue)}
          options={[
            { value: 'GET', label: 'GET' },
            { value: 'POST', label: 'POST' },
            { value: 'PUT', label: 'PUT' },
            { value: 'DELETE', label: 'DELETE' },
            { value: 'PATCH', label: 'PATCH' }
          ]}
        />
      );
    } else if (Array.isArray(value)) {
      // Regular list/Array
      return (
        <ListDisplay
          key={key}
          label={fieldName}
          value={value}
          onChange={(newValue) => handleChange(key, newValue)}
        />
      );
    } else if (key.includes('prompt') || key.includes('message') || key.includes('condition') || key.includes('goal')) {
      // Multi-line text
      return (
        <TextAreaDisplay
          key={key}
          label={fieldName}
          value={value}
          onChange={(newValue) => handleChange(key, newValue)}
          mono={key.includes('condition')}
        />
      );
    } else if (typeof value === 'number' || key.includes('timeout') || key.includes('iterations')) {
      // Number input
      return (
        <NumberDisplay
          key={key}
          label={fieldName}
          value={value}
          onChange={(newValue) => handleChange(key, newValue)}
        />
      );
    } else {
      // Single-line text
      return (
        <TextDisplay
          key={key}
          label={fieldName}
          value={value}
          onChange={(newValue) => handleChange(key, newValue)}
          mono={key.includes('path') || key.includes('id')}
        />
      );
    }
  };

  // Show outputs info if defined
  const outputs = node.data.outputs;

  return (
    <VStack spacing={4} align="stretch">
      <Text fontSize="md" fontWeight="bold">
        {node.data.label || node.data.type || 'Node'} Configuration
      </Text>

      {config && Object.keys(config).length > 0 ? (
        Object.entries(config).map(([key, value]) => {
          // Skip rendering complex objects (but not arrays)
          if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
            return null;
          }
          return renderConfigField(key, value);
        })
      ) : (
        <Text fontSize="sm" color="gray.500">No configuration available</Text>
      )}

      {outputs && outputs.length > 0 && (
        <Box p={3} bg="blue.50" borderRadius="md">
          <Text fontSize="xs" fontWeight="medium" color="blue.800">
            Outputs: {outputs.join(', ')}
          </Text>
          <Text fontSize="xs" color="blue.600" mt={1}>
            This node has multiple output handles
          </Text>
        </Box>
      )}

      <Box fontSize="xs" color="gray.600" pt={2} borderTop="1px" borderColor="gray.200">
        <Text><strong>Node Type:</strong> {node.data.type}</Text>
        <Text><strong>Node ID:</strong> {node.id}</Text>
      </Box>
    </VStack>
  );
}
