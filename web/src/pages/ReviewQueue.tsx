import { useCallback, useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { jobsApi } from '../api/client';
import type { JobSummary, SearchResult as ApiSearchResult } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import SearchResultRow from '../components/SearchResultRow';
import CustomSearchPanel from '../components/CustomSearchPanel';
import JobEventTimeline from '../components/JobEventTimeline';
import { useJobEvents } from '../hooks/useJobEvents';
import useStore from '../hooks/useStore';
import type { SearchResult as RowSearchResult } from '../components/SearchResultRow';

// ---------------------------------------------------------------------------
// Type adapters (same as JobDetail)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function relativeTime(dateStr: string): string {
  try {
    const diff = Date.now() - new Date(dateStr).getTime();
    const minutes = Math.floor(diff / 60_000);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);
    if (days > 0) return `${days}d ago`;
    if (hours > 0) return `${hours}h ago`;
    if (minutes > 0) return `${minutes}m ago`;
    return 'just now';
  } catch {
    return dateStr;
  }
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
function topScore(_job: JobSummary): number | null {
  return null;
}

// ---------------------------------------------------------------------------
// Shortcut help overlay
// ---------------------------------------------------------------------------

const SHORTCUTS = [
  { key: '1–9', description: 'Approve result by rank' },
  { key: '↑ / ↓', description: 'Navigate queue list' },
  { key: 's', description: 'Skip current item (cancel)' },
  { key: '?', description: 'Toggle this help overlay' },
];

function ShortcutHelp({ onClose }: { onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <div className="bg-white dark:bg-gray-900 rounded-xl shadow-xl border border-gray-200 dark:border-gray-700 p-6 w-80">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100">Keyboard shortcuts</h3>
          <button
            onClick={onClose}
            className="text-gray-400 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 text-lg leading-none"
          >
            ×
          </button>
        </div>
        <table className="w-full text-sm">
          <tbody>
            {SHORTCUTS.map(({ key, description }) => (
              <tr key={key} className="border-t border-gray-100 dark:border-gray-800 first:border-t-0">
                <td className="py-2 pr-4">
                  <kbd className="px-2 py-0.5 bg-gray-100 dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-xs font-mono text-gray-700 dark:text-gray-300">
                    {key}
                  </kbd>
                </td>
                <td className="py-2 text-gray-700 dark:text-gray-300">{description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Left panel row
// ---------------------------------------------------------------------------

interface QueueRowProps {
  job: JobSummary;
  selected: boolean;
  onClick: () => void;
  topConfidence: number | null;
}

function QueueRow({ job, selected, onClick, topConfidence }: QueueRowProps) {
  const title = job.scene?.title ?? job.stashdb_url;
  const studio = job.scene?.studio_name ?? null;

  const isMissing = job.status === 'search_failed';

  return (
    <button
      onClick={onClick}
      className={`w-full text-left px-3 py-2.5 border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition ${
        selected
          ? isMissing
            ? 'bg-orange-50 dark:bg-orange-900/20 border-l-2 border-l-orange-400'
            : 'bg-amber-50 dark:bg-amber-900/20 border-l-2 border-l-amber-400'
          : ''
      }`}
    >
      <p className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate" title={title}>
        {title}
      </p>
      <div className="flex items-center gap-2 mt-0.5 text-xs text-gray-500 dark:text-gray-400">
        {studio && <span className="truncate max-w-[100px]">{studio}</span>}
        {isMissing && (
          <span className="text-orange-500 dark:text-orange-400 font-medium">no results</span>
        )}
        {topConfidence !== null && (
          <span className="font-medium text-gray-700 dark:text-gray-300">{topConfidence}%</span>
        )}
        <span className="ml-auto flex-shrink-0">{relativeTime(job.created_at)}</span>
      </div>
    </button>
  );
}

// ---------------------------------------------------------------------------
// Right detail panel
// ---------------------------------------------------------------------------

interface DetailPanelProps {
  jobId: string;
  onApproved: () => void;
  onSkipped: () => void;
  onListRefresh: () => void;
}

function DetailPanel({ jobId, onApproved, onSkipped, onListRefresh }: DetailPanelProps) {
  const safeMode = useStore((s) => s.safeMode);
  const { data: job, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['job', jobId],
    queryFn: () => jobsApi.get(jobId),
    enabled: !!jobId,
  });
  const { events, connected } = useJobEvents(jobId);

  const results: ApiSearchResult[] = useMemo(
    () =>
      [...(job?.search_results ?? [])].sort(
        (a, b) => b.confidence_score - a.confidence_score,
      ),
    [job],
  );

  const handleApprove = async (resultId: string) => {
    await jobsApi.approve(jobId, { result_id: resultId });
    await refetch();
    onApproved();
  };

  const handleSkip = async () => {
    await jobsApi.cancel(jobId);
    onSkipped();
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500">
        <span className="animate-spin mr-2">⏳</span> Loading…
      </div>
    );
  }

  if (isError || !job) {
    return (
      <div className="p-6 text-red-600">
        Failed to load: {error instanceof Error ? error.message : 'Unknown error'}
      </div>
    );
  }

  const scene = job.scene;

  return (
    <div className="flex h-full overflow-hidden">
      {/* Main content */}
      <div className="flex-1 overflow-y-auto p-5">
        {/* Scene detail card */}
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 mb-5 overflow-hidden">
          <div className="flex items-stretch">
            {!safeMode && scene?.image_url && (
              <div className="relative group/thumb flex-shrink-0 w-2/5 bg-gray-100 dark:bg-gray-800 border-r border-gray-100 dark:border-gray-800">
                <img
                  src={scene.image_url}
                  alt={scene.title ?? ''}
                  className="w-full h-full object-cover"
                  loading="lazy"
                />
                <div className="hidden group-hover/thumb:block fixed z-[100] left-1/4 top-1/4 w-1/2 pointer-events-none shadow-2xl rounded-lg border-4 border-white dark:border-gray-700 overflow-hidden">
                  <img src={scene.image_url} alt={scene.title ?? ''} className="w-full h-auto" />
                </div>
              </div>
            )}
            <div className="flex-1 min-w-0 p-4">
              <div className="flex items-start justify-between gap-3 flex-wrap mb-3">
                <div className="min-w-0">
                  <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100">
                    {scene?.title ?? job.stashdb_url}
                  </h2>
                  {scene?.studio_name && (
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">{scene.studio_name}</p>
                  )}
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  <StatusBadge status={job.status} />
                  <button
                    onClick={handleSkip}
                    className="px-3 py-1 text-xs font-medium bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 border border-gray-300 dark:border-gray-600 rounded hover:bg-gray-200 dark:hover:bg-gray-700 transition"
                  >
                    Skip
                  </button>
                </div>
              </div>
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-xs">
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
                    <dt className="text-gray-500 dark:text-gray-400 font-medium">Date</dt>
                    <dd className="text-gray-800 dark:text-gray-200">{scene.release_date}</dd>
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
                        className="text-blue-600 dark:text-blue-400 hover:underline"
                      >
                        View ↗
                      </a>
                    </dd>
                  </>
                )}
              </dl>
            </div>
          </div>
        </div>

        {/* Custom search for search_failed jobs */}
        {job.status === 'search_failed' && scene && (
          <CustomSearchPanel
            jobId={jobId}
            scene={scene}
            onSearchComplete={async () => {
              await refetch();
              onListRefresh();
            }}
          />
        )}

        {/* Results */}
        {results.length === 0 && job.status !== 'search_failed' && (
          <p className="text-sm text-gray-500 dark:text-gray-400 italic">No search results available.</p>
        )}
        {results.length > 0 && (
          <div className="flex flex-col gap-2">
            {results.map((r, idx) => (
              <div key={r.id} className="relative">
                <span className="absolute -left-0 top-2 w-5 h-5 flex items-center justify-center bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-xs font-bold rounded-full z-10 -ml-2.5">
                  {idx + 1}
                </span>
                <div className="ml-4">
                  <SearchResultRow
                    result={mapApiResult(r)}
                    onApprove={
                      job.status === 'awaiting_review'
                        ? () => handleApprove(r.id)
                        : undefined
                    }
                  />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Timeline sidebar */}
      <div className="w-72 flex-shrink-0 border-l border-gray-200 dark:border-gray-700 flex flex-col overflow-hidden p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300">Timeline</h2>
          <span className={`inline-flex items-center gap-1 text-xs ${connected ? 'text-green-600 dark:text-green-400' : 'text-gray-400 dark:text-gray-500'}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-green-500' : 'bg-gray-300 dark:bg-gray-600'}`} />
            {connected ? 'Live' : 'Disconnected'}
          </span>
        </div>
        <div className="flex-1 overflow-hidden">
          <JobEventTimeline events={events} live={true} />
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function ReviewQueue() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [showHelp, setShowHelp] = useState(false);
  const [showMissing, setShowMissing] = useState(false);

  const statusFilter = showMissing ? 'awaiting_review,search_failed' : 'awaiting_review';

  const {
    data,
    isLoading,
    isError,
    error,
    refetch: refetchList,
  } = useQuery({
    queryKey: ['jobs', statusFilter],
    queryFn: () => jobsApi.list({ status: statusFilter, limit: 200 }),
    refetchInterval: 15_000,
  });

  const jobs: JobSummary[] = useMemo(
    () =>
      [...(data?.jobs ?? [])].sort(
        (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
      ),
    [data],
  );

  const effectiveSelectedId = useMemo<string | null>(() => {
    if (jobs.length === 0) return null;
    if (selectedId !== null && jobs.find((j) => j.id === selectedId)) return selectedId;
    return jobs[0].id;
  }, [jobs, selectedId]);

  const handleApprovedOrSkipped = useCallback(async () => {
    await refetchList();
  }, [refetchList]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement
      ) {
        return;
      }

      if (e.key === '?') {
        setShowHelp((v) => !v);
        return;
      }

      if (e.key === 'Escape') {
        setShowHelp(false);
        return;
      }

      if (e.key === 'ArrowDown') {
        e.preventDefault();
        const idx = jobs.findIndex((j) => j.id === effectiveSelectedId);
        const next = jobs[Math.min(idx + 1, jobs.length - 1)];
        setSelectedId(next?.id ?? effectiveSelectedId);
        return;
      }

      if (e.key === 'ArrowUp') {
        e.preventDefault();
        const idx = jobs.findIndex((j) => j.id === effectiveSelectedId);
        const next = jobs[Math.max(idx - 1, 0)];
        setSelectedId(next?.id ?? effectiveSelectedId);
        return;
      }

      if (e.key === 's' && effectiveSelectedId) {
        jobsApi.cancel(effectiveSelectedId).then(() => refetchList());
        return;
      }

      const digit = parseInt(e.key, 10);
      if (!isNaN(digit) && digit >= 1 && digit <= 9 && effectiveSelectedId) {
        window.dispatchEvent(
          new CustomEvent('stasharr:approve-rank', { detail: { rank: digit } }),
        );
        return;
      }
    };

    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [jobs, effectiveSelectedId, refetchList]);

  return (
    <div className="flex -m-6 h-screen overflow-hidden">
      {/* Left list panel */}
      <div className="w-72 flex-shrink-0 border-r border-gray-200 dark:border-gray-700 flex flex-col overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 flex items-center justify-between gap-2">
          <h1 className="text-sm font-semibold text-gray-800 dark:text-gray-200">Review Queue</h1>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setShowMissing((v) => !v)}
              title="Show scenes with no search results"
              className={`px-2 py-0.5 rounded-full text-xs font-medium transition ${
                showMissing
                  ? 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400 ring-1 ring-orange-400 dark:ring-orange-600'
                  : 'bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400 hover:text-orange-600 dark:hover:text-orange-400'
              }`}
            >
              missing
            </button>
            {!isLoading && (
              <span className="text-xs text-gray-500 dark:text-gray-400">
                {jobs.length} item{jobs.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          {isLoading && (
            <div className="p-4 text-sm text-gray-500 dark:text-gray-400 text-center">Loading…</div>
          )}
          {isError && (
            <div className="p-4 text-sm text-red-600">
              {error instanceof Error ? error.message : 'Error loading queue'}
            </div>
          )}
          {!isLoading && !isError && jobs.length === 0 && (
            <div className="p-6 text-center">
              <p className="text-2xl mb-2">✓</p>
              <p className="text-sm text-gray-500 dark:text-gray-400 font-medium">Queue is empty</p>
              <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">
                No items awaiting review.
              </p>
            </div>
          )}
          {jobs.map((job) => (
            <QueueRow
              key={job.id}
              job={job}
              selected={job.id === effectiveSelectedId}
              onClick={() => setSelectedId(job.id)}
              topConfidence={topScore(job)}
            />
          ))}
        </div>

        {/* Shortcut hint */}
        <div className="px-3 py-2 border-t border-gray-100 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/50">
          <button
            onClick={() => setShowHelp(true)}
            className="text-xs text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 transition"
          >
            ? keyboard shortcuts
          </button>
        </div>
      </div>

      {/* Right detail panel */}
      <div className="flex-1 min-w-0 overflow-hidden">
        {effectiveSelectedId ? (
          <DetailPanel
            key={effectiveSelectedId}
            jobId={effectiveSelectedId}
            onApproved={handleApprovedOrSkipped}
            onSkipped={handleApprovedOrSkipped}
            onListRefresh={refetchList}
          />
        ) : (
          <div className="flex flex-col items-center justify-center h-full text-gray-400 dark:text-gray-500">
            {!isLoading && jobs.length === 0 ? (
              <>
                <p className="text-3xl mb-3">✓</p>
                <p className="text-base font-medium text-gray-500 dark:text-gray-400">All caught up!</p>
                <p className="text-sm text-gray-400 dark:text-gray-500 mt-1">
                  No items in the review queue.
                </p>
              </>
            ) : (
              <p className="text-sm">Select an item from the list.</p>
            )}
          </div>
        )}
      </div>

      {/* Keyboard shortcut overlay */}
      {showHelp && <ShortcutHelp onClose={() => setShowHelp(false)} />}
    </div>
  );
}
