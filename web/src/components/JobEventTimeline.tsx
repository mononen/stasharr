import React, { useEffect, useRef, useState } from 'react';
import type { JobEvent } from '../hooks/useJobEvents';

interface JobEventTimelineProps {
  events: JobEvent[];
  live: boolean;
}

// Major events are pipeline milestones — visually prominent
const MAJOR_EVENTS = new Set([
  'job_submitted',
  'resolve_complete', 'resolve_failed',
  'search_complete', 'search_failed',
  'auto_approved', 'sent_to_review', 'user_approved',
  'download_submitted', 'download_complete', 'download_failed',
  'move_started', 'move_complete', 'move_failed',
  'scan_triggered', 'scan_complete', 'scan_failed',
  'job_complete', 'job_cancelled',
]);

const SUCCESS_EVENTS = new Set([
  'resolve_complete', 'search_complete', 'auto_approved', 'user_approved',
  'download_complete', 'move_complete', 'scan_complete', 'job_complete',
  'scrape_complete', 'stash_id_attached', 'phash_queued', 'sabnzbd_cleaned_up',
  'nzb_fetched',
]);

const FAILED_EVENTS = new Set([
  'resolve_failed', 'search_failed', 'download_failed', 'move_failed',
  'scan_failed', 'scrape_failed',
]);

const PENDING_EVENTS = new Set([
  'sent_to_review', 'download_submitted', 'scan_triggered', 'scrape_started',
  'nzb_fetching', 'nzb_submitting',
]);

function getEventColor(type: string): 'green' | 'red' | 'amber' | 'blue' | 'gray' {
  if (FAILED_EVENTS.has(type)) return 'red';
  if (SUCCESS_EVENTS.has(type)) return 'green';
  if (PENDING_EVENTS.has(type)) return 'blue';
  if (type === 'sent_to_review') return 'amber';
  if (type === 'job_cancelled') return 'gray';
  return 'gray';
}

function getEventIcon(type: string): string {
  const icons: Record<string, string> = {
    // resolver
    job_submitted: '📥',
    resolve_started: '🔍',
    stash_check_started: '🏠',
    already_stashed: '📌',
    stashdb_fetch_started: '🌐',
    resolve_complete: '✅',
    resolve_failed: '❌',
    // search
    search_started: '🔎',
    fallback_search: '↩️',
    results_found: '📋',
    scoring_complete: '🏆',
    search_complete: '📋',
    search_failed: '❌',
    auto_approved: '🤖',
    sent_to_review: '👁',
    user_approved: '👍',
    // download
    nzb_fetching: '⬇️',
    nzb_fetched: '📄',
    nzb_submitting: '📤',
    download_submitted: '⬇️',
    download_progress: '⏳',
    download_queued: '🕐',
    download_verifying: '🔐',
    download_repairing: '🔧',
    download_unpacking: '📦',
    download_complete: '✔️',
    download_failed: '❌',
    // move
    move_started: '🚚',
    video_file_found: '🎬',
    cross_fs_copy: '📋',
    move_complete: '📁',
    move_failed: '❌',
    // scan/import
    scan_triggered: '🔬',
    stash_id_attached: '🔗',
    scrape_started: '🌐',
    scrape_complete: '📝',
    scrape_failed: '❌',
    phash_queued: '#️⃣',
    sabnzbd_cleaned_up: '🗑️',
    scan_complete: '✅',
    scan_failed: '❌',
    job_complete: '🎉',
    job_cancelled: '🚫',
  };
  return icons[type] ?? '·';
}

function getPayloadLabel(type: string, payload: Record<string, unknown>): string | null {
  if (!payload) return null;
  // prefer explicit message field
  if (typeof payload.message === 'string') return payload.message;
  if (typeof payload.error === 'string') return payload.error;
  // event-specific useful fields
  switch (type) {
    case 'resolve_complete':
      return typeof payload.title === 'string' ? payload.title : null;
    case 'search_started':
      return typeof payload.query === 'string' ? `"${payload.query}"` : null;
    case 'fallback_search':
      return typeof payload.query === 'string' ? `"${payload.query}"` : null;
    case 'results_found':
      return typeof payload.count === 'number' ? `${payload.count} results` : null;
    case 'scoring_complete':
      return typeof payload.top_score === 'number' ? `top score ${payload.top_score}` : null;
    case 'search_complete':
      return typeof payload.count === 'number' ? `${payload.count} results` : null;
    case 'auto_approved':
      return typeof payload.title === 'string' ? payload.title : null;
    case 'nzb_fetched':
      return typeof payload.size_bytes === 'number' ? `${(payload.size_bytes / 1024).toFixed(1)} KB` : null;
    case 'nzb_submitting':
      return typeof payload.method === 'string' ? payload.method.replace(/_/g, ' ') : null;
    case 'download_submitted':
      return typeof payload.release_title === 'string' ? payload.release_title : null;
    case 'video_file_found':
      return typeof payload.filename === 'string' ? payload.filename : null;
    case 'download_complete':
      return typeof payload.path === 'string' ? payload.path : null;
    case 'move_complete':
    case 'job_complete':
      return typeof payload.final_path === 'string' ? payload.final_path : null;
    case 'scan_triggered':
      return typeof payload.stash_instance === 'string' ? payload.stash_instance : null;
    case 'stash_id_attached':
      return typeof payload.stash_scene_id === 'string' ? `scene #${payload.stash_scene_id}` : null;
    default:
      return null;
  }
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

function formatLabel(type: string): string {
  return type.replace(/_/g, ' ');
}

const colorClasses = {
  green: {
    dot: 'bg-green-500',
    border: 'border-green-200 dark:border-green-800',
    bg: 'bg-green-50 dark:bg-green-900/20',
    text: 'text-green-800 dark:text-green-200',
    icon: 'bg-green-100 dark:bg-green-900/40 border-green-200 dark:border-green-700',
  },
  red: {
    dot: 'bg-red-500',
    border: 'border-red-200 dark:border-red-800',
    bg: 'bg-red-50 dark:bg-red-900/20',
    text: 'text-red-800 dark:text-red-200',
    icon: 'bg-red-100 dark:bg-red-900/40 border-red-200 dark:border-red-700',
  },
  blue: {
    dot: 'bg-blue-500',
    border: 'border-blue-200 dark:border-blue-800',
    bg: 'bg-blue-50 dark:bg-blue-900/20',
    text: 'text-blue-800 dark:text-blue-200',
    icon: 'bg-blue-100 dark:bg-blue-900/40 border-blue-200 dark:border-blue-700',
  },
  amber: {
    dot: 'bg-amber-500',
    border: 'border-amber-200 dark:border-amber-800',
    bg: 'bg-amber-50 dark:bg-amber-900/20',
    text: 'text-amber-800 dark:text-amber-200',
    icon: 'bg-amber-100 dark:bg-amber-900/40 border-amber-200 dark:border-amber-700',
  },
  gray: {
    dot: 'bg-gray-400',
    border: 'border-gray-200 dark:border-gray-700',
    bg: 'bg-gray-50 dark:bg-gray-800/40',
    text: 'text-gray-700 dark:text-gray-300',
    icon: 'bg-white dark:bg-gray-900 border-gray-200 dark:border-gray-700',
  },
} as const;

const JobEventTimeline: React.FC<JobEventTimelineProps> = ({ events, live }) => {
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [userScrolledUp, setUserScrolledUp] = useState(false);

  const handleScroll = () => {
    const el = containerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    setUserScrolledUp(!atBottom);
  };

  useEffect(() => {
    if (live && !userScrolledUp) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [events, live, userScrolledUp]);

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="relative flex flex-col overflow-y-auto max-h-full"
    >
      {events.length === 0 && (
        <p className="text-sm text-gray-500 dark:text-gray-400 italic py-4 text-center">No events yet.</p>
      )}

      <ol className="relative border-l-2 border-gray-200 dark:border-gray-700 ml-3 flex flex-col gap-0">
        {events.map((event, idx) => {
          const isMajor = MAJOR_EVENTS.has(event.event_type);
          const color = getEventColor(event.event_type);
          const c = colorClasses[color];
          const label = getPayloadLabel(event.event_type, event.payload);

          if (isMajor) {
            return (
              <li key={`${event.event_type}:${event.created_at}:${idx}`} className="ml-4 my-1.5">
                {/* colored connector dot */}
                <span className={`absolute -left-[5px] w-2.5 h-2.5 rounded-full mt-3 ${c.dot}`} />
                <div className={`rounded-md border px-3 py-2 ${c.border} ${c.bg}`}>
                  <div className="flex items-center gap-2">
                    <span className="text-base leading-none">{getEventIcon(event.event_type)}</span>
                    <span className={`text-sm font-semibold capitalize flex-1 ${c.text}`}>
                      {formatLabel(event.event_type)}
                    </span>
                    <span className="text-xs text-gray-400 dark:text-gray-500 shrink-0">
                      {formatTimestamp(event.created_at)}
                    </span>
                  </div>
                  {label && (
                    <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 truncate pl-6">{label}</p>
                  )}
                </div>
              </li>
            );
          }

          // Minor event — compact, no card
          return (
            <li key={`${event.event_type}:${event.created_at}:${idx}`} className="ml-4 my-0.5">
              <span className="absolute -left-[3px] w-1.5 h-1.5 rounded-full bg-gray-300 dark:bg-gray-600 mt-1.5" />
              <div className="flex items-baseline gap-1.5 pl-1">
                <span className="text-xs leading-none">{getEventIcon(event.event_type)}</span>
                <span className="text-xs text-gray-500 dark:text-gray-400 capitalize">
                  {formatLabel(event.event_type)}
                </span>
                {label && (
                  <span className="text-xs text-gray-400 dark:text-gray-500 truncate">— {label}</span>
                )}
                <span className="text-xs text-gray-300 dark:text-gray-600 shrink-0 ml-auto">
                  {formatTimestamp(event.created_at)}
                </span>
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
