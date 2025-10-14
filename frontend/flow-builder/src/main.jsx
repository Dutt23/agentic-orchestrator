import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.jsx'
import { ChakraProvider } from '@chakra-ui/react';
import { WebSocketProvider } from './contexts/WebSocketContext';

const username = import.meta.env.VITE_DEV_USERNAME || 'test-user';

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <ChakraProvider>
      <WebSocketProvider username={username}>
        <App />
      </WebSocketProvider>
    </ChakraProvider>
  </StrictMode>,
)
