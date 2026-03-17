import { useMemo } from 'react';
import { useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { jobsApi } from '../api/client';
import type { SearchResult as ApiSearchResult } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import JobEventTimeline from '../components/JobEventTimeline';
import SearchResultRow from '../components/SearchResultRow';
import type { SearchResult as RowSearchResult } from '../components/SearchResultRow';
import { useJobEvents } from '../hooks/useJobEvents';

// Map API FieldScore → ScoreBreakdown's FieldScore shape
function mapBreakdown(
  breakdown: Record<string, { score: number; max: number; matched?: boolean; similarity?: number; delta_seconds?: number }>,
): Record<string, { score: number; max_score: number; matched?: string; expected?: string }> {
  const out: Record<string, { score: number; max_score: number; matched?: string; expected?: string }> = {};
  for (const [key, fs] of Object.entries(breakdown)) {
    out[key] = {
      score: fs.score,
      max_score: fs.max,
      matched: fs.matched !== undefined ? String(fs.matched) : undefined,
      expected: fs.similarity !== undefined ? `sim: ${fs.similarity.toFixed(2)}` : undefined,
    };
  }
  return out;
}

// Map API SearchResult → SearchResultRow's SearchResult shape
function mapApiResult(r: ApiSearchResult): RowSearchResult {
  return {
    id: r.id,
    title: r.release_title,
    indexer: r.indexer_name,
    size: r.size_bytes ?? 0,
    publish_date: r.publish_date ?? '',
    score: r.confidence_score,
    score_breakdown: mapBreakdown(r.score_breakdown),
  };
}

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

export default function JobDetail() {
  const { id } = useParams<{ id: string }>();
  const jobId = id ?? '';

  const { data: job, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['job', jobId],
    queryFn: () => jobsApi.get(jobId),
    enabled: !!jobId,
    refetchInterval: (query) => {
      const d = query.state.data;
      if (!d) return 5000;
      if (d.status === 'awaiting_review' || d.status === 'complete' || d.status === 'cancelled') return false;
      return 5000;
    },
  });

  const { events, connected } = useJobEvents(jobId);

  // Derive the latest download_progress event for the progress bar
  const latestProgress = useMemo(() => {
    for (let i = events.length - 1; i >= 0; i--) {
      if (events[i].event_type === 'download_progress') {
        const d = events[i].payload;
        if (d && typeof d.percent === 'number') return d.percent as number;
      }
    }
    return null;
  }, [events]);

  // Filter out download_progress events from the timeline — show a progress bar instead
  const timelineEvents = useMemo(
    () => events.filter((e) => e.event_type !== 'download_progress'),
    [events],
  );

  if (!jobId) {
    return <div className="p-6 text-red-600">No job ID provided.</div>;
  }

  if (isLoading) {
    return (
      <div className="p-6 flex items-center gap-2 text-gray-500">
        <span className="animate-spin text-lg">⏳</span>
        <span>Loading job…</span>
      </div>
    );
  }

  if (isError || !job) {
    return (
      <div className="p-6 text-red-600">
        Failed to load job: {error instanceof Error ? error.message : 'Unknown error'}
      </div>
    );
  }

  const scene = job.scene;
  const results = [...(job.search_results ?? [])].sort(
    (a, b) => b.confidence_score - a.confidence_score,
  );

  const handleApprove = async (resultId: string) => {
    await jobsApi.approve(jobId, { result_id: resultId });
    await refetch();
  };

  return (
    <div className="flex h-full min-h-screen">
      {/* Left column — metadata + results */}
      <div className="flex-1 min-w-0 overflow-y-auto p-6 border-r border-gray-200">
        {/* Scene metadata */}
        <div className="bg-white rounded-lg border border-gray-200 p-5 mb-6">
          <div className="flex items-start justify-between gap-4 flex-wrap">
            <div className="flex-1 min-w-0">
              <h1 className="text-xl font-semibold text-gray-900 truncate">
                {scene?.title ?? job.stashdb_url}
              </h1>
              {scene?.studio_name && (
                <p className="text-sm text-gray-500 mt-0.5">{scene.studio_name}</p>
              )}
            </div>
            <StatusBadge status={job.status} />
          </div>

          <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
            {scene?.performers && scene.performers.length > 0 && (
              <>
                <dt className="text-gray-500 font-medium">Performers</dt>
                <dd className="text-gray-800">
                  {scene.performers.map((p) => p.name).join(', ')}
                </dd>
              </>
            )}
            {scene?.release_date && (
              <>
                <dt className="text-gray-500 font-medium">Release date</dt>
                <dd className="text-gray-800">{scene.release_date}</dd>
              </>
            )}
            {scene?.duration_seconds != null && (
              <>
                <dt className="text-gray-500 font-medium">Duration</dt>
                <dd className="text-gray-800">{formatDuration(scene.duration_seconds)}</dd>
              </>
            )}
            {scene?.stashdb_scene_id && (
              <>
                <dt className="text-gray-500 font-medium">StashDB</dt>
                <dd>
                  <a
                    href={`https://stashdb.org/scenes/${scene.stashdb_scene_id}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-600 hover:underline text-xs"
                  >
                    View on StashDB ↗
                  </a>
                </dd>
              </>
            )}
          </dl>

          {job.error_message && (
            <div className="mt-3 p-2 bg-red-50 border border-red-200 rounded text-xs text-red-700">
              {job.error_message}
            </div>
          )}
        </div>

        {/* Download progress bar */}
        {latestProgress !== null && (
          <div className="mb-4 bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between text-xs text-gray-600 mb-1">
              <span>Download progress</span>
              <span>{latestProgress.toFixed(1)}%</span>
            </div>
            <div className="w-full bg-gray-200 rounded-full h-2">
              <div
                className="bg-blue-500 h-2 rounded-full transition-all duration-300"
                style={{ width: `${Math.min(100, latestProgress)}%` }}
              />
            </div>
          </div>
        )}

        {/* Search results */}
        {results.length > 0 && (
          <div>
            <h2 className="text-sm font-semibold text-gray-700 mb-3">
              Search Results ({results.length})
            </h2>
            <div className="flex flex-col gap-2">
              {results.map((r) => (
                <SearchResultRow
                  key={r.id}
                  result={mapApiResult(r)}
                  onApprove={
                    job.status === 'awaiting_review'
                      ? () => handleApprove(r.id)
                      : undefined
                  }
                />
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Right column — event timeline */}
      <div className="w-80 xl:w-96 flex-shrink-0 flex flex-col h-screen overflow-hidden p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold text-gray-700">Timeline</h2>
          <span
            className={`inline-flex items-center gap-1 text-xs ${
              connected ? 'text-green-600' : 'text-gray-400'
            }`}
          >
            <span
              className={`w-1.5 h-1.5 rounded-full ${
                connected ? 'bg-green-500' : 'bg-gray-300'
              }`}
            />
            {connected ? 'Live' : 'Disconnected'}
          </span>
        </div>
        <div className="flex-1 overflow-hidden">
          <JobEventTimeline events={timelineEvents} live={true} />
        </div>
      </div>
    </div>
  );
}
