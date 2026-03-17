import React, { useState } from 'react';

export interface FieldScore {
  score: number;
  max_score: number;
  matched?: string;
  expected?: string;
}

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
        className="text-xs text-blue-600 hover:underline focus:outline-none"
        aria-expanded={expanded}
      >
        {expanded ? '▲ Hide breakdown' : '▼ Show score breakdown'}
      </button>

      {expanded && (
        <div className="mt-2 overflow-x-auto">
          <table className="min-w-full text-xs border border-gray-200 rounded">
            <thead>
              <tr className="bg-gray-50 text-gray-600 uppercase">
                <th className="px-3 py-1.5 text-left font-semibold">Field</th>
                <th className="px-3 py-1.5 text-right font-semibold">Score</th>
                <th className="px-3 py-1.5 text-right font-semibold">Max</th>
                <th className="px-3 py-1.5 text-left font-semibold">Matched</th>
                <th className="px-3 py-1.5 text-left font-semibold">Expected</th>
              </tr>
            </thead>
            <tbody>
              {fields.map(([field, fs]) => {
                const pct = fs.max_score > 0 ? (fs.score / fs.max_score) * 100 : 0;
                const barColor =
                  pct >= 80
                    ? 'bg-green-400'
                    : pct >= 40
                    ? 'bg-amber-400'
                    : 'bg-red-400';

                return (
                  <tr key={field} className="border-t border-gray-100 hover:bg-gray-50">
                    <td className="px-3 py-1.5 font-medium text-gray-700 capitalize">
                      {field}
                    </td>
                    <td className="px-3 py-1.5 text-right text-gray-800">
                      <div className="flex items-center justify-end gap-1.5">
                        <div className="w-16 bg-gray-200 rounded-full h-1.5">
                          <div
                            className={`h-1.5 rounded-full ${barColor}`}
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                        <span>{fs.score}</span>
                      </div>
                    </td>
                    <td className="px-3 py-1.5 text-right text-gray-500">{fs.max_score}</td>
                    <td className="px-3 py-1.5 text-gray-600">
                      {fs.matched !== undefined ? fs.matched : '—'}
                    </td>
                    <td className="px-3 py-1.5 text-gray-600">
                      {fs.expected !== undefined ? fs.expected : '—'}
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
