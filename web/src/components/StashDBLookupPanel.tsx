import type { StashDBSceneCandidate } from '../api/client';

export interface LookupQueueEntry {
  status: string;
  jobId?: string;
}

interface StashDBLookupPanelProps {
  candidates: StashDBSceneCandidate[];
  loading: boolean;
  queueStatus: Record<string, LookupQueueEntry | 'error'>;
  safeMode: boolean;
  onQueue: (sceneId: string) => void;
  onClose: () => void;
}

export default function StashDBLookupPanel({ candidates, loading, queueStatus, safeMode, onQueue, onClose }: StashDBLookupPanelProps) {
  return (
    <div className="mt-2 border border-blue-200 dark:border-blue-800 rounded-lg bg-blue-50 dark:bg-blue-950/30 p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-semibold text-blue-700 dark:text-blue-400">StashDB matches</span>
        <button
          onClick={onClose}
          className="text-xs text-gray-400 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
        >
          ✕ close
        </button>
      </div>

      {loading && (
        <p className="text-xs text-gray-500 dark:text-gray-400 italic py-2">Searching StashDB…</p>
      )}

      {!loading && candidates.length === 0 && (
        <p className="text-xs text-gray-500 dark:text-gray-400 italic py-2">No StashDB matches found.</p>
      )}

      {!loading && candidates.length > 0 && (
        <div className="flex flex-col gap-2">
          {candidates.map((scene) => {
            const qs = queueStatus[scene.id];
            const queued = qs && qs !== 'error';
            const errored = qs === 'error';
            const label = queued
              ? (qs as LookupQueueEntry).status === 'already_stashed' ? 'Already in Stash' : '✓ Queued'
              : errored ? 'Error' : 'Queue & Download';
            return (
              <div key={scene.id} className="flex items-start gap-3 bg-white dark:bg-gray-900 rounded-md border border-gray-200 dark:border-gray-700 p-2">
                {!safeMode && scene.image_url && (
                  <img
                    src={scene.image_url}
                    alt={scene.title}
                    className="w-16 h-10 object-cover rounded flex-shrink-0"
                    loading="lazy"
                  />
                )}
                <div className="flex-1 min-w-0">
                  <p className="text-xs font-medium text-gray-900 dark:text-gray-100 truncate">{scene.title}</p>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {[scene.studio_name, scene.date].filter(Boolean).join(' · ')}
                  </p>
                  {scene.performers.length > 0 && (
                    <p className="text-xs text-gray-400 dark:text-gray-500 truncate">
                      {scene.performers.join(', ')}
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-1 flex-shrink-0">
                  <a
                    href={scene.stashdb_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
                  >
                    ↗
                  </a>
                  <button
                    onClick={() => !queued && !errored && onQueue(scene.id)}
                    disabled={queued || errored}
                    className={`px-2 py-1 text-xs font-medium rounded transition ${
                      queued
                        ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 cursor-default'
                        : errored
                        ? 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 cursor-default'
                        : 'bg-blue-600 text-white hover:bg-blue-700'
                    }`}
                  >
                    {label}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
