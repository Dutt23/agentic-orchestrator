import {
  FormControl,
  FormLabel,
  FormHelperText,
  FormErrorMessage,
  Input,
  Textarea,
  NumberInput,
  NumberInputField,
  Select,
} from '@chakra-ui/react';

/**
 * Reusable form field with label, input, helper text, and error handling
 *
 * @example
 * <FormField
 *   label="Workflow Name"
 *   value={name}
 *   onChange={setName}
 *   placeholder="Enter name"
 * />
 *
 * @example
 * <FormField
 *   type="textarea"
 *   label="Description"
 *   value={description}
 *   onChange={setDescription}
 *   error="Description is required"
 *   helperText="Provide a brief description"
 *   rows={4}
 * />
 *
 * @example
 * <FormField
 *   type="select"
 *   label="Type"
 *   value={type}
 *   onChange={setType}
 *   options={['http', 'agent', 'task']}
 * />
 */
export default function FormField({
  label,
  type = 'text',
  value,
  onChange,
  error,
  helperText,
  placeholder,
  required = false,
  disabled = false,
  mono = false,
  size = 'sm',
  rows = 3,
  options = [],
  ...inputProps
}) {
  const isInvalid = !!error;

  const renderInput = () => {
    switch (type) {
      case 'textarea':
        return (
          <Textarea
            value={value || ''}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder || `Enter ${label.toLowerCase()}`}
            fontFamily={mono ? 'monospace' : 'inherit'}
            size={size}
            rows={rows}
            isDisabled={disabled}
            {...inputProps}
          />
        );

      case 'number':
        return (
          <NumberInput
            value={value || ''}
            onChange={(valueString) => onChange(valueString)}
            size={size}
            isDisabled={disabled}
            {...inputProps}
          >
            <NumberInputField placeholder={placeholder || `Enter ${label.toLowerCase()}`} />
          </NumberInput>
        );

      case 'select':
        return (
          <Select
            value={value || ''}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder || `Select ${label.toLowerCase()}`}
            size={size}
            isDisabled={disabled}
            {...inputProps}
          >
            {options.map((option) => (
              <option key={option.value || option} value={option.value || option}>
                {option.label || option}
              </option>
            ))}
          </Select>
        );

      default:
        return (
          <Input
            type={type}
            value={value || ''}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder || `Enter ${label.toLowerCase()}`}
            fontFamily={mono ? 'monospace' : 'inherit'}
            size={size}
            isDisabled={disabled}
            {...inputProps}
          />
        );
    }
  };

  return (
    <FormControl isInvalid={isInvalid} isRequired={required}>
      <FormLabel fontSize="sm">{label}</FormLabel>
      {renderInput()}
      {helperText && !isInvalid && <FormHelperText>{helperText}</FormHelperText>}
      {isInvalid && <FormErrorMessage>{error}</FormErrorMessage>}
    </FormControl>
  );
}
