import { useEffect, useRef, useState } from 'react';
import useStore from './useStore';

export interface JobEvent {
  job_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  created_at: string;
}

interface UseJobEventsResult {
  events: JobEvent[];
  connected: boolean;
}

const API_BASE = '';
const MAX_BACKOFF_MS = 30_000;

export function useJobEvents(jobId: string): UseJobEventsResult {
  const apiKey = useStore((s) => s.apiKey);
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [connected, setConnected] = useState(false);

  const backoffRef = useRef(1_000);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const esRef = useRef<EventSource | null>(null);
  // Capture current values in refs so the cleanup/reconnect callbacks stay stable
  const jobIdRef = useRef(jobId);
  const apiKeyRef = useRef(apiKey);

  useEffect(() => {
    jobIdRef.current = jobId;
  }, [jobId]);

  useEffect(() => {
    apiKeyRef.current = apiKey;
  }, [apiKey]);

  useEffect(() => {
    let cancelled = false;

    function connect() {
      if (cancelled) return;

      const url = `${API_BASE}/api/v1/jobs/${jobIdRef.current}/events?api_key=${encodeURIComponent(apiKeyRef.current)}`;
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
          // Backfill: prepend past events, then append new ones as they arrive
          const backfill = parsed as JobEvent[];
          setEvents((prev) => {
            const existingKeys = new Set(prev.map((e) => `${e.event_type}:${e.created_at}`));
            const novel = backfill.filter((e) => !existingKeys.has(`${e.event_type}:${e.created_at}`));
            return [...novel, ...prev];
          });
        } else {
          const event = parsed as JobEvent;
          setEvents((prev) => {
            const key = `${event.event_type}:${event.created_at}`;
            if (prev.some((e) => `${e.event_type}:${e.created_at}` === key)) return prev;
            return [...prev, event];
          });
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
    // Re-connect when jobId or apiKey changes
  }, [jobId, apiKey]);

  return { events, connected };
}
