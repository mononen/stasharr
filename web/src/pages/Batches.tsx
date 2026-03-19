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
                  <td className="px-4 py-3">
                    <div className="text-gray-900 dark:text-gray-100 font-medium">
                      {batch.entity_name ?? (
                        <span className="text-gray-400 dark:text-gray-500 font-normal text-xs">
                          {batch.stashdb_entity_id}
                        </span>
                      )}
                    </div>
                    {batch.tag_names && batch.tag_names.length > 0 && (
                      <div className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">
                        {batch.tag_names.join(', ')}
                      </div>
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
