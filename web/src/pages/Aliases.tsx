import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { aliasesApi } from '../api/client';
import type { StudioAlias } from '../api/client';

// ---------------------------------------------------------------------------
// Add form
// ---------------------------------------------------------------------------

interface AddFormProps {
  onAdded: () => void;
}

const AddForm: React.FC<AddFormProps> = ({ onAdded }) => {
  const [canonical, setCanonical] = useState('');
  const [alias, setAlias] = useState('');
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState('');

  const handleAdd = async () => {
    if (!canonical || !alias) {
      setError('Both canonical name and alias are required.');
      return;
    }
    setAdding(true);
    setError('');
    try {
      await aliasesApi.create({ canonical, alias });
      setCanonical('');
      setAlias('');
      onAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add alias');
      setAdding(false);
    }
  };

  const inputClass =
    'block w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500';

  return (
    <div className="rounded-xl border border-blue-200 bg-blue-50 p-4">
      <h3 className="text-sm font-medium text-blue-900 mb-3">Add Alias</h3>
      <div className="flex flex-col sm:flex-row gap-3">
        <div className="flex-1">
          <label className="block text-xs font-medium text-gray-600 mb-1">Canonical name</label>
          <input
            type="text"
            value={canonical}
            onChange={e => setCanonical(e.target.value)}
            placeholder="Studio Name"
            className={inputClass}
          />
        </div>
        <div className="flex-1">
          <label className="block text-xs font-medium text-gray-600 mb-1">Alias</label>
          <input
            type="text"
            value={alias}
            onChange={e => setAlias(e.target.value)}
            placeholder="Alternate Name"
            className={inputClass}
          />
        </div>
        <div className="flex items-end">
          <button
            onClick={handleAdd}
            disabled={adding}
            className="w-full sm:w-auto px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
          >
            {adding ? 'Adding…' : 'Add'}
          </button>
        </div>
      </div>
      {error && <p className="mt-2 text-xs text-red-600">{error}</p>}
    </div>
  );
};

// ---------------------------------------------------------------------------
// Alias row (used in table)
// ---------------------------------------------------------------------------

interface AliasRowProps {
  alias: StudioAlias;
  onDeleted: () => void;
}

const AliasRow: React.FC<AliasRowProps> = ({ alias, onDeleted }) => {
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState('');

  const handleDelete = async () => {
    setDeleting(true);
    setError('');
    try {
      await aliasesApi.delete(alias.id);
      onDeleted();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete');
      setDeleting(false);
    }
  };

  return (
    <tr className="border-t border-gray-100 hover:bg-gray-50 transition">
      <td className="px-4 py-3 text-sm text-gray-900">{alias.canonical}</td>
      <td className="px-4 py-3 text-sm text-gray-700">{alias.alias}</td>
      <td className="px-4 py-3 text-right">
        {error && <span className="mr-3 text-xs text-red-600">{error}</span>}
        <button
          onClick={handleDelete}
          disabled={deleting}
          className="px-3 py-1 text-xs font-medium text-red-700 bg-red-50 border border-red-200 rounded-lg hover:bg-red-100 disabled:opacity-50 transition"
        >
          {deleting ? 'Deleting…' : 'Delete'}
        </button>
      </td>
    </tr>
  );
};

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function Aliases() {
  const { data, isLoading, isError, error, refetch } = useQuery<StudioAlias[]>({
    queryKey: ['aliases'],
    queryFn: () => aliasesApi.list(),
  });

  const handleRefetch = () => { refetch(); };

  if (isLoading) {
    return (
      <div className="p-8 text-sm text-gray-500 animate-pulse">Loading aliases…</div>
    );
  }

  if (isError) {
    return (
      <div className="p-8 text-sm text-red-600">
        Failed to load aliases: {error instanceof Error ? error.message : 'Unknown error'}
      </div>
    );
  }

  const aliases = data ?? [];

  return (
    <div className="max-w-3xl mx-auto px-4 py-8 space-y-6">
      <div>
        <h1 className="text-xl font-bold text-gray-900">Studio Aliases</h1>
        <p className="mt-1 text-sm text-gray-500">
          Map alternate studio names to their canonical names for improved matching.
        </p>
      </div>

      <AddForm onAdded={handleRefetch} />

      {aliases.length === 0 ? (
        <div className="rounded-xl border border-gray-200 bg-white p-8 text-center text-sm text-gray-500">
          No aliases configured. Add one above.
        </div>
      ) : (
        <div className="rounded-xl border border-gray-200 bg-white overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="bg-gray-50">
                <th className="px-4 py-3 text-left text-xs font-semibold text-gray-500 uppercase tracking-wide">
                  Canonical
                </th>
                <th className="px-4 py-3 text-left text-xs font-semibold text-gray-500 uppercase tracking-wide">
                  Alias
                </th>
                <th className="px-4 py-3 text-right text-xs font-semibold text-gray-500 uppercase tracking-wide">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {aliases.map(a => (
                <AliasRow key={a.id} alias={a} onDeleted={handleRefetch} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
