import React, { useEffect, useRef, useState } from 'react';
import type { JobEvent } from '../hooks/useJobEvents';


interface JobEventTimelineProps {
  events: JobEvent[];
  live: boolean;
}

function getEventIcon(type: string): string {
  const icons: Record<string, string> = {
    job_submitted: '📥',
    resolve_started: '🔍',
    resolve_complete: '✅',
    resolve_failed: '❌',
    search_started: '🔎',
    search_complete: '📋',
    search_failed: '❌',
    auto_approved: '🤖',
    sent_to_review: '👁',
    user_approved: '👍',
    download_submitted: '⬇️',
    download_progress: '⏳',
    download_complete: '✔️',
    download_failed: '❌',
    move_started: '📦',
    move_complete: '📁',
    move_failed: '❌',
    scan_triggered: '🔬',
    scan_complete: '✅',
    scan_failed: '❌',
    job_complete: '🎉',
    job_cancelled: '🚫',
  };
  return icons[type] ?? '•';
}

function formatTimestamp(ts: string): string {
  try {
    return new Date(ts).toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch {
    return ts;
  }
}

const JobEventTimeline: React.FC<JobEventTimelineProps> = ({ events, live }) => {
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [userScrolledUp, setUserScrolledUp] = useState(false);

  // Detect manual scroll up
  const handleScroll = () => {
    const el = containerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    setUserScrolledUp(!atBottom);
  };

  // Auto-scroll to bottom when new events arrive, if live and user hasn't scrolled up
  useEffect(() => {
    if (live && !userScrolledUp) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [events, live, userScrolledUp]);

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="relative flex flex-col gap-0 overflow-y-auto max-h-full"
    >
      {events.length === 0 && (
        <p className="text-sm text-gray-500 italic py-4 text-center">No events yet.</p>
      )}
      <ol className="relative border-l border-gray-200 ml-3">
        {events.map((event, idx) => {
          const message = event.payload?.message as string | undefined;
          return (
            <li key={`${event.event_type}:${event.created_at}:${idx}`} className="mb-4 ml-4">
              <span className="absolute -left-3 flex h-6 w-6 items-center justify-center rounded-full bg-white border border-gray-200 text-sm">
                {getEventIcon(event.event_type)}
              </span>
              <div className="flex flex-col">
                <span className="text-xs text-gray-400 mb-0.5">
                  {formatTimestamp(event.created_at)}
                </span>
                <span className="text-sm font-medium text-gray-800 capitalize">
                  {event.event_type.replace(/_/g, ' ')}
                </span>
                {message && (
                  <span className="text-xs text-gray-500 mt-0.5">{message}</span>
                )}
              </div>
            </li>
          );
        })}
      </ol>
      {live && userScrolledUp && (
        <button
          onClick={() => {
            setUserScrolledUp(false);
            bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
          }}
          className="sticky bottom-2 self-center bg-blue-600 text-white text-xs px-3 py-1 rounded-full shadow hover:bg-blue-700 transition"
        >
          ↓ Jump to latest
        </button>
      )}
      <div ref={bottomRef} />
    </div>
  );
};

export default JobEventTimeline;
