import { useEffect, useRef, useState } from 'react';
import useStore from './useStore';
import type { JobEvent } from './useJobEvents';

interface UseGlobalEventsResult {
  events: JobEvent[];
  connected: boolean;
}

const API_BASE = '';
const MAX_BACKOFF_MS = 30_000;

export function useGlobalEvents(): UseGlobalEventsResult {
  const apiKey = useStore((s) => s.apiKey);
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [connected, setConnected] = useState(false);

  const backoffRef = useRef(1_000);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const apiKeyRef = useRef(apiKey);

  useEffect(() => {
    apiKeyRef.current = apiKey;
  }, [apiKey]);

  useEffect(() => {
    let cancelled = false;

    function connect() {
      if (cancelled) return;

      const url = `${API_BASE}/api/v1/events?api_key=${encodeURIComponent(apiKeyRef.current)}`;
      const es = new EventSource(url);
      esRef.current = es;

      es.onopen = () => {
        if (cancelled) return;
        backoffRef.current = 1_000;
        setConnected(true);
      };

      const handleData = (raw: string) => {
        if (cancelled) return;
        let parsed: unknown;
        try {
          parsed = JSON.parse(raw);
        } catch {
          return;
        }
        if (Array.isArray(parsed)) {
          const backfill = parsed as JobEvent[];
          setEvents((prev) => {
            const existingKeys = new Set(prev.map((e) => `${e.event_type}:${e.created_at}`));
            const novel = backfill.filter((e) => !existingKeys.has(`${e.event_type}:${e.created_at}`));
            return [...novel, ...prev];
          });
        } else {
          const event = parsed as JobEvent;
          setEvents((prev) => [...prev, event]);
        }
      };

      // Named event emitted by the server
      es.addEventListener('job_event', (e: MessageEvent) => {
        handleData(e.data as string);
      });

      // Default message event (fallback / initial backfill)
      es.onmessage = (e: MessageEvent) => {
        handleData(e.data as string);
      };

      es.onerror = () => {
        es.close();
        esRef.current = null;
        if (cancelled) return;
        setConnected(false);
        const delay = backoffRef.current;
        backoffRef.current = Math.min(delay * 2, MAX_BACKOFF_MS);
        retryTimerRef.current = setTimeout(connect, delay);
      };
    }

    connect();

    return () => {
      cancelled = true;
      if (retryTimerRef.current !== null) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
      if (esRef.current !== null) {
        esRef.current.close();
        esRef.current = null;
      }
      setConnected(false);
    };
  }, [apiKey]);

  return { events, connected };
}
