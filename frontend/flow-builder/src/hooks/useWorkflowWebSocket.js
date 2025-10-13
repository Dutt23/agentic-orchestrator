import { useEffect, useRef, useState, useCallback } from 'react';

const FANOUT_WS_URL = import.meta.env.VITE_FANOUT_WS_URL || 'ws://localhost:8084';
const RECONNECT_DELAY = 3000; // 3 seconds
const MAX_RECONNECT_ATTEMPTS = 5;

/**
 * Custom hook to manage WebSocket connection to fanout service
 * Automatically connects, handles reconnection, and provides event stream
 *
 * @param {string} username - Username for channel subscription
 * @param {function} onEvent - Callback when an event is received
 * @returns {Object} WebSocket state and methods
 */
export function useWorkflowWebSocket(username, onEvent) {
  const [isConnected, setIsConnected] = useState(false);
  const [connectionError, setConnectionError] = useState(null);
  const wsRef = useRef(null);
  const reconnectAttempts = useRef(0);
  const reconnectTimeoutRef = useRef(null);
  const shouldReconnect = useRef(true);
  const isConnecting = useRef(false); // Guard against duplicate connections

  // Store onEvent callback in a ref to avoid re-creating connect function
  const onEventRef = useRef(onEvent);
  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  const connect = useCallback(() => {
    if (!username) {
      console.warn('useWorkflowWebSocket: No username provided, skipping connection');
      return;
    }

    // Guard against duplicate connections
    if (isConnecting.current || (wsRef.current && wsRef.current.readyState === WebSocket.OPEN)) {
      console.log('[WebSocket] Already connected or connecting, skipping duplicate connection');
      return;
    }

    isConnecting.current = true;

    // Clean up existing connection
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    try {
      const wsUrl = `${FANOUT_WS_URL}/ws?username=${encodeURIComponent(username)}`;
      console.log('[WebSocket] Connecting to:', wsUrl);

      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log('[WebSocket] Connected successfully');
        setIsConnected(true);
        setConnectionError(null);
        reconnectAttempts.current = 0;
        isConnecting.current = false;
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          console.log('[WebSocket] Event received:', data);

          // Use the ref to get the latest callback without causing re-renders
          if (onEventRef.current) {
            onEventRef.current(data);
          }
        } catch (error) {
          console.error('[WebSocket] Failed to parse message:', error);
        }
      };

      ws.onerror = (error) => {
        console.error('[WebSocket] Error:', error);
        setConnectionError('WebSocket connection error');
        isConnecting.current = false;
      };

      ws.onclose = (event) => {
        console.log('[WebSocket] Disconnected:', event.code, event.reason);
        setIsConnected(false);
        wsRef.current = null;
        isConnecting.current = false;

        // Attempt reconnection if enabled and not manually closed
        if (shouldReconnect.current && reconnectAttempts.current < MAX_RECONNECT_ATTEMPTS) {
          reconnectAttempts.current += 1;
          console.log(`[WebSocket] Reconnecting... (attempt ${reconnectAttempts.current}/${MAX_RECONNECT_ATTEMPTS})`);

          reconnectTimeoutRef.current = setTimeout(() => {
            connect();
          }, RECONNECT_DELAY);
        } else if (reconnectAttempts.current >= MAX_RECONNECT_ATTEMPTS) {
          console.error('[WebSocket] Max reconnection attempts reached');
          setConnectionError('Failed to connect after multiple attempts');
        }
      };
    } catch (error) {
      console.error('[WebSocket] Failed to create connection:', error);
      setConnectionError(error.message);
      isConnecting.current = false;
    }
  }, [username]); // Only username as dependency now

  const disconnect = useCallback(() => {
    console.log('[WebSocket] Manually disconnecting');
    shouldReconnect.current = false;

    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    setIsConnected(false);
  }, []);

  const reconnect = useCallback(() => {
    console.log('[WebSocket] Manual reconnect requested');
    shouldReconnect.current = true;
    reconnectAttempts.current = 0;
    disconnect();
    setTimeout(connect, 100);
  }, [connect, disconnect]);

  // Auto-connect on mount and when username changes
  useEffect(() => {
    shouldReconnect.current = true;
    connect();

    // Cleanup on unmount
    return () => {
      shouldReconnect.current = false;
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  return {
    isConnected,
    connectionError,
    reconnect,
    disconnect,
  };
}
