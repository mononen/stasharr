import { useMemo, useState, useRef, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { jobsApi } from '../api/client';
import type { SearchResult as ApiSearchResult, JobStatus } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import JobEventTimeline from '../components/JobEventTimeline';
import SearchResultRow from '../components/SearchResultRow';
import type { SearchResult as RowSearchResult } from '../components/SearchResultRow';
import CustomSearchPanel from '../components/CustomSearchPanel';
import useStore from '../hooks/useStore';
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

const OVERRIDE_STATUSES: JobStatus[] = [
  'submitted',
  'resolving',
  'resolve_failed',
  'resolved',
  'searching',
  'search_failed',
  'awaiting_review',
  'approved',
  'downloading',
  'download_failed',
  'download_complete',
  'moving',
  'move_failed',
  'moved',
  'scanning',
  'scan_failed',
  'complete',
  'cancelled',
];

function StatusOverrideButton({ jobId, currentStatus, onOverridden }: { jobId: string; currentStatus: string; onOverridden: () => void }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  async function apply(status: JobStatus) {
    setBusy(true);
    setOpen(false);
    try {
      await jobsApi.setStatus(jobId, status);
      onOverridden();
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        disabled={busy}
        title="Override status"
        className="px-2 py-1.5 text-xs font-medium text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded transition disabled:opacity-50"
      >
        Override
      </button>
      {open && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          aria-modal="true"
          role="dialog"
        >
          <div
            className="absolute inset-0 bg-black/40"
            onClick={() => setOpen(false)}
            aria-hidden="true"
          />
          <div className="relative bg-white dark:bg-gray-900 rounded-xl shadow-xl p-5 max-w-xs w-full mx-4">
            <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100 mb-1">Override Status</h2>
            <p className="text-xs text-gray-500 dark:text-gray-400 mb-3">
              Current: <span className="font-medium capitalize">{currentStatus.replace(/_/g, ' ')}</span>
            </p>
            <ul className="max-h-72 overflow-y-auto divide-y divide-gray-100 dark:divide-gray-800">
              {OVERRIDE_STATUSES.map((s) => (
                <li key={s}>
                  <button
                    onClick={() => apply(s)}
                    className={`w-full text-left px-3 py-2 text-sm capitalize rounded transition hover:bg-gray-50 dark:hover:bg-gray-800 ${
                      s === currentStatus
                        ? 'text-blue-600 dark:text-blue-400 font-medium'
                        : 'text-gray-700 dark:text-gray-300'
                    }`}
                  >
                    {s.replace(/_/g, ' ')}
                  </button>
                </li>
              ))}
            </ul>
            <div className="mt-3 flex justify-end">
              <button
                onClick={() => setOpen(false)}
                className="px-3 py-1.5 text-xs font-medium text-gray-600 dark:text-gray-400 bg-gray-100 dark:bg-gray-800 rounded hover:bg-gray-200 dark:hover:bg-gray-700 transition"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function ScanWatchDirButton({ jobId, onMatched }: { jobId: string; onMatched: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function scan() {
    setBusy(true);
    setError(null);
    try {
      await jobsApi.localMatch(jobId);
      onMatched();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No match found');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex items-center gap-2">
      <button
        onClick={scan}
        disabled={busy}
        className="px-3 py-1.5 text-sm font-medium text-blue-700 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20 rounded hover:bg-blue-100 dark:hover:bg-blue-900/30 disabled:opacity-50 transition"
      >
        {busy ? 'Scanning…' : 'Scan Watch Dir'}
      </button>
      {error && <span className="text-sm text-red-600 dark:text-red-400">{error}</span>}
    </div>
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

  const [timelineWidth, setTimelineWidth] = useState(320);
  const isDragging = useRef(false);
  const dragStartX = useRef(0);
  const dragStartWidth = useRef(0);

  const handleResizeMouseDown = (e: React.MouseEvent) => {
    isDragging.current = true;
    dragStartX.current = e.clientX;
    dragStartWidth.current = timelineWidth;
    e.preventDefault();
  };

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      const delta = dragStartX.current - e.clientX;
      setTimelineWidth(Math.max(240, Math.min(600, dragStartWidth.current + delta)));
    };
    const onMouseUp = () => { isDragging.current = false; };
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    return () => {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };
  }, []);
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
  const safeMode = useStore((s) => s.safeMode);

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
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 mb-6 overflow-hidden">
          <div className="flex items-stretch">
            {!safeMode && scene?.image_url && (
              <div className="relative group/thumb flex-shrink-0 w-1/3 p-4 bg-gray-100 dark:bg-gray-800 border-r border-gray-100 dark:border-gray-800">
                <img
                  src={scene.image_url}
                  alt={scene?.title ?? ''}
                  className="w-full h-full object-contain rounded-lg"
                  loading="lazy"
                />
                <div className="hidden group-hover/thumb:block fixed z-[100] left-1/4 top-1/4 w-1/2 pointer-events-none shadow-2xl rounded-lg border-4 border-white dark:border-gray-700 overflow-hidden bg-white dark:bg-gray-900">
                  <img
                    src={scene.image_url}
                    alt={scene?.title ?? ''}
                    className="w-full h-auto"
                  />
                </div>
              </div>
            )}
            <div className={`flex-1 min-w-0 py-5 px-5`}>
              <div className="flex items-start justify-between gap-4 flex-wrap">
                <div className="min-w-0">
                  <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 truncate">
                    {scene?.title ?? job.stashdb_url}
                  </h1>
                  {scene?.studio_name && (
                    <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{scene.studio_name}</p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <StatusBadge status={job.status} />
                  {scene?.stash_link && (
                    <a
                      href={scene.stash_link}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="px-3 py-1.5 text-sm font-medium text-teal-700 dark:text-teal-400 bg-teal-50 dark:bg-teal-900/20 rounded hover:bg-teal-100 dark:hover:bg-teal-900/30 transition"
                    >
                      View in Stash ↗
                    </a>
                  )}
                  {job.status !== 'complete' && job.status !== 'cancelled' && (
                    <button
                      onClick={async () => {
                        if (window.confirm('Are you sure you want to cancel this job?')) {
                          try {
                            await jobsApi.cancel(jobId);
                            await refetch();
                          } catch (err) {
                            alert(err instanceof Error ? err.message : 'Failed to cancel');
                          }
                        }
                      }}
                      className="px-3 py-1.5 text-sm font-medium text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/20 rounded hover:bg-red-100 dark:hover:bg-red-900/30 transition"
                    >
                      Cancel
                    </button>
                  )}
                  {job.status === 'search_failed' && (
                    <ScanWatchDirButton jobId={jobId} onMatched={refetch} />
                  )}
                  {RETRYABLE_STATUSES.has(job.status) && (
                    <RetryButton jobId={jobId} isInProgress={IN_PROGRESS_STATUSES.has(job.status)} onRetried={refetch} />
                  )}
                  {ADVANCEABLE_STATUSES.has(job.status) && (
                    <AdvanceButton jobId={jobId} onAdvanced={refetch} />
                  )}
                  <StatusOverrideButton jobId={jobId} currentStatus={job.status} onOverridden={refetch} />
                </div>
              </div>

              <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm mb-4">
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
                <div className="p-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-xs text-red-700 dark:text-red-400">
                  {job.error_message}
                </div>
              )}
            </div>
          </div>
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

      {/* Resize handle */}
      <div
        onMouseDown={handleResizeMouseDown}
        className="w-1 flex-shrink-0 cursor-col-resize bg-gray-200 dark:bg-gray-700 hover:bg-blue-400 dark:hover:bg-blue-600 transition-colors"
        title="Drag to resize timeline"
      />

      {/* Right column — event timeline */}
      <div className="flex-shrink-0 flex flex-col h-screen overflow-hidden p-4" style={{ width: timelineWidth }}>
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
