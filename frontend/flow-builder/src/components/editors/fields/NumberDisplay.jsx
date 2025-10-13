import { FormControl, FormLabel, NumberInput, NumberInputField, Text } from '@chakra-ui/react';

export default function NumberDisplay({ label, value, onChange, placeholder, min = 0, helperText }) {
  return (
    <FormControl>
      <FormLabel fontSize="sm">{label}</FormLabel>
      <NumberInput
        size="sm"
        value={value || 0}
        onChange={(valueString) => onChange(parseInt(valueString) || 0)}
        min={min}
      >
        <NumberInputField placeholder={placeholder || `Enter ${label.toLowerCase()}`} />
      </NumberInput>
      {helperText && (
        <Text fontSize="xs" color="gray.500" mt={1}>
          {helperText}
        </Text>
      )}
    </FormControl>
  );
}
