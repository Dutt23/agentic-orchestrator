import { FormControl, FormLabel, Textarea } from '@chakra-ui/react';

export default function TextAreaDisplay({ label, value, onChange, placeholder, rows = 3, mono = false }) {
  return (
    <FormControl>
      <FormLabel fontSize="sm">{label}</FormLabel>
      <Textarea
        size="sm"
        value={value || ''}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder || `Enter ${label.toLowerCase()}...`}
        rows={rows}
        fontFamily={mono ? 'mono' : 'inherit'}
      />
    </FormControl>
  );
}
