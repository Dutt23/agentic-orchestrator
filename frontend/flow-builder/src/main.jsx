import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.jsx'
import { ChakraProvider } from '@chakra-ui/react';
import { WebSocketProvider } from './contexts/WebSocketContext';
import ErrorBoundary from './components/ErrorBoundary';

const username = import.meta.env.VITE_DEV_USERNAME || 'test-user';

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <ErrorBoundary>
      <ChakraProvider>
        <WebSocketProvider username={username}>
          <App />
        </WebSocketProvider>
      </ChakraProvider>
    </ErrorBoundary>
  </StrictMode>,
)
