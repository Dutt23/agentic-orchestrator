import { FormControl, FormLabel, Input } from '@chakra-ui/react';

export default function TextDisplay({ label, value, onChange, placeholder, mono = false }) {
  return (
    <FormControl>
      <FormLabel fontSize="sm">{label}</FormLabel>
      <Input
        size="sm"
        value={value || ''}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder || `Enter ${label.toLowerCase()}`}
        fontFamily={mono ? 'mono' : 'inherit'}
      />
    </FormControl>
  );
}
