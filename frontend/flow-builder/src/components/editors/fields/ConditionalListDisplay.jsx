import { FormControl, FormLabel, VStack, HStack, Input, Textarea, IconButton, Button, Text, Box } from '@chakra-ui/react';
import { FiPlus, FiX, FiTrash2 } from 'react-icons/fi';
import { useState } from 'react';

export default function ConditionalListDisplay({ label, value = [], onChange }) {
  const [conditions, setConditions] = useState(
    Array.isArray(value) && value.length > 0
      ? value
      : [{ label: '', condition: '' }]
  );

  const handleAdd = () => {
    const newConditions = [...conditions, { label: '', condition: '' }];
    setConditions(newConditions);
    onChange(newConditions);
  };

  const handleRemove = (index) => {
    // Don't allow removing the last condition
    if (conditions.length === 1) return;

    const newConditions = conditions.filter((_, i) => i !== index);
    setConditions(newConditions);
    onChange(newConditions);
  };

  const handleLabelChange = (index, newLabel) => {
    const newConditions = [...conditions];
    newConditions[index] = { ...newConditions[index], label: newLabel };
    setConditions(newConditions);
    onChange(newConditions);
  };

  const handleConditionChange = (index, newCondition) => {
    const newConditions = [...conditions];
    newConditions[index] = { ...newConditions[index], condition: newCondition };
    setConditions(newConditions);
    onChange(newConditions);
  };

  return (
    <FormControl>
      <FormLabel fontSize="sm">{label}</FormLabel>
      <VStack spacing={4} align="stretch">
        {conditions.map((condition, index) => (
          <Box key={index} p={3} borderRadius="md" border="1px" borderColor="gray.200" bg="gray.50">
            <HStack justify="space-between" mb={2}>
              <Text fontSize="xs" fontWeight="bold" color="gray.700">
                {label} {index > 0 ? index + 1 : ''}
              </Text>
              {conditions.length > 1 && (
                <IconButton
                  size="xs"
                  icon={<FiTrash2 />}
                  onClick={() => handleRemove(index)}
                  aria-label="Remove condition"
                  colorScheme="red"
                  variant="ghost"
                />
              )}
            </HStack>
            <VStack spacing={2} align="stretch">
              <Input
                size="sm"
                value={condition.label || ''}
                onChange={(e) => handleLabelChange(index, e.target.value)}
                placeholder="Enter label (e.g., 'ff')"
                bg="white"
              />
              <Textarea
                size="sm"
                value={condition.condition || ''}
                onChange={(e) => handleConditionChange(index, e.target.value)}
                placeholder="Enter condition, e.g. input == 5"
                fontFamily="monospace"
                fontSize="xs"
                rows={3}
                bg="white"
              />
            </VStack>
          </Box>
        ))}
        <Button
          size="sm"
          leftIcon={<FiPlus />}
          onClick={handleAdd}
          variant="outline"
          colorScheme="blue"
        >
          Add
        </Button>
        <Text fontSize="xs" color="gray.600">
          Use Common Expression Language to create a custom expression.{' '}
          <Text as="span" color="blue.600" textDecoration="underline" cursor="pointer">
            Learn more.
          </Text>
        </Text>

        {/* Else Block */}
        <Box p={3} borderRadius="md" border="1px" borderColor="gray.200" bg="gray.50" mt={2}>
          <Text fontSize="xs" fontWeight="bold" color="gray.700" mb={2}>
            Else
          </Text>
          <Text fontSize="xs" color="gray.500">
            Default path when no conditions match
          </Text>
        </Box>
      </VStack>
    </FormControl>
  );
}
