import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { useGlobalEvents } from '../hooks/useGlobalEvents';
import type { JobEvent } from '../hooks/useJobEvents';
import { systemApi, jobsApi, reviewApi, batchesApi } from '../api/client';
import type { SystemStatus, WorkerPoolStatus } from '../api/client';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatRelativeTime(ts: string): string {
  try {
    const diff = Date.now() - new Date(ts).getTime();
    const secs = Math.floor(diff / 1000);
    if (secs < 60) return `${secs}s ago`;
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
  } catch {
    return '';
  }
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

// ---------------------------------------------------------------------------
// Event Feed
// ---------------------------------------------------------------------------

interface EventFeedProps {
  events: JobEvent[];
  connected: boolean;
}

const EventFeed: React.FC<EventFeedProps> = ({ events, connected }) => {
  const navigate = useNavigate();
  const containerRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const [userScrolledUp, setUserScrolledUp] = useState(false);

  const handleScroll = () => {
    const el = containerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    setUserScrolledUp(!atBottom);
  };

  useEffect(() => {
    if (!userScrolledUp) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [events, userScrolledUp]);

  const handleRowClick = (event: JobEvent) => {
    if (event.job_id) {
      navigate(`/queue/${event.job_id}`);
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-gray-700 uppercase tracking-wide">
          Live Activity
        </h2>
        <span
          className={`inline-flex items-center gap-1.5 text-xs ${connected ? 'text-green-600' : 'text-red-500'}`}
        >
          <span
            className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500 animate-pulse' : 'bg-red-400'}`}
          />
          {connected ? 'Live' : 'Reconnecting…'}
        </span>
      </div>

      {/* Scrollable list */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto min-h-0"
      >
        {events.length === 0 && (
          <p className="text-sm text-gray-400 italic text-center py-8">
            Waiting for events…
          </p>
        )}
        <ul className="divide-y divide-gray-100">
          {[...events].reverse().map((event, idx) => (
            <li
              key={`${event.id ?? ''}-${idx}`}
              onClick={() => handleRowClick(event)}
              className={`flex items-start gap-3 px-2 py-2.5 text-sm hover:bg-gray-50 transition-colors ${
                event.job_id ? 'cursor-pointer' : ''
              }`}
            >
              <span className="text-base leading-none mt-0.5 shrink-0">
                {getEventIcon(event.type)}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex items-baseline justify-between gap-2">
                  <span className="font-medium text-gray-800 truncate">
                    {event.message || event.type.replace(/_/g, ' ')}
                  </span>
                  <span className="text-xs text-gray-400 shrink-0 whitespace-nowrap">
                    {formatRelativeTime(event.created_at)}
                  </span>
                </div>
                {event.message && (
                  <span className="text-xs text-gray-500 capitalize">
                    {event.type.replace(/_/g, ' ')}
                  </span>
                )}
              </div>
            </li>
          ))}
        </ul>
        <div ref={bottomRef} />
      </div>

      {/* Jump to latest button */}
      {userScrolledUp && (
        <button
          onClick={() => {
            setUserScrolledUp(false);
            bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
          }}
          className="mt-2 self-center bg-blue-600 text-white text-xs px-3 py-1 rounded-full shadow hover:bg-blue-700 transition"
        >
          ↓ Jump to latest
        </button>
      )}
    </div>
  );
};

// ---------------------------------------------------------------------------
// Worker Status Panel
// ---------------------------------------------------------------------------

function isWorkerPool(w: WorkerPoolStatus | { running: boolean; last_poll: string | null }): w is WorkerPoolStatus {
  return 'pool_size' in w;
}

interface WorkerStatusPanelProps {
  status: SystemStatus | undefined;
  isLoading: boolean;
  isError: boolean;
}

const WorkerStatusPanel: React.FC<WorkerStatusPanelProps> = ({ status, isLoading, isError }) => {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {[1, 2, 3, 4, 5, 6].map((i) => (
          <div key={i} className="h-8 bg-gray-100 rounded animate-pulse" />
        ))}
      </div>
    );
  }

  if (isError || !status) {
    return (
      <p className="text-sm text-red-500">Failed to load worker status.</p>
    );
  }

  const workers = status.workers;

  type WorkerEntry = {
    name: string;
    worker: WorkerPoolStatus | { running: boolean; last_poll: string | null };
  };

  const entries: WorkerEntry[] = [
    { name: 'Resolver', worker: workers.resolver },
    { name: 'Search', worker: workers.search },
    { name: 'Download', worker: workers.download },
    { name: 'Monitor', worker: workers.monitor },
    { name: 'Mover', worker: workers.mover },
    { name: 'Scanner', worker: workers.scanner },
  ];

  return (
    <ul className="divide-y divide-gray-100">
      {entries.map(({ name, worker }) => {
        const running = worker.running;
        return (
          <li key={name} className="flex items-center justify-between py-2 text-sm">
            <div className="flex items-center gap-2">
              <span
                className={`w-2 h-2 rounded-full shrink-0 ${running ? 'bg-green-500' : 'bg-red-400'}`}
              />
              <span className="text-gray-700">{name}</span>
            </div>
            <div className="text-gray-500 text-xs">
              {isWorkerPool(worker) ? (
                <span>
                  {worker.active}/{worker.pool_size} active
                </span>
              ) : (
                <span>{running ? 'Running' : 'Stopped'}</span>
              )}
            </div>
          </li>
        );
      })}
    </ul>
  );
};

// ---------------------------------------------------------------------------
// Quick Stats Panel
// ---------------------------------------------------------------------------

const QuickStatsPanel: React.FC = () => {
  // Fetch counts for jobs today: submitted, complete, failed
  // We approximate by listing with status filters (no server-side date filter, so we count all)
  const { data: reviewData } = useQuery({
    queryKey: ['review-list-count'],
    queryFn: () => reviewApi.list({ limit: 1 }),
    refetchInterval: 30_000,
  });

  const { data: batchData } = useQuery({
    queryKey: ['batches-count'],
    queryFn: () => batchesApi.list(),
    refetchInterval: 30_000,
  });

  const { data: completedData } = useQuery({
    queryKey: ['jobs-complete-count'],
    queryFn: () => jobsApi.list({ status: 'complete', limit: 1 }),
    refetchInterval: 30_000,
  });

  const { data: failedData } = useQuery({
    queryKey: ['jobs-failed-count'],
    queryFn: () => jobsApi.list({ status: 'resolve_failed,search_failed,download_failed,move_failed,scan_failed', limit: 1 }),
    refetchInterval: 30_000,
  });

  const { data: submittedData } = useQuery({
    queryKey: ['jobs-submitted-count'],
    queryFn: () => jobsApi.list({ limit: 1 }),
    refetchInterval: 30_000,
  });

  const reviewCount = reviewData?.total ?? null;
  const pendingBatches = batchData?.batches.filter((b) => b.pending_count > 0 && !b.confirmed).length ?? null;
  const completedCount = completedData?.total ?? null;
  const failedCount = failedData?.total ?? null;
  const totalJobs = submittedData?.total ?? null;

  const statItem = (label: string, value: number | null, href?: string) => {
    const inner = (
      <div className={`flex items-center justify-between py-2 text-sm ${href ? 'cursor-pointer hover:text-blue-600 transition-colors' : ''}`}>
        <span className="text-gray-600">{label}</span>
        <span className={`font-semibold ${value === null ? 'text-gray-300' : 'text-gray-900'}`}>
          {value === null ? '—' : value.toLocaleString()}
        </span>
      </div>
    );
    if (href) {
      return (
        <Link to={href} key={label} className="block border-b border-gray-100 last:border-0">
          {inner}
        </Link>
      );
    }
    return (
      <div key={label} className="border-b border-gray-100 last:border-0">
        {inner}
      </div>
    );
  };

  return (
    <div>
      {statItem('Total jobs', totalJobs)}
      {statItem('Completed', completedCount)}
      {statItem('Failed', failedCount)}
      {statItem('Review queue', reviewCount, '/review')}
      {statItem('Pending batch confirmations', pendingBatches, '/batches')}
    </div>
  );
};

// ---------------------------------------------------------------------------
// Dashboard page
// ---------------------------------------------------------------------------

export default function Dashboard() {
  const { events, connected } = useGlobalEvents();

  const {
    data: statusData,
    isLoading: statusLoading,
    isError: statusError,
  } = useQuery({
    queryKey: ['system-status'],
    queryFn: () => systemApi.status(),
    refetchInterval: 30_000,
  });

  return (
    <div className="h-full flex flex-col">
      <h1 className="text-xl font-bold text-gray-900 mb-4">Dashboard</h1>

      <div className="flex-1 grid grid-cols-1 lg:grid-cols-2 gap-6 min-h-0">
        {/* Left column — event feed */}
        <div className="bg-white border border-gray-200 rounded-xl p-4 flex flex-col min-h-0 overflow-hidden">
          <EventFeed events={events} connected={connected} />
        </div>

        {/* Right column */}
        <div className="flex flex-col gap-4 min-h-0">
          {/* Worker status */}
          <div className="bg-white border border-gray-200 rounded-xl p-4">
            <h2 className="text-sm font-semibold text-gray-700 uppercase tracking-wide mb-3">
              Workers
            </h2>
            <WorkerStatusPanel
              status={statusData}
              isLoading={statusLoading}
              isError={statusError}
            />
          </div>

          {/* Quick stats */}
          <div className="bg-white border border-gray-200 rounded-xl p-4">
            <h2 className="text-sm font-semibold text-gray-700 uppercase tracking-wide mb-3">
              Quick Stats
            </h2>
            <QuickStatsPanel />
          </div>
        </div>
      </div>
    </div>
  );
}
