import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import { batchesApi, type BatchJob } from '../api/client';

export default function Batches() {
  const navigate = useNavigate();

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['batches'],
    queryFn: () => batchesApi.list(),
    refetchInterval: 5000,
  });

  const batches: BatchJob[] = data?.batches ?? [];

  const pendingBatches = batches.filter(
    (b) => b.pending_count > 0 && !b.confirmed,
  );

  if (isLoading) {
    return (
      <div className="p-6">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 mb-4">Batches</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading batches…</p>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="p-6">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 mb-4">Batches</h1>
        <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-4 text-sm text-red-700 dark:text-red-400">
          Failed to load batches:{' '}
          {error instanceof Error ? error.message : 'Unknown error'}
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 mb-4">Batches</h1>

      {/* Pending confirmation banner */}
      {pendingBatches.length > 0 && (
        <div className="mb-4 rounded-lg bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-700 px-4 py-3 flex flex-col gap-1">
          <p className="text-sm font-medium text-yellow-800 dark:text-yellow-300">
            {pendingBatches.length} batch
            {pendingBatches.length !== 1 ? 'es' : ''} pending confirmation
          </p>
          <ul className="list-disc list-inside space-y-0.5">
            {pendingBatches.map((b) => (
              <li key={b.id} className="text-sm text-yellow-700 dark:text-yellow-400">
                <Link
                  to={`/batches/${b.id}`}
                  className="underline hover:text-yellow-900 dark:hover:text-yellow-200"
                >
                  {b.entity_name ?? b.stashdb_entity_id}
                </Link>{' '}
                — {b.pending_count} scene
                {b.pending_count !== 1 ? 's' : ''} waiting
              </li>
            ))}
          </ul>
        </div>
      )}

      {batches.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">No batches found.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800">
              <tr>
                <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                  Type
                </th>
                <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                  Entity
                </th>
                <th className="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-400">
                  Total Scenes
                </th>
                <th className="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-400">
                  Enqueued
                </th>
                <th className="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-400">
                  Pending
                </th>
                <th className="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-400">
                  Duplicates
                </th>
                <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-400">
                  Confirmed
                </th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 bg-white dark:bg-gray-900">
              {batches.map((batch) => (
                <tr
                  key={batch.id}
                  className="cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
                  onClick={() => navigate(`/batches/${batch.id}`)}
                >
                  <td className="px-4 py-3 capitalize text-gray-700 dark:text-gray-300">
                    {batch.type}
                  </td>
                  <td className="px-4 py-3 text-gray-900 dark:text-gray-100 font-medium">
                    {batch.entity_name ?? (
                      <span className="text-gray-400 dark:text-gray-500 font-normal text-xs">
                        {batch.stashdb_entity_id}
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-700 dark:text-gray-300">
                    {batch.total_scene_count ?? '—'}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-700 dark:text-gray-300">
                    {batch.enqueued_count}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {batch.pending_count > 0 ? (
                      <span className="text-yellow-700 dark:text-yellow-400 font-medium">
                        {batch.pending_count}
                      </span>
                    ) : (
                      <span className="text-gray-700 dark:text-gray-300">0</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-700 dark:text-gray-300">
                    {batch.duplicate_count}
                  </td>
                  <td className="px-4 py-3">
                    {batch.confirmed ? (
                      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300">
                        Yes
                      </span>
                    ) : (
                      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300">
                        No
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Link
                      to={`/batches/${batch.id}`}
                      className="text-blue-600 dark:text-blue-400 hover:underline text-xs"
                      onClick={(e) => e.stopPropagation()}
                    >
                      View
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
