import { useEffect, useRef, useCallback } from 'react';

type SSEHandler = (event: string, data: unknown) => void;

export function useSSE(handler: SSEHandler, onConnect: (() => void) | undefined, stack: string) {
  const handlerRef = useRef(handler);
  handlerRef.current = handler;
  const onConnectRef = useRef(onConnect);
  onConnectRef.current = onConnect;

  const connect = useCallback(() => {
    const es = new EventSource(`/api/stacks/${stack}/events`);

    es.onopen = () => {
      onConnectRef.current?.();
    };

    const onEvent = (eventName: string) => (e: MessageEvent) => {
      try {
        const data: unknown = JSON.parse(e.data as string);
        handlerRef.current(eventName, data);
      } catch {
        // ignore parse errors
      }
    };

    es.addEventListener('services', onEvent('services'));
    es.addEventListener('infrastructure', onEvent('infrastructure'));
    es.addEventListener('health', onEvent('health'));
    es.addEventListener('log', onEvent('log'));
    es.addEventListener('stack_progress', onEvent('stack_progress'));
    es.addEventListener('stack_starting', onEvent('stack_starting'));
    es.addEventListener('stack_started', onEvent('stack_started'));
    es.addEventListener('stack_stopped', onEvent('stack_stopped'));
    es.addEventListener('stack_error', onEvent('stack_error'));
    es.addEventListener('stack_stopping', onEvent('stack_stopping'));
    es.addEventListener('diagnose_started', onEvent('diagnose_started'));
    es.addEventListener('diagnose_stream', onEvent('diagnose_stream'));
    es.addEventListener('diagnose_result', onEvent('diagnose_result'));
    es.addEventListener('diagnose_error', onEvent('diagnose_error'));
    es.addEventListener('diagnose_interrupted', onEvent('diagnose_interrupted'));
    es.addEventListener('diagnose_session_closed', onEvent('diagnose_session_closed'));

    es.onerror = () => {
      es.close();
      // Reconnect after 3s
      setTimeout(connect, 3000);
    };

    return es;
  }, [stack]);

  useEffect(() => {
    const es = connect();

    return () => es.close();
  }, [connect]);
}
