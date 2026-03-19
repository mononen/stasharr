import { useMemo, useState, useRef, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { jobsApi } from '../api/client';
import type { SearchResult as ApiSearchResult } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import JobEventTimeline from '../components/JobEventTimeline';
import SearchResultRow from '../components/SearchResultRow';
import type { SearchResult as RowSearchResult } from '../components/SearchResultRow';
import CustomSearchPanel from '../components/CustomSearchPanel';
import { useJobEvents } from '../hooks/useJobEvents';

const RETRYABLE_STATUSES = new Set([
  // Failed states
  'resolve_failed',
  'search_failed',
  'download_failed',
  'move_failed',
  'scan_failed',
  // Stuck in-progress states (force reset to prior state)
  'resolving',
  'searching',
  'search_complete', // legacy status from old recoverStuckJobs
  'downloading',
  'moving',
  'scanning',
]);

const ADVANCEABLE_STATUSES = new Set([
  'search_complete', // legacy status from old recoverStuckJobs
  'downloading',
  'moving',
  'scanning',
]);

const IN_PROGRESS_STATUSES = new Set([
  'resolving',
  'searching',
  'search_complete',
  'downloading',
  'moving',
  'scanning',
]);

function RetryButton({ jobId, isInProgress, onRetried }: { jobId: string; isInProgress: boolean; onRetried: () => void }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handler(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  async function retry(fromStart: boolean) {
    setBusy(true);
    setOpen(false);
    try {
      if (fromStart) {
        await jobsApi.retryFromStart(jobId);
      } else {
        await jobsApi.retry(jobId);
      }
      onRetried();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div ref={ref} className="relative flex">
      <button
        onClick={() => retry(false)}
        disabled={busy}
        className="px-3 py-1.5 text-sm font-medium bg-amber-500 hover:bg-amber-600 disabled:opacity-50 text-white rounded-l"
      >
        {busy ? 'Retrying…' : isInProgress ? 'Force Retry' : 'Retry'}
      </button>
      <button
        onClick={() => setOpen((o) => !o)}
        disabled={busy}
        className="px-1.5 py-1.5 text-sm font-medium bg-amber-500 hover:bg-amber-600 disabled:opacity-50 text-white rounded-r border-l border-amber-400"
        aria-label="More retry options"
      >
        <svg className="w-3 h-3" viewBox="0 0 12 12" fill="currentColor">
          <path d="M6 8L1 3h10L6 8z" />
        </svg>
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 w-44 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded shadow-lg z-10">
          <button
            onClick={() => retry(true)}
            className="w-full text-left px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700"
          >
            Retry from start
          </button>
        </div>
      )}
    </div>
  );
}

function AdvanceButton({ jobId, onAdvanced }: { jobId: string; onAdvanced: () => void }) {
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);

  async function advance() {
    setBusy(true);
    setConfirm(false);
    try {
      await jobsApi.advance(jobId);
      onAdvanced();
    } finally {
      setBusy(false);
    }
  }

  if (confirm) {
    return (
      <div className="flex items-center gap-1">
        <span className="text-xs text-gray-500 dark:text-gray-400">Skip step?</span>
        <button
          onClick={advance}
          disabled={busy}
          className="px-2 py-1 text-xs font-medium bg-red-500 hover:bg-red-600 disabled:opacity-50 text-white rounded"
        >
          Confirm
        </button>
        <button
          onClick={() => setConfirm(false)}
          className="px-2 py-1 text-xs font-medium bg-gray-200 hover:bg-gray-300 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded"
        >
          Cancel
        </button>
      </div>
    );
  }

  return (
    <button
      onClick={() => setConfirm(true)}
      disabled={busy}
      className="px-3 py-1.5 text-sm font-medium bg-gray-500 hover:bg-gray-600 disabled:opacity-50 text-white rounded"
    >
      Skip Step
    </button>
  );
}

// Map API SearchResult → SearchResultRow's SearchResult shape
function mapApiResult(r: ApiSearchResult): RowSearchResult {
  return {
    id: r.id,
    title: r.release_title,
    indexer: r.indexer_name,
    size: r.size_bytes ?? 0,
    publish_date: r.publish_date ?? '',
    score: r.confidence_score,
    score_breakdown: r.score_breakdown,
    info_url: r.info_url,
  };
}

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

export default function JobDetail() {
  const { id } = useParams<{ id: string }>();
  const jobId = id ?? '';
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const statusFilter = searchParams.get('status') ?? undefined;

  // Fetch neighboring job IDs for prev/next navigation
  const { data: neighbors } = useQuery({
    queryKey: ['job-neighbors', jobId, statusFilter],
    queryFn: () => jobsApi.neighbors(jobId, statusFilter ? { status: statusFilter } : undefined),
    enabled: !!jobId,
  });

  const navigateTo = (targetId: string) => {
    const qs = statusFilter ? `?status=${encodeURIComponent(statusFilter)}` : '';
    navigate(`/queue/${targetId}${qs}`);
  };

  // Keyboard shortcuts for prev/next
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.key === 'ArrowLeft' && neighbors?.prev_id) {
        navigateTo(neighbors.prev_id);
      } else if (e.key === 'ArrowRight' && neighbors?.next_id) {
        navigateTo(neighbors.next_id);
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [neighbors, statusFilter]);

  const { data: job, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['job', jobId],
    queryFn: () => jobsApi.get(jobId),
    enabled: !!jobId,
    refetchInterval: (query) => {
      const d = query.state.data;
      if (!d) return 5000;
      if (d.status === 'awaiting_review' || d.status === 'complete' || d.status === 'cancelled') return false;
      return 5000;
    },
  });

  const { events, connected } = useJobEvents(jobId);

  // Derive the latest download_progress event for the progress bar
  const latestProgress = useMemo(() => {
    for (let i = events.length - 1; i >= 0; i--) {
      if (events[i].event_type === 'download_progress') {
        const d = events[i].payload;
        if (d && typeof d.percent === 'number') return d.percent as number;
      }
    }
    return null;
  }, [events]);

  // Filter out download_progress events from the timeline — show a progress bar instead
  const timelineEvents = useMemo(
    () => events.filter((e) => e.event_type !== 'download_progress'),
    [events],
  );

  if (!jobId) {
    return <div className="p-6 text-red-600">No job ID provided.</div>;
  }

  if (isLoading) {
    return (
      <div className="p-6 flex items-center gap-2 text-gray-500 dark:text-gray-400">
        <span className="animate-spin text-lg">⏳</span>
        <span>Loading job…</span>
      </div>
    );
  }

  if (isError || !job) {
    return (
      <div className="p-6 text-red-600">
        Failed to load job: {error instanceof Error ? error.message : 'Unknown error'}
      </div>
    );
  }

  const scene = job.scene;
  const results = [...(job.search_results ?? [])].sort(
    (a, b) => b.confidence_score - a.confidence_score,
  );

  const handleApprove = async (resultId: string) => {
    await jobsApi.approve(jobId, { result_id: resultId });
    await refetch();
  };

  return (
    <div className="flex h-full min-h-screen">
      {/* Left column — metadata + results */}
      <div className="flex-1 min-w-0 overflow-y-auto p-6 border-r border-gray-200 dark:border-gray-700">
        {/* Prev/Next navigation */}
        <div className="flex items-center justify-between mb-4">
          <button
            onClick={() => neighbors?.prev_id && navigateTo(neighbors.prev_id)}
            disabled={!neighbors?.prev_id}
            className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium rounded-lg transition disabled:opacity-30 disabled:cursor-not-allowed text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
            title="Previous job (Left arrow)"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Prev
          </button>
          {statusFilter && (
            <span className="text-xs text-gray-400 dark:text-gray-500">
              Filtered: {statusFilter.split(',').join(', ').replace(/_/g, ' ')}
            </span>
          )}
          <button
            onClick={() => neighbors?.next_id && navigateTo(neighbors.next_id)}
            disabled={!neighbors?.next_id}
            className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium rounded-lg transition disabled:opacity-30 disabled:cursor-not-allowed text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
            title="Next job (Right arrow)"
          >
            Next
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
            </svg>
          </button>
        </div>

        {/* Scene metadata */}
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-5 mb-6">
          <div className="flex items-start justify-between gap-4 flex-wrap">
            <div className="flex-1 min-w-0">
              <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 truncate">
                {scene?.title ?? job.stashdb_url}
              </h1>
              {scene?.studio_name && (
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{scene.studio_name}</p>
              )}
            </div>
            <div className="flex items-center gap-2">
              <StatusBadge status={job.status} />
              {RETRYABLE_STATUSES.has(job.status) && (
                <RetryButton jobId={jobId} isInProgress={IN_PROGRESS_STATUSES.has(job.status)} onRetried={refetch} />
              )}
              {ADVANCEABLE_STATUSES.has(job.status) && (
                <AdvanceButton jobId={jobId} onAdvanced={refetch} />
              )}
            </div>
          </div>

          <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
            {scene?.performers && scene.performers.length > 0 && (
              <>
                <dt className="text-gray-500 dark:text-gray-400 font-medium">Performers</dt>
                <dd className="text-gray-800 dark:text-gray-200">
                  {scene.performers.map((p) => p.name).join(', ')}
                </dd>
              </>
            )}
            {scene?.release_date && (
              <>
                <dt className="text-gray-500 dark:text-gray-400 font-medium">Release date</dt>
                <dd className="text-gray-800 dark:text-gray-200">{scene.release_date}</dd>
              </>
            )}
            {scene?.duration_seconds != null && (
              <>
                <dt className="text-gray-500 dark:text-gray-400 font-medium">Duration</dt>
                <dd className="text-gray-800 dark:text-gray-200">{formatDuration(scene.duration_seconds)}</dd>
              </>
            )}
            {scene?.stashdb_scene_id && (
              <>
                <dt className="text-gray-500 dark:text-gray-400 font-medium">StashDB</dt>
                <dd>
                  <a
                    href={`https://stashdb.org/scenes/${scene.stashdb_scene_id}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-600 dark:text-blue-400 hover:underline text-xs"
                  >
                    View on StashDB ↗
                  </a>
                </dd>
              </>
            )}
          </dl>

          {job.error_message && (
            <div className="mt-3 p-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-xs text-red-700 dark:text-red-400">
              {job.error_message}
            </div>
          )}
        </div>

        {/* Custom search builder — shown when automatic search failed */}
        {job.status === 'search_failed' && scene && (
          <CustomSearchPanel jobId={jobId} scene={scene} onSearchComplete={refetch} />
        )}

        {/* Download progress bar */}
        {latestProgress !== null && (
          <div className="mb-4 bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            <div className="flex items-center justify-between text-xs text-gray-600 dark:text-gray-400 mb-1">
              <span>Download progress</span>
              <span>{latestProgress.toFixed(1)}%</span>
            </div>
            <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
              <div
                className="bg-blue-500 h-2 rounded-full transition-all duration-300"
                style={{ width: `${Math.min(100, latestProgress)}%` }}
              />
            </div>
          </div>
        )}

        {/* Search results */}
        {results.length > 0 && (
          <div>
            <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
              Search Results ({results.length})
            </h2>
            <div className="flex flex-col gap-2">
              {results.map((r) => (
                <SearchResultRow
                  key={r.id}
                  result={mapApiResult(r)}
                  onApprove={
                    job.status === 'awaiting_review' || job.status === 'search_failed'
                      ? () => handleApprove(r.id)
                      : undefined
                  }
                  approveLabel={job.status === 'search_failed' ? 'Override' : undefined}
                />
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Right column — event timeline */}
      <div className="w-80 xl:w-96 flex-shrink-0 flex flex-col h-screen overflow-hidden p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300">Timeline</h2>
          <span
            className={`inline-flex items-center gap-1 text-xs ${
              connected ? 'text-green-600 dark:text-green-400' : 'text-gray-400 dark:text-gray-500'
            }`}
          >
            <span
              className={`w-1.5 h-1.5 rounded-full ${
                connected ? 'bg-green-500' : 'bg-gray-300 dark:bg-gray-600'
              }`}
            />
            {connected ? 'Live' : 'Disconnected'}
          </span>
        </div>
        <div className="flex-1 overflow-hidden">
          <JobEventTimeline events={timelineEvents} live={true} />
        </div>
      </div>
    </div>
  );
}
