import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { jobsApi } from '../api/client';
import type { JobSummary, JobStatus, JobType } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import ConfirmModal from '../components/ConfirmModal';
import useStore from '../hooks/useStore';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const ALL_STATUSES: JobStatus[] = [
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

function formatDate(ts: string): string {
  try {
    return new Date(ts).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  } catch {
    return ts;
  }
}

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

function getTypeIcon(type: JobType): string {
  const icons: Record<JobType, string> = {
    scene: '🎬',
    performer: '👤',
    studio: '🏢',
  };
  return icons[type] ?? '?';
}

function isFailedStatus(status: JobStatus): boolean {
  return status.endsWith('_failed');
}

function isCancellableStatus(status: JobStatus): boolean {
  return status !== 'complete' && status !== 'cancelled';
}

// ---------------------------------------------------------------------------
// Skeleton row
// ---------------------------------------------------------------------------

const SkeletonRow: React.FC = () => (
  <tr className="animate-pulse">
    <td className="px-3 py-2"><div className="w-14 h-9 bg-gray-200 dark:bg-gray-700 rounded" /></td>
    <td className="px-3 py-3"><div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-48" /></td>
    <td className="px-3 py-3"><div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-28" /></td>
    <td className="px-3 py-3"><div className="h-5 bg-gray-200 dark:bg-gray-700 rounded w-20" /></td>
    <td className="px-3 py-3"><div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-24" /></td>
    <td className="px-3 py-3"><div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-16" /></td>
    <td className="px-3 py-3"><div className="h-7 bg-gray-200 dark:bg-gray-700 rounded w-20" /></td>
  </tr>
);

// ---------------------------------------------------------------------------
// Job row
// ---------------------------------------------------------------------------

interface JobRowProps {
  job: JobSummary;
  statusFilter?: string;
  onCancel: (job: JobSummary) => void;
  onRetry: (id: string) => void;
}

const JobRow: React.FC<JobRowProps> = ({ job, statusFilter, onCancel, onRetry }) => {
  const navigate = useNavigate();
  const safeMode = useStore((s) => s.safeMode);

  const title = job.scene?.title ?? job.stashdb_url;
  const studio = job.scene?.studio_name ?? '—';

  const handleRowClick = (e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('button')) return;
    const qs = statusFilter ? `?status=${encodeURIComponent(statusFilter)}` : '';
    navigate(`/queue/${job.id}${qs}`);
  };

  return (
    <tr
      onClick={handleRowClick}
      className="cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors border-b border-gray-100 dark:border-gray-800 last:border-0"
    >
      <td className="px-3 py-2 w-16">
        {!safeMode && job.scene?.image_url ? (
          <div className="relative group/thumb">
            <img
              src={job.scene.image_url}
              alt={title}
              className="w-14 h-9 rounded object-cover bg-gray-200 dark:bg-gray-700"
              loading="lazy"
            />
            <img
              src={job.scene.image_url}
              alt={title}
              className="hidden group-hover/thumb:block absolute z-50 left-full top-0 ml-2 max-w-sm rounded-lg shadow-xl border border-gray-200 dark:border-gray-700 pointer-events-none"
            />
          </div>
        ) : (
          <span className="flex items-center justify-center w-14 h-9 rounded bg-gray-100 dark:bg-gray-800 text-base" title={job.type}>
            {getTypeIcon(job.type)}
          </span>
        )}
      </td>
      <td className="px-3 py-3 max-w-xs">
        <span className="text-sm font-medium text-gray-800 dark:text-gray-200 truncate block" title={title}>
          {title}
        </span>
      </td>
      <td className="px-3 py-3 max-w-[10rem]">
        <span className="text-sm text-gray-600 dark:text-gray-400 truncate block" title={studio}>
          {studio}
        </span>
      </td>
      <td className="px-3 py-3">
        <StatusBadge status={job.status} />
      </td>
      <td className="px-3 py-3 text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">
        {formatDate(job.created_at)}
      </td>
      <td className="px-3 py-3 text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">
        {formatRelativeTime(job.updated_at)}
      </td>
      <td className="px-3 py-3">
        <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
          {isCancellableStatus(job.status) && (
            <button
              onClick={() => onCancel(job)}
              className="px-2 py-1 text-xs font-medium text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/20 rounded hover:bg-red-100 dark:hover:bg-red-900/30 transition"
            >
              Cancel
            </button>
          )}
          {isFailedStatus(job.status) && (
            <button
              onClick={() => onRetry(job.id)}
              className="px-2 py-1 text-xs font-medium text-blue-700 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20 rounded hover:bg-blue-100 dark:hover:bg-blue-900/30 transition"
            >
              Retry
            </button>
          )}
          <button
            onClick={() => navigate(`/queue/${job.id}`)}
            className="px-2 py-1 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded hover:bg-gray-200 dark:hover:bg-gray-700 transition"
          >
            View
          </button>
        </div>
      </td>
    </tr>
  );
};

// ---------------------------------------------------------------------------
// Pipeline stats
// ---------------------------------------------------------------------------

/** Primary pipeline stages — each groups one or more raw statuses. */
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

  // Bottleneck = highest count among active (non-terminal) stages
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
// Queue page
// ---------------------------------------------------------------------------

interface Filters {
  status: JobStatus | '';
  search: string;
}

export default function Queue() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  const { data: statsData } = useQuery({
    queryKey: ['job-stats'],
    queryFn: () => jobsApi.stats(),
    refetchInterval: 15_000,
  });

  const [filters, setFilters] = useState<Filters>(() => ({
    status: (searchParams.get('status') ?? '') as JobStatus | '',
    search: searchParams.get('search') ?? '',
  }));

  // Status dropdown state
  const [statusDropdownOpen, setStatusDropdownOpen] = useState(false);
  const [statusSearch, setStatusSearch] = useState('');
  const statusDropdownRef = useRef<HTMLDivElement>(null);

  // Title search state
  const [titleFocused, setTitleFocused] = useState(false);

  // Close status dropdown on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (statusDropdownRef.current && !statusDropdownRef.current.contains(e.target as Node)) {
        setStatusDropdownOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  // Sync filters to URL search params
  useEffect(() => {
    const params = new URLSearchParams();
    if (filters.status) {
      params.set('status', filters.status);
    }
    if (filters.search) {
      params.set('search', filters.search);
    }
    setSearchParams(params, { replace: true });
  }, [filters, setSearchParams]);

  const [debouncedSearch, setDebouncedSearch] = useState(filters.search);
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(filters.search), 300);
    return () => clearTimeout(timer);
  }, [filters.search]);

  const [cancelTarget, setCancelTarget] = useState<JobSummary | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const queryKey = [
    'jobs',
    filters.status,
    debouncedSearch,
  ];

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isError,
    error,
  } = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) => {
      const params: Parameters<typeof jobsApi.list>[0] = {
        limit: 50,
      };
      if (filters.status) {
        params.status = filters.status;
      }
      if (debouncedSearch) {
        params.search = debouncedSearch;
      }
      if (pageParam) {
        params.before = pageParam as string;
      }
      return jobsApi.list(params);
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  });

  const handleIntersect = useCallback(
    (entries: IntersectionObserverEntry[]) => {
      if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
        void fetchNextPage();
      }
    },
    [fetchNextPage, hasNextPage, isFetchingNextPage],
  );

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;
    const observer = new IntersectionObserver(handleIntersect, { threshold: 0.1 });
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [handleIntersect]);

  const jobs: JobSummary[] = data?.pages.flatMap((p) => p.jobs) ?? [];

  const handleCancelClick = (job: JobSummary) => {
    setCancelTarget(job);
  };

  const handleCancelConfirm = async () => {
    if (!cancelTarget) return;
    try {
      await jobsApi.cancel(cancelTarget.id);
      await queryClient.invalidateQueries({ queryKey });
    } catch {
      // Error handling could be enhanced with a toast
    } finally {
      setCancelTarget(null);
    }
  };

  const handleRetry = async (id: string) => {
    try {
      await jobsApi.retry(id);
      await queryClient.invalidateQueries({ queryKey });
    } catch {
      // Error handling could be enhanced with a toast
    }
  };

  const statusLabel = filters.status
    ? filters.status.replace(/_/g, ' ')
    : 'All statuses';

  return (
    <div className="flex flex-col h-full">
      <div className="flex flex-wrap items-center gap-4 mb-4">
        <h1 className="text-xl font-bold text-gray-900 dark:text-gray-100">Queue</h1>
        {statsData && <PipelineStats counts={statsData.counts} />}
      </div>

      <div className="flex-1 overflow-auto min-h-0 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-xl">
        <table className="w-full text-left border-collapse">
          <thead className="sticky top-0 bg-white dark:bg-gray-900 z-10 border-b border-gray-200 dark:border-gray-700">
            <tr>
              <th className="px-3 py-2.5 w-16" />
              {/* Title column header with inline search */}
              <th className="px-3 py-2.5">
                <div className="relative">
                  <input
                    type="text"
                    value={filters.search}
                    onChange={(e) => setFilters((f) => ({ ...f, search: e.target.value }))}
                    onFocus={() => setTitleFocused(true)}
                    onBlur={() => setTitleFocused(false)}
                    placeholder="Title"
                    className={`w-full text-xs font-semibold uppercase tracking-wide bg-transparent border-b transition-colors focus:outline-none py-0.5 pr-6 ${
                      titleFocused || filters.search
                        ? 'border-blue-500 text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500'
                        : 'border-transparent text-gray-500 dark:text-gray-400 placeholder-gray-500 dark:placeholder-gray-400'
                    }`}
                  />
                  {filters.search ? (
                    <button
                      type="button"
                      onClick={() => setFilters((f) => ({ ...f, search: '' }))}
                      className="absolute right-0 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-xs"
                    >
                      ✕
                    </button>
                  ) : (
                    <svg
                      className="absolute right-0 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-400 pointer-events-none"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                    </svg>
                  )}
                </div>
              </th>
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">Studio</th>
              {/* Status column header with dropdown */}
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide relative">
                <div ref={statusDropdownRef} className="inline-block">
                  <button
                    type="button"
                    onClick={() => {
                      setStatusDropdownOpen((v) => !v);
                      setStatusSearch('');
                    }}
                    className={`inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-gray-200 transition ${
                      filters.status ? 'text-blue-600 dark:text-blue-400' : ''
                    }`}
                  >
                    <span className="capitalize">{statusLabel}</span>
                    <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                    </svg>
                  </button>
                  {statusDropdownOpen && (
                    <div className="absolute z-50 mt-1 left-0 w-56 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg">
                      <div className="p-2">
                        <input
                          type="text"
                          autoFocus
                          value={statusSearch}
                          onChange={(e) => setStatusSearch(e.target.value)}
                          placeholder="Search statuses…"
                          className="w-full px-2 py-1.5 text-xs border border-gray-200 dark:border-gray-600 rounded bg-gray-50 dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
                        />
                      </div>
                      <ul className="max-h-48 overflow-y-auto text-xs font-normal">
                        <li>
                          <button
                            type="button"
                            onClick={() => {
                              setFilters((f) => ({ ...f, status: '' }));
                              setStatusDropdownOpen(false);
                            }}
                            className={`w-full text-left px-3 py-1.5 hover:bg-gray-100 dark:hover:bg-gray-700 ${
                              !filters.status ? 'text-blue-600 dark:text-blue-400 font-medium' : 'text-gray-700 dark:text-gray-300'
                            }`}
                          >
                            All statuses
                          </button>
                        </li>
                        {ALL_STATUSES
                          .filter((s) => s.replace(/_/g, ' ').toLowerCase().includes(statusSearch.toLowerCase()))
                          .map((s) => (
                            <li key={s}>
                              <button
                                type="button"
                                onClick={() => {
                                  setFilters((f) => ({ ...f, status: s }));
                                  setStatusDropdownOpen(false);
                                }}
                                className={`w-full text-left px-3 py-1.5 hover:bg-gray-100 dark:hover:bg-gray-700 capitalize ${
                                  filters.status === s ? 'text-blue-600 dark:text-blue-400 font-medium' : 'text-gray-700 dark:text-gray-300'
                                }`}
                              >
                                {s.replace(/_/g, ' ')}
                              </button>
                            </li>
                          ))}
                      </ul>
                    </div>
                  )}
                </div>
              </th>
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">Created</th>
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">Updated</th>
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">Actions</th>
            </tr>
          </thead>
          <tbody>
            {isLoading &&
              Array.from({ length: 8 }).map((_, i) => <SkeletonRow key={i} />)}

            {isError && (
              <tr>
                <td colSpan={7} className="px-4 py-12 text-center">
                  <p className="text-sm text-red-500 font-medium">Failed to load jobs.</p>
                  <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">
                    {error instanceof Error ? error.message : 'Unknown error'}
                  </p>
                </td>
              </tr>
            )}

            {!isLoading && !isError && jobs.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-12 text-center">
                  <p className="text-sm text-gray-500 dark:text-gray-400">No jobs found.</p>
                  {(filters.status || filters.search) && (
                    <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">Try adjusting your filters.</p>
                  )}
                </td>
              </tr>
            )}

            {jobs.map((job) => (
              <JobRow
                key={job.id}
                job={job}
                statusFilter={filters.status || undefined}
                onCancel={handleCancelClick}
                onRetry={handleRetry}
              />
            ))}

            {isFetchingNextPage && Array.from({ length: 3 }).map((_, i) => <SkeletonRow key={`next-${i}`} />)}
          </tbody>
        </table>

        <div ref={sentinelRef} className="h-4" />

        {!hasNextPage && jobs.length > 0 && (
          <p className="text-center text-xs text-gray-400 dark:text-gray-500 py-3">
            All {jobs.length} job{jobs.length !== 1 ? 's' : ''} loaded.
          </p>
        )}
      </div>

      <ConfirmModal
        isOpen={cancelTarget !== null}
        title="Cancel job?"
        message={`Are you sure you want to cancel "${cancelTarget?.scene?.title ?? cancelTarget?.stashdb_url ?? 'this job'}"? This cannot be undone.`}
        onConfirm={() => void handleCancelConfirm()}
        onCancel={() => setCancelTarget(null)}
      />
    </div>
  );
}
