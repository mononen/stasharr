import { useEffect, useMemo, useState } from 'react';
import { jobsApi } from '../api/client';
import type { SceneDetail } from '../api/client';

interface CustomSearchPanelProps {
  jobId: string;
  scene: SceneDetail;
  onSearchComplete: () => void;
}

/** Convert "Net Girl" → "NetGirl", "Brazzers" → "Brazzers" */
function toCamelCase(s: string): string {
  return s
    .split(/\s+/)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join('');
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
  const [query, setQuery] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Rebuild query whenever checkboxes/performers change, overwriting any manual edits.
  const builtQuery = useMemo(() => {
    const tokens: string[] = [];
    if (includeTitle && scene.title) tokens.push(scene.title);
    if (includeStudio && scene.studio_name) tokens.push(toCamelCase(scene.studio_name));
    nonMalePerformers
      .filter((p) => selectedPerformers.has(p.name))
      .forEach((p) => tokens.push(p.name));
    if (includeDate && scene.release_date) tokens.push(scene.release_date);
    return tokens.join(' ');
  }, [includeTitle, includeStudio, includeDate, selectedPerformers, scene, nonMalePerformers]);

  useEffect(() => {
    setQuery(builtQuery);
  }, [builtQuery]);

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
    if (!query.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await jobsApi.customSearch(jobId, query);
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
            <span className="text-gray-400 dark:text-gray-500">"{toCamelCase(scene.studio_name)}"</span>
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

      {/* Editable query */}
      <div className="mb-4">
        <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Query</p>
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') handleSearch(); }}
          placeholder="Select fields above or type a custom query"
          className="w-full bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded px-3 py-2 text-sm font-mono text-gray-800 dark:text-gray-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        />
      </div>

      {/* Actions */}
      <div className="flex items-center gap-3">
        <button
          onClick={handleSearch}
          disabled={busy || !query.trim()}
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
