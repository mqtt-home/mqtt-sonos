import { useEffect, useRef, useState, useCallback } from 'react';
import type { SonosState } from '@/types/sonos';
import { API_BASE } from '@/lib/api';

interface SSEHookReturn {
  states: Record<string, SonosState>;
  isConnected: boolean;
  error: string | null;
  reconnect: () => void;
}

export function useSSE(): SSEHookReturn {
  const [states, setStates] = useState<Record<string, SonosState>>({});
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const cleanup = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
  }, []);

  const connect = useCallback(() => {
    cleanup();

    try {
      const eventSource = new EventSource(`${API_BASE}/events`);
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        setIsConnected(true);
        setError(null);
      };

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.type === 'state' && data.device && data.state) {
            setStates(prev => ({ ...prev, [data.device]: data.state as SonosState }));
          }
        } catch {
          setError('Failed to parse server data');
        }
      };

      eventSource.onerror = () => {
        setIsConnected(false);
        setError(eventSource.readyState === EventSource.CLOSED
          ? 'Connection closed by server'
          : 'Connection error');

        reconnectTimeoutRef.current = setTimeout(() => {
          if (eventSourceRef.current === eventSource) {
            connect();
          }
        }, 3000);
      };
    } catch {
      setError('Failed to connect to server');
    }
  }, [cleanup]);

  useEffect(() => {
    connect();
    return cleanup;
  }, [connect, cleanup]);

  const reconnect = useCallback(() => {
    setError(null);
    connect();
  }, [connect]);

  return { states, isConnected, error, reconnect };
}
