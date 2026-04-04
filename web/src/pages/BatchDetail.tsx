import { useState, useMemo, useRef, useEffect } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { batchesApi, jobsApi, type JobSummary } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import ConfirmModal from '../components/ConfirmModal';
import useStore from '../hooks/useStore';

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export default function BatchDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const safeMode = useStore((s) => s.safeMode);

  const [actionError, setActionError] = useState<string | null>(null);
  const [loadingNext, setLoadingNext] = useState(false);
  const [loadingAutoStart, setLoadingAutoStart] = useState(false);
  const [loadingCheckLatest, setLoadingCheckLatest] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  const [performerFilter, setPerformerFilter] = useState<string>('');
  const [performerSearch, setPerformerSearch] = useState('');
  const [performerDropdownOpen, setPerformerDropdownOpen] = useState(false);
  const performerDropdownRef = useRef<HTMLDivElement>(null);

  const [tagFilter, setTagFilter] = useState<string>('');
  const [tagSearch, setTagSearch] = useState('');
  const [tagDropdownOpen, setTagDropdownOpen] = useState(false);
  const tagDropdownRef = useRef<HTMLDivElement>(null);

  const [showMissing, setShowMissing] = useState(false);

  const batchId = id ?? '';

  const {
    data: batch,
    isLoading: batchLoading,
    isError: batchError,
    error: batchFetchError,
  } = useQuery({
    queryKey: ['batches', batchId],
    queryFn: () => batchesApi.get(batchId),
    enabled: Boolean(batchId),
    refetchInterval: 5000,
  });

  const {
    data: jobsData,
    isLoading: jobsLoading,
    isError: jobsError,
  } = useQuery({
    queryKey: ['jobs', { batch_id: batchId }],
    queryFn: () => jobsApi.list({ batch_id: batchId }),
    enabled: Boolean(batchId),
    refetchInterval: 5000,
  });

  const jobs: JobSummary[] = jobsData?.jobs ?? [];
  const pendingJobs = jobs.filter((j) => j.status === 'pending_approval');

  const allPerformers = useMemo(() => {
    const set = new Set<string>();
    for (const j of jobs) {
      for (const p of j.scene?.performers ?? []) set.add(p);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [jobs]);

  const allTags = useMemo(() => {
    const set = new Set<string>();
    for (const j of jobs) {
      for (const t of j.scene?.tags ?? []) set.add(t);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [jobs]);

  const missingCount = useMemo(
    () => jobs.filter((j) => j.status === 'search_failed').length,
    [jobs],
  );

  const filteredJobs = useMemo(() => {
    let result = jobs;
    if (showMissing) {
      result = result.filter((j) => j.status === 'search_failed');
    }
    if (performerFilter) {
      result = result.filter((j) => j.scene?.performers?.includes(performerFilter));
    }
    if (tagFilter) {
      result = result.filter((j) => j.scene?.tags?.includes(tagFilter));
    }
    return [...result].sort((a, b) => {
      const da = a.scene?.release_date ?? '';
      const db = b.scene?.release_date ?? '';
      if (da > db) return -1;
      if (da < db) return 1;
      return 0;
    });
  }, [jobs, showMissing, performerFilter, tagFilter]);

  const hasActiveFilter = Boolean(performerFilter || tagFilter);

  // Close performer dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (performerDropdownRef.current && !performerDropdownRef.current.contains(e.target as Node)) {
        setPerformerDropdownOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  // Close tag dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (tagDropdownRef.current && !tagDropdownRef.current.contains(e.target as Node)) {
        setTagDropdownOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  async function invalidate() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['batches', batchId] }),
      queryClient.invalidateQueries({ queryKey: ['batches'] }),
      queryClient.invalidateQueries({ queryKey: ['jobs', { batch_id: batchId }] }),
    ]);
  }

  async function handleApprove(sceneIds: string[]) {
    setActionError(null);
    try {
      await batchesApi.approve(batchId, { scene_ids: sceneIds });
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to approve');
    }
  }

  async function handleDeny(sceneIds: string[]) {
    setActionError(null);
    try {
      await batchesApi.deny(batchId, { scene_ids: sceneIds });
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to deny');
    }
  }

  async function handleApproveAll() {
    setActionError(null);
    try {
      if (hasActiveFilter) {
        const ids = filteredJobs
          .filter((j) => j.status === 'pending_approval')
          .map((j) => j.id);
        if (ids.length > 0) {
          await batchesApi.approve(batchId, { scene_ids: ids });
        }
      } else {
        await batchesApi.approve(batchId, { all: true });
      }
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to approve all');
    }
  }

  async function handleDenyAll() {
    setActionError(null);
    try {
      if (hasActiveFilter) {
        const ids = filteredJobs
          .filter((j) => j.status === 'pending_approval')
          .map((j) => j.id);
        if (ids.length > 0) {
          await batchesApi.deny(batchId, { scene_ids: ids });
        }
      } else {
        await batchesApi.deny(batchId, { all: true });
      }
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to deny all');
    }
  }

  async function handleAddNext() {
    setLoadingNext(true);
    setActionError(null);
    try {
      await batchesApi.next(batchId);
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to load next scenes');
    } finally {
      setLoadingNext(false);
    }
  }

  async function handleAutoStart() {
    setLoadingAutoStart(true);
    setActionError(null);
    try {
      await batchesApi.autoStart(batchId);
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to auto-start');
    } finally {
      setLoadingAutoStart(false);
    }
  }

  async function handleCheckLatest() {
    setLoadingCheckLatest(true);
    setActionError(null);
    try {
      await batchesApi.checkLatest(batchId);
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to check for latest scenes');
    } finally {
      setLoadingCheckLatest(false);
    }
  }

  async function handleDelete() {
    setShowDeleteConfirm(false);
    try {
      await batchesApi.delete(batchId);
      navigate('/batches');
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to delete batch');
    }
  }

  if (batchLoading) {
    return (
      <div className="p-6">
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading batch…</p>
      </div>
    );
  }

  if (batchError || !batch) {
    return (
      <div className="p-6">
        <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-4 text-sm text-red-700 dark:text-red-400">
          Failed to load batch:{' '}
          {batchFetchError instanceof Error ? batchFetchError.message : 'Unknown error'}
        </div>
        <Link
          to="/batches"
          className="mt-4 inline-block text-sm text-blue-600 dark:text-blue-400 hover:underline"
        >
          ← Back to Batches
        </Link>
      </div>
    );
  }

  const hasPendingApproval = pendingJobs.length > 0;
  const canLoadMore = !batch.confirmed;

  return (
    <div className="p-6 space-y-6">
      {/* Breadcrumb */}
      <div>
        <Link to="/batches" className="text-sm text-blue-600 dark:text-blue-400 hover:underline">
          ← Batches
        </Link>
      </div>

      {/* Batch metadata header */}
      <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-5">
        <div className="flex items-start justify-between gap-4 flex-wrap">
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
              {batch.entity_name ?? batch.stashdb_entity_id}
            </h1>
            <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400 capitalize">
              {batch.type}
            </p>
          </div>
          <div className="flex items-center gap-2 flex-wrap">
            <button
              onClick={handleCheckLatest}
              disabled={loadingCheckLatest}
              className="px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50 transition"
            >
              {loadingCheckLatest ? 'Checking…' : 'Check for latest'}
            </button>
            {hasPendingApproval && (
              <button
                onClick={handleAutoStart}
                disabled={loadingAutoStart}
                className="px-3 py-1.5 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
              >
                {loadingAutoStart ? 'Starting…' : `Start all ${pendingJobs.length} now`}
              </button>
            )}
            <button
              onClick={() => setShowDeleteConfirm(true)}
              title="Delete batch"
              className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 transition"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </button>
          </div>
        </div>

        <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3 lg:grid-cols-6 text-sm">
          <div>
            <dt className="text-gray-500 dark:text-gray-400">Created</dt>
            <dd className="text-gray-900 dark:text-gray-100 font-medium">
              {formatDate(batch.created_at)}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500 dark:text-gray-400">Last checked</dt>
            <dd className="text-gray-900 dark:text-gray-100 font-medium">
              {batch.last_checked_at ? formatDate(batch.last_checked_at) : '—'}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500 dark:text-gray-400">Total scenes</dt>
            <dd className="text-gray-900 dark:text-gray-100 font-medium">
              {batch.total_scene_count ?? '—'}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500 dark:text-gray-400">Loaded</dt>
            <dd className="text-gray-900 dark:text-gray-100 font-medium">
              {batch.enqueued_count}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500 dark:text-gray-400">Remaining</dt>
            <dd
              className={`font-medium ${
                batch.pending_count > 0 ? 'text-yellow-700 dark:text-yellow-400' : 'text-gray-900 dark:text-gray-100'
              }`}
            >
              {batch.confirmed ? '—' : `~${batch.pending_count}`}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500 dark:text-gray-400">Duplicates</dt>
            <dd className="text-gray-900 dark:text-gray-100 font-medium">
              {batch.duplicate_count}
            </dd>
          </div>
        </dl>
      </div>

      {/* Error banner */}
      {actionError && (
        <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 px-4 py-3 text-sm text-red-700 dark:text-red-400">
          {actionError}
        </div>
      )}

      {/* Jobs table */}
      <div>
        <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
          <div className="flex items-center gap-3">
            <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100">Jobs</h2>
            {missingCount > 0 && (
              <button
                type="button"
                onClick={() => setShowMissing((v) => !v)}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium transition ${
                  showMissing
                    ? 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400 ring-1 ring-orange-400 dark:ring-orange-600'
                    : 'bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 hover:text-orange-600 dark:hover:text-orange-400'
                }`}
              >
                <span className="w-1.5 h-1.5 rounded-full bg-orange-500" />
                {missingCount} missing
              </button>
            )}
          </div>

          {/* Bulk actions — only shown when there are pending_approval jobs in the current view */}
          {filteredJobs.some((j) => j.status === 'pending_approval') && (
            <div className="flex items-center gap-2">
              <button
                onClick={handleApproveAll}
                className="px-3 py-1.5 text-sm font-medium text-white bg-green-600 rounded-lg hover:bg-green-700 transition"
              >
                Approve {hasActiveFilter ? 'filtered' : 'all'} ({filteredJobs.filter(j => j.status === 'pending_approval').length})
              </button>
              <button
                onClick={handleDenyAll}
                className="px-3 py-1.5 text-sm font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 transition"
              >
                Deny {hasActiveFilter ? 'filtered' : 'all'} ({filteredJobs.filter(j => j.status === 'pending_approval').length})
              </button>
            </div>
          )}
        </div>

        {jobsLoading ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">Loading jobs…</p>
        ) : jobsError ? (
          <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-4 text-sm text-red-700 dark:text-red-400">
            Failed to load jobs for this batch.
          </div>
        ) : jobs.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">No jobs in this batch yet.</p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  {!safeMode && (
                    <th className="px-4 py-3 w-20" />
                  )}
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Title
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400 relative">
                    <div ref={performerDropdownRef} className="inline-block">
                      <button
                        type="button"
                        onClick={() => {
                          setPerformerDropdownOpen((v) => !v);
                          setPerformerSearch('');
                        }}
                        className={`inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-gray-200 transition ${
                          performerFilter ? 'text-blue-600 dark:text-blue-400' : ''
                        }`}
                      >
                        Performers
                        {performerFilter && (
                          <span className="text-xs font-normal">({performerFilter})</span>
                        )}
                        <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                        </svg>
                      </button>
                      {performerDropdownOpen && (
                        <div className="absolute z-50 mt-1 left-0 w-56 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg">
                          <div className="p-2">
                            <input
                              type="text"
                              autoFocus
                              value={performerSearch}
                              onChange={(e) => setPerformerSearch(e.target.value)}
                              placeholder="Search performers…"
                              className="w-full px-2 py-1.5 text-xs border border-gray-200 dark:border-gray-600 rounded bg-gray-50 dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
                            />
                          </div>
                          <ul className="max-h-48 overflow-y-auto text-xs font-normal">
                            <li>
                              <button
                                type="button"
                                onClick={() => {
                                  setPerformerFilter('');
                                  setPerformerDropdownOpen(false);
                                }}
                                className={`w-full text-left px-3 py-1.5 hover:bg-gray-100 dark:hover:bg-gray-700 ${
                                  !performerFilter ? 'text-blue-600 dark:text-blue-400 font-medium' : 'text-gray-700 dark:text-gray-300'
                                }`}
                              >
                                All performers
                              </button>
                            </li>
                            {allPerformers
                              .filter((p) => p.toLowerCase().includes(performerSearch.toLowerCase()))
                              .map((p) => (
                                <li key={p}>
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setPerformerFilter(p);
                                      setPerformerDropdownOpen(false);
                                    }}
                                    className={`w-full text-left px-3 py-1.5 hover:bg-gray-100 dark:hover:bg-gray-700 ${
                                      performerFilter === p ? 'text-blue-600 dark:text-blue-400 font-medium' : 'text-gray-700 dark:text-gray-300'
                                    }`}
                                  >
                                    {p}
                                  </button>
                                </li>
                              ))}
                          </ul>
                        </div>
                      )}
                    </div>
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400 relative">
                    <div ref={tagDropdownRef} className="inline-block">
                      <button
                        type="button"
                        onClick={() => {
                          setTagDropdownOpen((v) => !v);
                          setTagSearch('');
                        }}
                        className={`inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-gray-200 transition ${
                          tagFilter ? 'text-blue-600 dark:text-blue-400' : ''
                        }`}
                      >
                        Tags
                        {tagFilter && (
                          <span className="text-xs font-normal">({tagFilter})</span>
                        )}
                        <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                        </svg>
                      </button>
                      {tagDropdownOpen && (
                        <div className="absolute z-50 mt-1 left-0 w-56 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg">
                          <div className="p-2">
                            <input
                              type="text"
                              autoFocus
                              value={tagSearch}
                              onChange={(e) => setTagSearch(e.target.value)}
                              placeholder="Search tags…"
                              className="w-full px-2 py-1.5 text-xs border border-gray-200 dark:border-gray-600 rounded bg-gray-50 dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
                            />
                          </div>
                          <ul className="max-h-48 overflow-y-auto text-xs font-normal">
                            <li>
                              <button
                                type="button"
                                onClick={() => {
                                  setTagFilter('');
                                  setTagDropdownOpen(false);
                                }}
                                className={`w-full text-left px-3 py-1.5 hover:bg-gray-100 dark:hover:bg-gray-700 ${
                                  !tagFilter ? 'text-blue-600 dark:text-blue-400 font-medium' : 'text-gray-700 dark:text-gray-300'
                                }`}
                              >
                                All tags
                              </button>
                            </li>
                            {allTags
                              .filter((t) => t.toLowerCase().includes(tagSearch.toLowerCase()))
                              .map((t) => (
                                <li key={t}>
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setTagFilter(t);
                                      setTagDropdownOpen(false);
                                    }}
                                    className={`w-full text-left px-3 py-1.5 hover:bg-gray-100 dark:hover:bg-gray-700 ${
                                      tagFilter === t ? 'text-blue-600 dark:text-blue-400 font-medium' : 'text-gray-700 dark:text-gray-300'
                                    }`}
                                  >
                                    {t}
                                  </button>
                                </li>
                              ))}
                          </ul>
                        </div>
                      )}
                    </div>
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Studio
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Date
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Status
                  </th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800 bg-white dark:bg-gray-900">
                {filteredJobs.map((job) => (
                  <tr
                    key={job.id}
                    className={`transition-colors hover:bg-gray-50 dark:hover:bg-gray-800/50 ${
                      job.status === 'pending_approval'
                        ? 'bg-white dark:bg-gray-900'
                        : 'cursor-pointer'
                    }`}
                    onClick={() => {
                      if (job.status !== 'pending_approval') navigate(`/queue/${job.id}`);
                    }}
                  >
                    {!safeMode && (
                      <td className="px-4 py-2">
                        {job.scene?.image_url ? (
                          <div className="relative group/thumb">
                            <img
                              src={job.scene.image_url}
                              alt={job.scene?.title ?? ''}
                              className="w-16 h-10 rounded object-cover bg-gray-200 dark:bg-gray-700"
                              loading="lazy"
                            />
                            <img
                              src={job.scene.image_url}
                              alt={job.scene?.title ?? ''}
                              className="hidden group-hover/thumb:block absolute z-50 left-full top-0 ml-2 max-w-sm rounded-lg shadow-xl border border-gray-200 dark:border-gray-700 pointer-events-none"
                            />
                          </div>
                        ) : (
                          <span className="block w-16 h-10 rounded bg-gray-200 dark:bg-gray-700" />
                        )}
                      </td>
                    )}
                    <td className="px-4 py-3 text-gray-900 dark:text-gray-100">
                      {job.scene?.title ?? (
                        <span className="text-gray-400 dark:text-gray-500 text-xs">
                          {job.stashdb_url}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-gray-700 dark:text-gray-300 text-xs">
                      {job.scene?.performers?.join(', ') || '—'}
                    </td>
                    <td className="px-4 py-3 text-gray-700 dark:text-gray-300 text-xs">
                      {job.scene?.tags?.join(', ') || '—'}
                    </td>
                    <td className="px-4 py-3 text-gray-700 dark:text-gray-300">
                      {job.scene?.studio_name ?? '—'}
                    </td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 whitespace-nowrap text-xs">
                      {job.scene?.release_date ?? '—'}
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge status={job.status} />
                    </td>
                    <td className="px-4 py-3">
                      {job.status === 'pending_approval' && (
                        <div className="flex items-center gap-1.5 justify-end">
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleApprove([job.id]);
                            }}
                            title="Approve"
                            className="p-1 rounded text-green-600 hover:bg-green-50 dark:hover:bg-green-900/20 transition"
                          >
                            ✓
                          </button>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleDeny([job.id]);
                            }}
                            title="Deny"
                            className="p-1 rounded text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 transition"
                          >
                            ✕
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Load next 50 / Check for latest */}
        <div className="mt-4 flex items-center gap-3 flex-wrap">
          {canLoadMore && (
            <>
              <button
                onClick={handleAddNext}
                disabled={loadingNext}
                className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50 transition"
              >
                {loadingNext ? 'Loading…' : 'Load next 50'}
              </button>
              {batch.pending_count > 0 && (
                <span className="text-sm text-gray-500 dark:text-gray-400">
                  ~{batch.pending_count} remaining
                </span>
              )}
            </>
          )}
        </div>
      </div>

      <ConfirmModal
        isOpen={showDeleteConfirm}
        title="Delete batch"
        message={`Delete "${batch.entity_name ?? batch.stashdb_entity_id}" and all its scenes? This cannot be undone.`}
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </div>
  );
}
