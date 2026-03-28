import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { useGlobalEvents } from '../hooks/useGlobalEvents';
import type { JobEvent } from '../hooks/useJobEvents';
import { systemApi, jobsApi } from '../api/client';
import type { SystemStatus, JobStatus } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import useStore from '../hooks/useStore';

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
// Pipeline Stats (same stages as Queue page)
// ---------------------------------------------------------------------------

const PIPELINE_STAGES: { label: string; statuses: JobStatus[] }[] = [
  { label: 'Resolving', statuses: ['submitted', 'resolving'] },
  { label: 'Searching', statuses: ['resolved', 'searching'] },
  { label: 'Review', statuses: ['awaiting_review'] },
  { label: 'Downloading', statuses: ['approved', 'downloading'] },
  { label: 'Moving', statuses: ['download_complete', 'moving'] },
  { label: 'Scanning', statuses: ['moved', 'scanning'] },
  { label: 'Complete', statuses: ['complete'] },
  { label: 'Failed', statuses: ['resolve_failed', 'search_failed', 'download_failed', 'move_failed', 'scan_failed'] },
  { label: 'Cancelled', statuses: ['cancelled'] },
];

const PipelineStats: React.FC<{ counts: Record<string, number> }> = ({ counts }) => {
  const stages = PIPELINE_STAGES.map((s) => ({
    ...s,
    count: s.statuses.reduce((sum, st) => sum + (counts[st] ?? 0), 0),
  })).filter((s) => s.count > 0);

  const activeStages = stages.filter(
    (s) => s.label !== 'Complete' && s.label !== 'Cancelled' && s.label !== 'Failed',
  );
  const maxCount = Math.max(0, ...activeStages.map((s) => s.count));
  const bottleneckLabel = maxCount > 0
    ? activeStages.find((s) => s.count === maxCount)?.label
    : null;

  return (
    <div className="flex flex-wrap items-center gap-2">
      {stages.map((s) => {
        const isBottleneck = s.label === bottleneckLabel;
        return (
          <span
            key={s.label}
            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
              isBottleneck
                ? 'bg-amber-100 dark:bg-amber-900/30 text-amber-800 dark:text-amber-300 ring-1 ring-amber-300 dark:ring-amber-700'
                : s.label === 'Failed'
                ? 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400'
                : s.label === 'Complete'
                ? 'bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-400'
                : s.label === 'Cancelled'
                ? 'bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400'
                : 'bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300'
            }`}
            title={isBottleneck ? `${s.label} is the current bottleneck` : s.label}
          >
            {s.label}
            <span className="font-semibold">{s.count}</span>
          </span>
        );
      })}
    </div>
  );
};

// ---------------------------------------------------------------------------
// In-Flight Panel
// ---------------------------------------------------------------------------

const IN_FLIGHT_STATUSES =
  'submitted,resolving,resolved,searching,awaiting_review,approved,downloading,download_complete,moving,moved,scanning';

const InFlightPanel: React.FC<{ counts: Record<string, number> | undefined }> = ({ counts }) => {
  const navigate = useNavigate();
  const safeMode = useStore((s) => s.safeMode);

  const { data, isLoading } = useQuery({
    queryKey: ['jobs-in-flight'],
    queryFn: () => jobsApi.list({ status: IN_FLIGHT_STATUSES, limit: 50 }),
    refetchInterval: 15_000,
  });

  const jobs = data?.jobs ?? [];
  const total = data?.total ?? 0;

  const needsSearching = counts
    ? (counts['resolved'] ?? 0) + (counts['searching'] ?? 0)
    : null;
  const needsReview = counts ? (counts['awaiting_review'] ?? 0) : null;

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wide">
          In Flight
        </h2>
        <span className="text-xs text-gray-500 dark:text-gray-400">
          {total > 0 ? `${total} active` : 'none active'}
        </span>
      </div>

      {/* Action callouts */}
      {(needsSearching !== null || needsReview !== null) && (
        <div className="flex flex-wrap gap-2 mb-3">
          {needsSearching !== null && needsSearching > 0 && (
            <Link
              to="/queue?status=searching"
              className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-400 hover:bg-blue-100 dark:hover:bg-blue-900/30 transition-colors"
            >
              🔎 {needsSearching} need searching
            </Link>
          )}
          {needsReview !== null && needsReview > 0 && (
            <Link
              to="/review"
              className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-400 hover:bg-amber-100 dark:hover:bg-amber-900/30 transition-colors"
            >
              👁 {needsReview} need review
            </Link>
          )}
        </div>
      )}

      {/* Scrollable job list */}
      <div className="flex-1 overflow-y-auto min-h-0">
        {isLoading && (
          <div className="space-y-2">
            {[1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="h-12 bg-gray-100 dark:bg-gray-800 rounded animate-pulse" />
            ))}
          </div>
        )}
        {!isLoading && jobs.length === 0 && (
          <p className="text-sm text-gray-400 dark:text-gray-500 italic text-center py-8">
            No active jobs.
          </p>
        )}
        <ul className="divide-y divide-gray-100 dark:divide-gray-800">
          {jobs.map((job) => (
            <li
              key={job.id}
              onClick={() => navigate(`/queue/${job.id}`)}
              className="flex items-center gap-3 px-2 py-2 hover:bg-gray-50 dark:hover:bg-gray-800/50 cursor-pointer transition-colors"
            >
              {!safeMode && job.scene?.image_url ? (
                <img
                  src={job.scene.image_url}
                  alt=""
                  className="w-12 h-8 rounded object-cover bg-gray-200 dark:bg-gray-700 shrink-0"
                  loading="lazy"
                />
              ) : (
                <span className="flex items-center justify-center w-12 h-8 rounded bg-gray-100 dark:bg-gray-800 text-sm shrink-0">
                  🎬
                </span>
              )}
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium text-gray-800 dark:text-gray-200 truncate">
                  {job.scene?.title ?? job.stashdb_url}
                </div>
                {job.scene?.studio_name && (
                  <div className="text-xs text-gray-500 dark:text-gray-400 truncate">
                    {job.scene.studio_name}
                  </div>
                )}
              </div>
              <StatusBadge status={job.status} />
            </li>
          ))}
        </ul>
        {total > jobs.length && (
          <p className="text-center text-xs text-gray-400 dark:text-gray-500 py-2">
            + {total - jobs.length} more — <Link to="/queue" className="text-blue-500 hover:underline">view all</Link>
          </p>
        )}
      </div>
    </div>
  );
};

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
    if (event.job_id) navigate(`/queue/${event.job_id}`);
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wide">
          Live Activity
        </h2>
        <span
          className={`inline-flex items-center gap-1.5 text-xs ${connected ? 'text-green-600 dark:text-green-400' : 'text-red-500 dark:text-red-400'}`}
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
          <p className="text-sm text-gray-400 dark:text-gray-500 italic text-center py-8">
            Waiting for events…
          </p>
        )}
        <ul className="divide-y divide-gray-100 dark:divide-gray-800">
          {[...events].reverse().map((event, idx) => (
            <li
              key={`${event.event_type}:${event.created_at}:${idx}`}
              onClick={() => handleRowClick(event)}
              className={`flex items-start gap-3 px-2 py-2.5 text-sm hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors ${
                event.job_id ? 'cursor-pointer' : ''
              }`}
            >
              <span className="text-base leading-none mt-0.5 shrink-0">
                {getEventIcon(event.event_type)}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex items-baseline justify-between gap-2">
                  <span className="font-medium text-gray-800 dark:text-gray-200 truncate">
                    {String(event.payload?.message || event.event_type.replace(/_/g, ' '))}
                  </span>
                  <span className="text-xs text-gray-400 dark:text-gray-500 shrink-0 whitespace-nowrap">
                    {formatRelativeTime(event.created_at)}
                  </span>
                </div>
                {!!event.payload?.message && (
                  <span className="text-xs text-gray-500 dark:text-gray-400 capitalize">
                    {event.event_type.replace(/_/g, ' ')}
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
          <div key={i} className="h-8 bg-gray-100 dark:bg-gray-800 rounded animate-pulse" />
        ))}
      </div>
    );
  }

  if (isError || !status) {
    return <p className="text-sm text-red-500">Failed to load worker status.</p>;
  }

  return (
    <ul className="divide-y divide-gray-100 dark:divide-gray-800">
      {Object.entries(status.workers).map(([key, worker]) => (
        <li key={key} className="flex items-center justify-between py-2 text-sm">
          <div className="flex items-center gap-2">
            <span className={`w-2 h-2 rounded-full shrink-0 ${worker.running ? 'bg-green-500' : 'bg-red-400'}`} />
            <span className="text-gray-700 dark:text-gray-300 capitalize">{key}</span>
          </div>
          <span className="text-gray-500 dark:text-gray-400 text-xs">
            {worker.pool_size > 0 ? `pool: ${worker.pool_size}` : (worker.running ? 'Running' : 'Stopped')}
          </span>
        </li>
      ))}
    </ul>
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

  const { data: statsData } = useQuery({
    queryKey: ['job-stats'],
    queryFn: () => jobsApi.stats(),
    refetchInterval: 15_000,
  });

  return (
    <div className="h-full flex flex-col">
      <div className="flex flex-wrap items-center gap-4 mb-4">
        <h1 className="text-xl font-bold text-gray-900 dark:text-gray-100">Dashboard</h1>
        {statsData && <PipelineStats counts={statsData.counts} />}
      </div>

      <div className="flex-1 grid grid-cols-1 lg:grid-cols-2 gap-6 min-h-0">
        {/* Left column — in-flight scenes */}
        <div className="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-xl p-4 flex flex-col min-h-0 overflow-hidden">
          <InFlightPanel counts={statsData?.counts} />
        </div>

        {/* Right column — workers + live activity */}
        <div className="flex flex-col gap-4 min-h-0">
          {/* Worker status */}
          <div className="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-xl p-4">
            <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wide mb-3">
              Workers
            </h2>
            <WorkerStatusPanel
              status={statusData}
              isLoading={statusLoading}
              isError={statusError}
            />
          </div>

          {/* Live activity */}
          <div className="flex-1 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-xl p-4 flex flex-col min-h-0 overflow-hidden">
            <EventFeed events={events} connected={connected} />
          </div>
        </div>
      </div>
    </div>
  );
}
