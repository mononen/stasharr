import React, { useState } from 'react';
import ScoreBreakdown from './ScoreBreakdown';
import type { FieldScore } from './ScoreBreakdown';

export interface SearchResult {
  id: string;
  title: string;
  indexer: string;
  size: number;       // bytes
  publish_date: string;
  score: number;
  score_breakdown: Record<string, FieldScore>;
  info_url?: string | null;
}

interface SearchResultRowProps {
  result: SearchResult;
  onApprove?: () => void;
  approveLabel?: string;
}

function formatGB(bytes: number): string {
  const gb = bytes / 1_073_741_824;
  return `${gb.toFixed(2)} GB`;
}

function formatDate(dateStr: string): string {
  try {
    return new Date(dateStr).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  } catch {
    return dateStr;
  }
}

function scoreColor(score: number): string {
  if (score >= 80) return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300';
  if (score >= 50) return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
  return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300';
}

const SearchResultRow: React.FC<SearchResultRowProps> = ({ result, onApprove, approveLabel = 'Approve' }) => {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="border border-gray-200 dark:border-gray-700 rounded-lg p-3 bg-white dark:bg-gray-900 hover:bg-gray-50 dark:hover:bg-gray-800 transition">
      <div className="flex flex-wrap items-center gap-3">
        {/* Title */}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate" title={result.title}>
            {result.title}
          </p>
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">{result.indexer}</p>
        </div>

        {/* Metadata chips */}
        <div className="flex items-center gap-2 flex-shrink-0 text-xs text-gray-600 dark:text-gray-400">
          <span>{formatGB(result.size)}</span>
          <span className="text-gray-300 dark:text-gray-600">|</span>
          <span>{formatDate(result.publish_date)}</span>
        </div>

        {/* Confidence score badge */}
        <span
          className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold ${scoreColor(result.score)}`}
        >
          {result.score}%
        </span>

        {/* Approve button */}
        {onApprove && (
          <button
            onClick={onApprove}
            className="flex-shrink-0 px-3 py-1 text-xs font-medium bg-blue-600 text-white rounded hover:bg-blue-700 active:bg-blue-800 transition"
          >
            {approveLabel}
          </button>
        )}

        {/* Expand toggle */}
        <button
          onClick={() => setExpanded((v) => !v)}
          className="flex-shrink-0 text-xs text-gray-400 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 transition"
          aria-label="Toggle score breakdown"
        >
          {expanded ? '▲' : '▼'}
        </button>
      </div>

      {/* Expandable score breakdown */}
      {expanded && (
        <div className="mt-2 pt-2 border-t border-gray-100 dark:border-gray-800">
          {result.info_url && (
            <div className="mb-2">
              <a
                href={result.info_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs text-blue-600 dark:text-blue-400 hover:underline"
              >
                View on {result.indexer} ↗
              </a>
            </div>
          )}
          <ScoreBreakdown breakdown={result.score_breakdown} />
        </div>
      )}
    </div>
  );
};

export default SearchResultRow;
