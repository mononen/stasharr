import { useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { batchesApi, jobsApi, type JobSummary } from '../api/client';
import StatusBadge from '../components/StatusBadge';

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

  const [actionError, setActionError] = useState<string | null>(null);
  const [loadingNext, setLoadingNext] = useState(false);
  const [loadingAutoStart, setLoadingAutoStart] = useState(false);
  const [showThumbnails, setShowThumbnails] = useState(true);

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
      await batchesApi.approve(batchId, { all: true });
      await invalidate();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to approve all');
    }
  }

  async function handleDenyAll() {
    setActionError(null);
    try {
      await batchesApi.deny(batchId, { all: true });
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
          <div className="flex items-center gap-2">
            {hasPendingApproval && (
              <button
                onClick={handleAutoStart}
                disabled={loadingAutoStart}
                className="px-3 py-1.5 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
              >
                {loadingAutoStart ? 'Starting…' : `Start all ${pendingJobs.length} now`}
              </button>
            )}
          </div>
        </div>

        <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-5 text-sm">
          <div>
            <dt className="text-gray-500 dark:text-gray-400">Created</dt>
            <dd className="text-gray-900 dark:text-gray-100 font-medium">
              {formatDate(batch.created_at)}
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
            <button
              onClick={() => setShowThumbnails((v) => !v)}
              className="px-2 py-1 text-xs font-medium rounded border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 transition"
              title={showThumbnails ? 'Hide performer thumbnails' : 'Show performer thumbnails'}
            >
              {showThumbnails ? 'Hide photos' : 'Show photos'}
            </button>
          </div>

          {/* Bulk actions — only shown when there are pending_approval jobs */}
          {hasPendingApproval && (
            <div className="flex items-center gap-2">
              <button
                onClick={handleApproveAll}
                className="px-3 py-1.5 text-sm font-medium text-white bg-green-600 rounded-lg hover:bg-green-700 transition"
              >
                Approve all ({pendingJobs.length})
              </button>
              <button
                onClick={handleDenyAll}
                className="px-3 py-1.5 text-sm font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 transition"
              >
                Deny all ({pendingJobs.length})
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
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Title
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Performers
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Studio
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Status
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                    Created
                  </th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800 bg-white dark:bg-gray-900">
                {jobs.map((job) => (
                  <tr
                    key={job.id}
                    className={`transition-colors ${
                      job.status === 'pending_approval'
                        ? 'bg-white dark:bg-gray-900'
                        : 'cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50'
                    }`}
                    onClick={() => {
                      if (job.status !== 'pending_approval') navigate(`/queue/${job.id}`);
                    }}
                  >
                    <td className="px-4 py-3 text-gray-900 dark:text-gray-100">
                      {job.scene?.title ?? (
                        <span className="text-gray-400 dark:text-gray-500 text-xs">
                          {job.stashdb_url}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <PerformerCell
                        performers={job.scene?.performer_infos}
                        fallbackNames={job.scene?.performers}
                        showThumbnails={showThumbnails}
                      />
                    </td>
                    <td className="px-4 py-3 text-gray-700 dark:text-gray-300">
                      {job.scene?.studio_name ?? '—'}
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge status={job.status} />
                    </td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 whitespace-nowrap">
                      {formatDate(job.created_at)}
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

        {/* Load next 20 */}
        {canLoadMore && (
          <div className="mt-4 flex items-center gap-3">
            <button
              onClick={handleAddNext}
              disabled={loadingNext}
              className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50 transition"
            >
              {loadingNext ? 'Loading…' : 'Load next 20'}
            </button>
            {batch.pending_count > 0 && (
              <span className="text-sm text-gray-500 dark:text-gray-400">
                ~{batch.pending_count} remaining
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// --- Performer cell with optional thumbnails ---

interface PerformerCellProps {
  performers?: { name: string; image_url?: string }[];
  fallbackNames?: string[];
  showThumbnails: boolean;
}

function PerformerCell({ performers, fallbackNames, showThumbnails }: PerformerCellProps) {
  const infos: { name: string; image_url?: string }[] =
    performers ?? fallbackNames?.map((n) => ({ name: n })) ?? [];
  if (infos.length === 0) {
    return <span className="text-gray-400 dark:text-gray-500">—</span>;
  }

  if (showThumbnails) {
    return (
      <div className="flex flex-wrap gap-2">
        {infos.map((p, i) => (
          <div key={i} className="flex items-center gap-1.5">
            {p.image_url ? (
              <img
                src={p.image_url}
                alt={p.name}
                className="w-7 h-7 rounded-full object-cover flex-shrink-0 bg-gray-200 dark:bg-gray-700"
                loading="lazy"
              />
            ) : (
              <span className="w-7 h-7 rounded-full bg-gray-200 dark:bg-gray-700 flex items-center justify-center text-[10px] font-medium text-gray-500 dark:text-gray-400 flex-shrink-0">
                {p.name.charAt(0).toUpperCase()}
              </span>
            )}
            <span className="text-xs text-gray-700 dark:text-gray-300 whitespace-nowrap">
              {p.name}
            </span>
          </div>
        ))}
      </div>
    );
  }

  return (
    <span className="text-gray-700 dark:text-gray-300 text-xs">
      {infos.map((p) => p.name).join(', ')}
    </span>
  );
}

// Small type icon component
function TypeIcon({ type }: { type: string }) {
  if (type === 'performer') {
    return (
      <span
        title="Performer"
        className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 text-xs font-bold"
      >
        P
      </span>
    );
  }
  if (type === 'studio') {
    return (
      <span
        title="Studio"
        className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 text-xs font-bold"
      >
        S
      </span>
    );
  }
  return (
    <span
      title="Scene"
      className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-xs font-bold"
    >
      {type.charAt(0).toUpperCase()}
    </span>
  );
}

// Keep TypeIcon exported for potential reuse elsewhere
export { TypeIcon };
