import { useMemo, useState } from 'react';
import { jobsApi } from '../api/client';
import type { SceneDetail } from '../api/client';

interface CustomSearchPanelProps {
  jobId: string;
  scene: SceneDetail;
  onSearchComplete: () => void;
}

export default function CustomSearchPanel({ jobId, scene, onSearchComplete }: CustomSearchPanelProps) {
  const nonMalePerformers = useMemo(
    () =>
      (scene.performers ?? []).filter(
        (p) => p.gender !== 'MALE' && p.gender !== 'TRANSGENDER_MALE',
      ),
    [scene.performers],
  );

  const [includeTitle, setIncludeTitle] = useState(true);
  const [includeStudio, setIncludeStudio] = useState(!!scene.studio_name);
  const [includeDate, setIncludeDate] = useState(false);
  const [selectedPerformers, setSelectedPerformers] = useState<Set<string>>(
    () => new Set(nonMalePerformers.map((p) => p.name)),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const queryPreview = useMemo(() => {
    const tokens: string[] = [];
    if (includeTitle && scene.title) tokens.push(scene.title);
    if (includeStudio && scene.studio_name) tokens.push(scene.studio_name);
    nonMalePerformers
      .filter((p) => selectedPerformers.has(p.name))
      .forEach((p) => tokens.push(p.name));
    if (includeDate && scene.release_date) tokens.push(scene.release_date);
    return tokens.join(' ');
  }, [includeTitle, includeStudio, includeDate, selectedPerformers, scene, nonMalePerformers]);

  function togglePerformer(name: string) {
    setSelectedPerformers((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }

  async function handleSearch() {
    if (!queryPreview.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await jobsApi.customSearch(jobId, queryPreview);
      onSearchComplete();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Search failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-5 mb-6">
      <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">Custom Search</h2>

      {/* Field toggles */}
      <div className="flex flex-wrap gap-3 mb-3">
        <label className="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
          <input
            type="checkbox"
            checked={includeTitle}
            onChange={(e) => setIncludeTitle(e.target.checked)}
            className="rounded border-gray-300 dark:border-gray-600 text-blue-600"
          />
          <span className="font-medium">Title</span>
          {scene.title && (
            <span className="text-gray-400 dark:text-gray-500 truncate max-w-48">"{scene.title}"</span>
          )}
        </label>

        {scene.studio_name && (
          <label className="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
            <input
              type="checkbox"
              checked={includeStudio}
              onChange={(e) => setIncludeStudio(e.target.checked)}
              className="rounded border-gray-300 dark:border-gray-600 text-blue-600"
            />
            <span className="font-medium">Studio</span>
            <span className="text-gray-400 dark:text-gray-500">"{scene.studio_name}"</span>
          </label>
        )}

        {scene.release_date && (
          <label className="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
            <input
              type="checkbox"
              checked={includeDate}
              onChange={(e) => setIncludeDate(e.target.checked)}
              className="rounded border-gray-300 dark:border-gray-600 text-blue-600"
            />
            <span className="font-medium">Date</span>
            <span className="text-gray-400 dark:text-gray-500">"{scene.release_date}"</span>
          </label>
        )}
      </div>

      {/* Performer checkboxes */}
      {nonMalePerformers.length > 0 && (
        <div className="mb-4">
          <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">Performers</p>
          <div className="flex flex-wrap gap-2">
            {nonMalePerformers.map((p) => (
              <label
                key={p.name}
                className="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300 bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded px-2 py-1 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700"
              >
                <input
                  type="checkbox"
                  checked={selectedPerformers.has(p.name)}
                  onChange={() => togglePerformer(p.name)}
                  className="rounded border-gray-300 dark:border-gray-600 text-blue-600"
                />
                {p.name}
              </label>
            ))}
          </div>
        </div>
      )}

      {/* Query preview */}
      <div className="mb-4">
        <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Query</p>
        {queryPreview ? (
          <code className="block bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded px-3 py-2 text-sm font-mono text-gray-800 dark:text-gray-200 break-all">
            {queryPreview}
          </code>
        ) : (
          <p className="text-sm text-gray-400 dark:text-gray-500 italic">
            Select at least one field above
          </p>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-3">
        <button
          onClick={handleSearch}
          disabled={busy || !queryPreview.trim()}
          className="px-4 py-1.5 text-sm font-medium bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white rounded"
        >
          {busy ? 'Searching…' : 'Search'}
        </button>
        {error && (
          <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
        )}
      </div>
    </div>
  );
}
