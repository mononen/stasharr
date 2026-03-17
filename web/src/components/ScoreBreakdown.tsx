import React, { useState } from 'react';
import type { FieldScore } from '../api/client';

export type { FieldScore };

interface ScoreBreakdownProps {
  breakdown: Record<string, FieldScore>;
}

const ScoreBreakdown: React.FC<ScoreBreakdownProps> = ({ breakdown }) => {
  const [expanded, setExpanded] = useState(false);

  const fields = Object.entries(breakdown);

  return (
    <div className="mt-1">
      <button
        onClick={() => setExpanded((v) => !v)}
        className="text-xs text-blue-600 dark:text-blue-400 hover:underline focus:outline-none"
        aria-expanded={expanded}
      >
        {expanded ? '▲ Hide breakdown' : '▼ Show score breakdown'}
      </button>

      {expanded && (
        <div className="mt-2 overflow-x-auto">
          <table className="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
            <thead>
              <tr className="bg-gray-50 dark:bg-gray-800 text-gray-600 dark:text-gray-400 uppercase">
                <th className="px-3 py-1.5 text-left font-semibold">Field</th>
                <th className="px-3 py-1.5 text-right font-semibold">Score</th>
                <th className="px-3 py-1.5 text-right font-semibold">Max</th>
                <th className="px-3 py-1.5 text-left font-semibold">Matched</th>
                <th className="px-3 py-1.5 text-left font-semibold">Expected</th>
              </tr>
            </thead>
            <tbody>
              {fields.map(([field, fs]) => {
                const isInfo = fs.max === 0;
                const pct = !isInfo && fs.max > 0 ? (fs.score / fs.max) * 100 : 0;
                const barColor =
                  pct >= 80
                    ? 'bg-green-400'
                    : pct >= 40
                    ? 'bg-amber-400'
                    : 'bg-red-400';

                return (
                  <tr key={field} className="border-t border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    <td className="px-3 py-1.5 font-medium text-gray-700 dark:text-gray-300 capitalize">
                      {field}
                    </td>
                    <td className="px-3 py-1.5 text-right text-gray-800 dark:text-gray-200">
                      {isInfo ? (
                        <span className="text-gray-400 dark:text-gray-500 italic text-xs">—</span>
                      ) : (
                        <div className="flex items-center justify-end gap-1.5">
                          <div className="w-16 bg-gray-200 dark:bg-gray-700 rounded-full h-1.5">
                            <div
                              className={`h-1.5 rounded-full ${barColor}`}
                              style={{ width: `${pct}%` }}
                            />
                          </div>
                          <span>{fs.score}</span>
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-1.5 text-right text-gray-500 dark:text-gray-400">
                      {isInfo ? '—' : fs.max}
                    </td>
                    <td className="px-3 py-1.5 text-gray-600 dark:text-gray-400">
                      {fs.matched !== undefined ? String(fs.matched) : '—'}
                    </td>
                    <td className="px-3 py-1.5 text-gray-600 dark:text-gray-400">
                      {fs.value !== undefined
                        ? fs.value || '—'
                        : fs.similarity !== undefined
                        ? `sim: ${fs.similarity.toFixed(2)}`
                        : fs.delta_seconds !== undefined
                        ? `Δ${fs.delta_seconds}s`
                        : '—'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};

export default ScoreBreakdown;
