import { useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { batchesApi, jobsApi, type JobSummary } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import ConfirmModal from '../components/ConfirmModal';

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

  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [confirmError, setConfirmError] = useState<string | null>(null);

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
  });

  const {
    data: jobsData,
    isLoading: jobsLoading,
    isError: jobsError,
  } = useQuery({
    queryKey: ['jobs', { batch_id: batchId }],
    queryFn: () => jobsApi.list({ batch_id: batchId }),
    enabled: Boolean(batchId),
  });

  const jobs: JobSummary[] = jobsData?.jobs ?? [];

  async function handleConfirm() {
    setConfirming(true);
    setConfirmError(null);
    try {
      await batchesApi.confirm(batchId);
      await queryClient.invalidateQueries({ queryKey: ['batches', batchId] });
      await queryClient.invalidateQueries({ queryKey: ['batches'] });
      setConfirmOpen(false);
    } catch (err) {
      setConfirmError(
        err instanceof Error ? err.message : 'Failed to confirm batch',
      );
    } finally {
      setConfirming(false);
    }
  }

  if (batchLoading) {
    return (
      <div className="p-6">
        <p className="text-sm text-gray-500">Loading batch…</p>
      </div>
    );
  }

  if (batchError || !batch) {
    return (
      <div className="p-6">
        <div className="rounded-lg bg-red-50 border border-red-200 p-4 text-sm text-red-700">
          Failed to load batch:{' '}
          {batchFetchError instanceof Error
            ? batchFetchError.message
            : 'Unknown error'}
        </div>
        <Link
          to="/batches"
          className="mt-4 inline-block text-sm text-blue-600 hover:underline"
        >
          ← Back to Batches
        </Link>
      </div>
    );
  }

  const showConfirmBanner = batch.pending_count > 0 && !batch.confirmed;

  return (
    <div className="p-6 space-y-6">
      {/* Breadcrumb */}
      <div>
        <Link to="/batches" className="text-sm text-blue-600 hover:underline">
          ← Batches
        </Link>
      </div>

      {/* Batch metadata header */}
      <div className="rounded-lg border border-gray-200 bg-white p-5">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-lg font-semibold text-gray-900">
              {batch.entity_name ?? batch.stashdb_entity_id}
            </h1>
            <p className="mt-0.5 text-sm text-gray-500 capitalize">
              {batch.type}
            </p>
          </div>
          <span
            className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
              batch.confirmed
                ? 'bg-green-100 text-green-800'
                : 'bg-gray-100 text-gray-600'
            }`}
          >
            {batch.confirmed ? 'Confirmed' : 'Unconfirmed'}
          </span>
        </div>

        <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4 text-sm">
          <div>
            <dt className="text-gray-500">Created</dt>
            <dd className="text-gray-900 font-medium">
              {formatDate(batch.created_at)}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500">Total scenes</dt>
            <dd className="text-gray-900 font-medium">
              {batch.total_scene_count ?? '—'}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500">Enqueued</dt>
            <dd className="text-gray-900 font-medium">
              {batch.enqueued_count}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500">Pending</dt>
            <dd
              className={`font-medium ${
                batch.pending_count > 0 ? 'text-yellow-700' : 'text-gray-900'
              }`}
            >
              {batch.pending_count}
            </dd>
          </div>
          <div>
            <dt className="text-gray-500">Duplicates</dt>
            <dd className="text-gray-900 font-medium">
              {batch.duplicate_count}
            </dd>
          </div>
        </dl>
      </div>

      {/* Pending confirmation banner */}
      {showConfirmBanner && (
        <div className="rounded-lg bg-yellow-50 border border-yellow-300 p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div>
            <p className="text-sm font-semibold text-yellow-800">
              {batch.pending_count} scene
              {batch.pending_count !== 1 ? 's' : ''} waiting for confirmation
            </p>
            <p className="text-xs text-yellow-700 mt-0.5">
              Confirm to queue the remaining scenes for this batch.
            </p>
          </div>
          <button
            onClick={() => setConfirmOpen(true)}
            className="shrink-0 px-4 py-2 text-sm font-medium text-white bg-yellow-600 rounded-lg hover:bg-yellow-700 transition"
          >
            Confirm {batch.pending_count} pending job
            {batch.pending_count !== 1 ? 's' : ''}
          </button>
        </div>
      )}

      {/* Confirm error */}
      {confirmError && (
        <div className="rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
          {confirmError}
        </div>
      )}

      {/* Jobs table */}
      <div>
        <h2 className="text-base font-semibold text-gray-900 mb-3">Jobs</h2>

        {jobsLoading ? (
          <p className="text-sm text-gray-500">Loading jobs…</p>
        ) : jobsError ? (
          <div className="rounded-lg bg-red-50 border border-red-200 p-4 text-sm text-red-700">
            Failed to load jobs for this batch.
          </div>
        ) : jobs.length === 0 ? (
          <p className="text-sm text-gray-500">No jobs in this batch yet.</p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-gray-200">
            <table className="min-w-full divide-y divide-gray-200 text-sm">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-3 text-left font-medium text-gray-600">
                    Type
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600">
                    Title
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600">
                    Studio
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600">
                    Status
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-gray-600">
                    Created
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 bg-white">
                {jobs.map((job) => (
                  <tr
                    key={job.id}
                    className="cursor-pointer hover:bg-gray-50 transition-colors"
                    onClick={() => navigate(`/queue/${job.id}`)}
                  >
                    <td className="px-4 py-3">
                      <TypeIcon type={job.type} />
                    </td>
                    <td className="px-4 py-3 text-gray-900">
                      {job.scene?.title ?? (
                        <span className="text-gray-400 text-xs">
                          {job.stashdb_url}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-gray-700">
                      {job.scene?.studio_name ?? '—'}
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge status={job.status} />
                    </td>
                    <td className="px-4 py-3 text-gray-500">
                      {formatDate(job.created_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Confirm modal */}
      <ConfirmModal
        isOpen={confirmOpen}
        title="Confirm Batch"
        message={`Queue ${batch.pending_count} pending scene${
          batch.pending_count !== 1 ? 's' : ''
        } for "${batch.entity_name ?? batch.stashdb_entity_id}"?`}
        onConfirm={handleConfirm}
        onCancel={() => {
          if (!confirming) {
            setConfirmOpen(false);
          }
        }}
      />
    </div>
  );
}

// Small type icon component
function TypeIcon({ type }: { type: string }) {
  if (type === 'performer') {
    return (
      <span
        title="Performer"
        className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-purple-100 text-purple-700 text-xs font-bold"
      >
        P
      </span>
    );
  }
  if (type === 'studio') {
    return (
      <span
        title="Studio"
        className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-blue-100 text-blue-700 text-xs font-bold"
      >
        S
      </span>
    );
  }
  return (
    <span
      title="Scene"
      className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-gray-100 text-gray-600 text-xs font-bold"
    >
      {type.charAt(0).toUpperCase()}
    </span>
  );
}
