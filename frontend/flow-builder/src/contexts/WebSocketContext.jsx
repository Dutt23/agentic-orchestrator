import { createContext, useContext, useEffect, useRef, useState } from 'react';

const WebSocketContext = createContext(null);

const FANOUT_WS_URL = import.meta.env.VITE_FANOUT_WS_URL || 'ws://localhost:8084';
const RECONNECT_DELAY = 3000;
const MAX_RECONNECT_ATTEMPTS = 5;

/**
 * WebSocketProvider connects once at app level and provides events to all components
 */
export function WebSocketProvider({ username, children }) {
  const [isConnected, setIsConnected] = useState(false);
  const [connectionError, setConnectionError] = useState(null);
  const wsRef = useRef(null);
  const reconnectAttempts = useRef(0);
  const reconnectTimeoutRef = useRef(null);
  const shouldReconnect = useRef(true);
  const isConnecting = useRef(false);

  // Store event listeners by subscription ID
  const listenersRef = useRef(new Map());
  const nextSubscriptionId = useRef(0);

  const connect = () => {
    if (!username) {
      console.warn('[WebSocketProvider] No username, skipping connection');
      return;
    }

    if (isConnecting.current || (wsRef.current && wsRef.current.readyState === WebSocket.OPEN)) {
      console.log('[WebSocketProvider] Already connected or connecting');
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
      console.log('[WebSocketProvider] Connecting to:', wsUrl);

      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log('[WebSocketProvider] Connected');
        setIsConnected(true);
        setConnectionError(null);
        reconnectAttempts.current = 0;
        isConnecting.current = false;
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          console.log('[WebSocketProvider] Event received:', data);

          // Notify all subscribed listeners
          listenersRef.current.forEach((listener) => {
            listener(data);
          });
        } catch (error) {
          console.error('[WebSocketProvider] Failed to parse message:', error);
        }
      };

      ws.onerror = (error) => {
        console.error('[WebSocketProvider] Error:', error);
        setConnectionError('WebSocket connection error');
        isConnecting.current = false;
      };

      ws.onclose = (event) => {
        console.log('[WebSocketProvider] Disconnected:', event.code, event.reason);
        setIsConnected(false);
        wsRef.current = null;
        isConnecting.current = false;

        // Attempt reconnection
        if (shouldReconnect.current && reconnectAttempts.current < MAX_RECONNECT_ATTEMPTS) {
          reconnectAttempts.current += 1;
          console.log(`[WebSocketProvider] Reconnecting... (${reconnectAttempts.current}/${MAX_RECONNECT_ATTEMPTS})`);

          reconnectTimeoutRef.current = setTimeout(() => {
            connect();
          }, RECONNECT_DELAY);
        } else if (reconnectAttempts.current >= MAX_RECONNECT_ATTEMPTS) {
          console.error('[WebSocketProvider] Max reconnection attempts reached');
          setConnectionError('Failed to connect after multiple attempts');
        }
      };
    } catch (error) {
      console.error('[WebSocketProvider] Failed to create connection:', error);
      setConnectionError(error.message);
      isConnecting.current = false;
    }
  };

  // Connect on mount
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
  }, [username]);

  // Subscribe to events with an optional filter
  const subscribe = (callback, filter) => {
    const id = nextSubscriptionId.current++;

    // Wrap callback with filter if provided
    const wrappedCallback = filter
      ? (event) => {
          if (filter(event)) {
            callback(event);
          }
        }
      : callback;

    listenersRef.current.set(id, wrappedCallback);

    // Return unsubscribe function
    return () => {
      listenersRef.current.delete(id);
    };
  };

  const value = {
    isConnected,
    connectionError,
    subscribe,
  };

  return (
    <WebSocketContext.Provider value={value}>
      {children}
    </WebSocketContext.Provider>
  );
}

/**
 * Hook to subscribe to WebSocket events with optional filtering
 *
 * @param {function} onEvent - Callback when matching event received
 * @param {function} filter - Optional filter function (event) => boolean
 */
export function useWebSocketEvents(onEvent, filter) {
  const context = useContext(WebSocketContext);

  if (!context) {
    throw new Error('useWebSocketEvents must be used within WebSocketProvider');
  }

  useEffect(() => {
    const unsubscribe = context.subscribe(onEvent, filter);
    return unsubscribe;
  }, [onEvent, filter, context]);

  return {
    isConnected: context.isConnected,
    connectionError: context.connectionError,
  };
}
