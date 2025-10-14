import { ButtonGroup, Button } from '@chakra-ui/react';

/**
 * Reusable toggle button group for view mode switching
 *
 * @example
 * <ToggleButtons
 *   options={[
 *     { value: 'execution', label: 'Execution View' },
 *     { value: 'patch', label: 'Patch Overlay' }
 *   ]}
 *   value={currentView}
 *   onChange={setCurrentView}
 * />
 *
 * @example
 * <ToggleButtons
 *   options={[
 *     { value: 'side', label: 'Side-by-Side', icon: FiColumns },
 *     { value: 'overlay', label: 'Overlay', icon: FiLayers }
 *   ]}
 *   value={viewMode}
 *   onChange={setViewMode}
 *   colorScheme="purple"
 *   fullWidth
 * />
 */
export default function ToggleButtons({
  options = [],
  value,
  onChange,
  size = 'sm',
  colorScheme = 'blue',
  inactiveColorScheme = 'gray',
  fullWidth = false,
  ...buttonGroupProps
}) {
  if (!options || options.length === 0) {
    return null;
  }

  return (
    <ButtonGroup
      size={size}
      isAttached
      variant="outline"
      w={fullWidth ? '100%' : 'auto'}
      {...buttonGroupProps}
    >
      {options.map((option) => {
        const isActive = value === option.value;
        const ButtonIcon = option.icon;

        return (
          <Button
            key={option.value}
            flex={fullWidth ? '1' : undefined}
            leftIcon={ButtonIcon ? <ButtonIcon /> : undefined}
            onClick={() => onChange(option.value)}
            colorScheme={isActive ? colorScheme : inactiveColorScheme}
            variant={isActive ? 'solid' : 'outline'}
          >
            {option.label}
          </Button>
        );
      })}
    </ButtonGroup>
  );
}
