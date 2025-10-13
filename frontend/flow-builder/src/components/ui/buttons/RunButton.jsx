import { Button } from '@chakra-ui/react';
import { FiPlay, FiStopCircle } from 'react-icons/fi';

export default function RunButton({
  onClick,
  isRunning = false,
  isDisabled = false,
  size = 'sm'
}) {
  return (
    <Button
      size={size}
      leftIcon={isRunning ? <FiStopCircle /> : <FiPlay />}
      colorScheme={isRunning ? 'red' : 'green'}
      onClick={onClick}
      isDisabled={isDisabled}
      variant="solid"
    >
      {isRunning ? 'Running' : 'Run'}
    </Button>
  );
}
