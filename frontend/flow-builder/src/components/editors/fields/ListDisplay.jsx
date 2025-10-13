import { FormControl, FormLabel, VStack, HStack, Input, IconButton, Button, Text } from '@chakra-ui/react';
import { FiPlus, FiX } from 'react-icons/fi';
import { useState } from 'react';

export default function ListDisplay({ label, value = [], onChange, placeholder }) {
  const [items, setItems] = useState(Array.isArray(value) ? value : []);

  const handleAdd = () => {
    const newItems = [...items, ''];
    setItems(newItems);
    onChange(newItems);
  };

  const handleRemove = (index) => {
    const newItems = items.filter((_, i) => i !== index);
    setItems(newItems);
    onChange(newItems);
  };

  const handleChange = (index, newValue) => {
    const newItems = [...items];
    newItems[index] = newValue;
    setItems(newItems);
    onChange(newItems);
  };

  return (
    <FormControl>
      <FormLabel fontSize="sm">{label}</FormLabel>
      <VStack spacing={2} align="stretch">
        {items.map((item, index) => (
          <HStack key={index}>
            <Input
              size="sm"
              value={item}
              onChange={(e) => handleChange(index, e.target.value)}
              placeholder={placeholder || `Enter item ${index + 1}`}
            />
            <IconButton
              size="sm"
              icon={<FiX />}
              onClick={() => handleRemove(index)}
              aria-label="Remove item"
              colorScheme="red"
              variant="ghost"
            />
          </HStack>
        ))}
        <Button
          size="sm"
          leftIcon={<FiPlus />}
          onClick={handleAdd}
          variant="outline"
          colorScheme="blue"
        >
          Add Item
        </Button>
        {items.length === 0 && (
          <Text fontSize="xs" color="gray.500">No items added</Text>
        )}
      </VStack>
    </FormControl>
  );
}
