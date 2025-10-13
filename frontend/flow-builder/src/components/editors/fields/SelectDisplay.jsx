import { FormControl, FormLabel, Select } from '@chakra-ui/react';

export default function SelectDisplay({ label, value, onChange, options = [], placeholder }) {
  return (
    <FormControl>
      <FormLabel fontSize="sm">{label}</FormLabel>
      <Select
        size="sm"
        value={value || ''}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder || `Select ${label.toLowerCase()}`}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </Select>
    </FormControl>
  );
}
