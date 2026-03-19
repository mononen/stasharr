import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import { jobsApi } from '../api/client';
import type { JobSummary, JobStatus, JobType } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import ConfirmModal from '../components/ConfirmModal';

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

const JOB_TYPES: JobType[] = ['scene', 'performer', 'studio'];

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
  return !['complete', 'cancelled', 'resolve_failed', 'search_failed', 'download_failed', 'move_failed', 'scan_failed'].includes(status);
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
        {job.scene?.image_url ? (
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
              className="hidden group-hover/thumb:block absolute z-50 left-full top-0 ml-2 w-[21rem] h-[13.5rem] rounded-lg object-cover shadow-xl border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 pointer-events-none"
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
// Filter bar
// ---------------------------------------------------------------------------

interface Filters {
  selectedStatuses: JobStatus[];
  type: JobType | '';
  search: string;
}

interface FilterBarProps {
  filters: Filters;
  onChange: (filters: Filters) => void;
}

const FilterBar: React.FC<FilterBarProps> = ({ filters, onChange }) => {
  const [showStatusDropdown, setShowStatusDropdown] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowStatusDropdown(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const toggleStatus = (status: JobStatus) => {
    const current = filters.selectedStatuses;
    const next = current.includes(status)
      ? current.filter((s) => s !== status)
      : [...current, status];
    onChange({ ...filters, selectedStatuses: next });
  };

  const statusLabel =
    filters.selectedStatuses.length === 0
      ? 'All statuses'
      : filters.selectedStatuses.length === 1
      ? filters.selectedStatuses[0].replace(/_/g, ' ')
      : `${filters.selectedStatuses.length} statuses`;

  return (
    <div className="flex flex-wrap items-center gap-3 mb-4">
      {/* Status multi-select */}
      <div className="relative" ref={dropdownRef}>
        <button
          type="button"
          onClick={() => setShowStatusDropdown((v) => !v)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 hover:bg-gray-50 dark:hover:bg-gray-700 transition min-w-[130px] justify-between"
        >
          <span className="capitalize truncate">{statusLabel}</span>
          <svg className="w-4 h-4 text-gray-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {showStatusDropdown && (
          <div className="absolute z-20 mt-1 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg min-w-[180px] py-1 max-h-72 overflow-y-auto">
            <button
              type="button"
              onClick={() => onChange({ ...filters, selectedStatuses: [] })}
              className="w-full text-left px-3 py-1.5 text-xs font-medium text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700"
            >
              Clear all
            </button>
            <div className="border-t border-gray-100 dark:border-gray-700 my-1" />
            {ALL_STATUSES.map((s) => (
              <label
                key={s}
                className="flex items-center gap-2 px-3 py-1.5 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
              >
                <input
                  type="checkbox"
                  checked={filters.selectedStatuses.includes(s)}
                  onChange={() => toggleStatus(s)}
                  className="rounded text-blue-600"
                />
                <span className="capitalize">{s.replace(/_/g, ' ')}</span>
              </label>
            ))}
          </div>
        )}
      </div>

      {/* Type select */}
      <select
        value={filters.type}
        onChange={(e) => onChange({ ...filters, type: e.target.value as JobType | '' })}
        className="px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 hover:bg-gray-50 dark:hover:bg-gray-700 transition"
      >
        <option value="">All types</option>
        {JOB_TYPES.map((t) => (
          <option key={t} value={t} className="capitalize">
            {t}
          </option>
        ))}
      </select>

      {/* Text search */}
      <div className="relative flex-1 min-w-[200px] max-w-sm">
        <svg
          className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          type="text"
          value={filters.search}
          onChange={(e) => onChange({ ...filters, search: e.target.value })}
          placeholder="Search titles…"
          className="w-full pl-8 pr-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        />
        {filters.search && (
          <button
            type="button"
            onClick={() => onChange({ ...filters, search: '' })}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          >
            ✕
          </button>
        )}
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// Queue page
// ---------------------------------------------------------------------------

export default function Queue() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  const [filters, setFilters] = useState<Filters>(() => {
    const statusParam = searchParams.get('status');
    return {
      selectedStatuses: statusParam ? statusParam.split(',').filter(Boolean) as JobStatus[] : [],
      type: (searchParams.get('type') ?? '') as JobType | '',
      search: searchParams.get('search') ?? '',
    };
  });

  // Sync filters to URL search params
  useEffect(() => {
    const params = new URLSearchParams();
    if (filters.selectedStatuses.length > 0) {
      params.set('status', filters.selectedStatuses.join(','));
    }
    if (filters.type) {
      params.set('type', filters.type);
    }
    if (filters.search) {
      params.set('search', filters.search);
    }
    setSearchParams(params, { replace: true });
  }, [filters, setSearchParams]);

  const [debouncedSearch, setDebouncedSearch] = useState('');
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(filters.search), 300);
    return () => clearTimeout(timer);
  }, [filters.search]);

  const [cancelTarget, setCancelTarget] = useState<JobSummary | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const queryKey = [
    'jobs',
    filters.selectedStatuses.join(','),
    filters.type,
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
      if (filters.selectedStatuses.length > 0) {
        params.status = filters.selectedStatuses.join(',');
      }
      if (filters.type) {
        params.type = filters.type as JobType;
      }
      if (debouncedSearch) {
        (params as Record<string, unknown>)['search'] = debouncedSearch;
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

  return (
    <div className="flex flex-col h-full">
      <h1 className="text-xl font-bold text-gray-900 dark:text-gray-100 mb-4">Queue</h1>

      <FilterBar filters={filters} onChange={setFilters} />

      <div className="flex-1 overflow-auto min-h-0 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-xl">
        <table className="w-full text-left border-collapse">
          <thead className="sticky top-0 bg-white dark:bg-gray-900 z-10 border-b border-gray-200 dark:border-gray-700">
            <tr>
              <th className="px-3 py-2.5 w-16" />
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">Title</th>
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">Studio</th>
              <th className="px-3 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">Status</th>
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
                  {(filters.selectedStatuses.length > 0 || filters.type || filters.search) && (
                    <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">Try adjusting your filters.</p>
                  )}
                </td>
              </tr>
            )}

            {jobs.map((job) => (
              <JobRow
                key={job.id}
                job={job}
                statusFilter={filters.selectedStatuses.length > 0 ? filters.selectedStatuses.join(',') : undefined}
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
