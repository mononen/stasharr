import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { stashInstancesApi } from '../api/client';
import type { StashInstance } from '../api/client';
import ConfirmModal from '../components/ConfirmModal';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function maskApiKey(key: string): string {
  if (!key) return '';
  if (key.length <= 8) return '••••••••';
  return key.slice(0, 4) + '••••' + key.slice(-4);
}

type TestStatus = 'idle' | 'testing' | 'ok' | 'error';

// ---------------------------------------------------------------------------
// Inline edit form
// ---------------------------------------------------------------------------

interface EditFormProps {
  instance: StashInstance;
  onSave: () => void;
  onCancel: () => void;
}

const EditForm: React.FC<EditFormProps> = ({ instance, onSave, onCancel }) => {
  const [name, setName] = useState(instance.name);
  const [url, setUrl] = useState(instance.url);
  const [apiKey, setApiKey] = useState('');
  const [isDefault, setIsDefault] = useState(instance.is_default);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const handleSave = async () => {
    setSaving(true);
    setError('');
    try {
      const updates: { name?: string; url?: string; api_key?: string; is_default?: boolean } = {
        name,
        url,
        is_default: isDefault,
      };
      if (apiKey) updates.api_key = apiKey;
      await stashInstancesApi.update(instance.id, updates);
      onSave();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
      setSaving(false);
    }
  };

  const inputClass =
    'block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500';

  return (
    <div className="mt-3 rounded-lg bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 p-4 space-y-3">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1">Name</label>
          <input type="text" value={name} onChange={e => setName(e.target.value)} className={inputClass} />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1">URL</label>
          <input type="url" value={url} onChange={e => setUrl(e.target.value)} className={inputClass} />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1">
            API Key <span className="text-gray-400 dark:text-gray-500">(leave blank to keep current)</span>
          </label>
          <input
            type="password"
            value={apiKey}
            onChange={e => setApiKey(e.target.value)}
            placeholder="••••••••"
            className={inputClass}
          />
        </div>
        <div className="flex items-center gap-2 pt-5">
          <input
            type="checkbox"
            id={`default-${instance.id}`}
            checked={isDefault}
            onChange={e => setIsDefault(e.target.checked)}
            className="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
          />
          <label htmlFor={`default-${instance.id}`} className="text-sm text-gray-700 dark:text-gray-300">
            Set as default instance
          </label>
        </div>
      </div>
      {error && <p className="text-xs text-red-600 dark:text-red-400">{error}</p>}
      <div className="flex items-center gap-2">
        <button
          onClick={handleSave}
          disabled={saving}
          className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          onClick={onCancel}
          className="px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition"
        >
          Cancel
        </button>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// Instance row
// ---------------------------------------------------------------------------

interface InstanceRowProps {
  instance: StashInstance;
  isOnly: boolean;
  onRefetch: () => void;
}

const InstanceRow: React.FC<InstanceRowProps> = ({ instance, isOnly, onRefetch }) => {
  const [editing, setEditing] = useState(false);
  const [testStatus, setTestStatus] = useState<TestStatus>('idle');
  const [testMessage, setTestMessage] = useState('');
  const [showConfirm, setShowConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleTest = async () => {
    setTestStatus('testing');
    setTestMessage('');
    try {
      const result = await stashInstancesApi.test(instance.id);
      setTestStatus(result.ok ? 'ok' : 'error');
      setTestMessage(result.message);
    } catch (err) {
      setTestStatus('error');
      setTestMessage(err instanceof Error ? err.message : 'Test failed');
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      await stashInstancesApi.delete(instance.id);
      onRefetch();
    } catch {
      setDeleting(false);
    }
    setShowConfirm(false);
  };

  return (
    <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
      <div className="flex items-start justify-between gap-4">
        {/* Info */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-gray-900 dark:text-gray-100 truncate">{instance.name}</span>
            {instance.is_default && (
              <span
                title="Default instance"
                className="text-amber-500 text-base leading-none"
              >
                ★
              </span>
            )}
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5 truncate">{instance.url}</p>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5 font-mono">{maskApiKey(instance.api_key)}</p>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 flex-shrink-0">
          <button
            onClick={handleTest}
            disabled={testStatus === 'testing'}
            className="px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-lg hover:bg-blue-100 dark:hover:bg-blue-900/30 disabled:opacity-50 transition"
          >
            {testStatus === 'testing' ? 'Testing…' : 'Test'}
          </button>

          <button
            onClick={() => setEditing(prev => !prev)}
            className="px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition"
          >
            {editing ? 'Cancel' : 'Edit'}
          </button>

          {isOnly ? (
            <div className="relative group">
              <button
                disabled
                className="px-3 py-1.5 text-xs font-medium text-red-400 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg opacity-50 cursor-not-allowed"
              >
                Delete
              </button>
              <div className="absolute right-0 top-full mt-1 z-10 hidden group-hover:block w-48 rounded-lg bg-gray-800 dark:bg-gray-700 text-white text-xs px-3 py-2 shadow-lg pointer-events-none">
                Cannot delete the only instance
              </div>
            </div>
          ) : (
            <button
              onClick={() => setShowConfirm(true)}
              disabled={deleting}
              className="px-3 py-1.5 text-xs font-medium text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg hover:bg-red-100 dark:hover:bg-red-900/30 disabled:opacity-50 transition"
            >
              {deleting ? 'Deleting…' : 'Delete'}
            </button>
          )}
        </div>
      </div>

      {/* Test result */}
      {testStatus !== 'idle' && testMessage && (
        <p className={`mt-2 text-xs font-medium ${testStatus === 'ok' ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
          {testStatus === 'ok' ? '✓' : '✗'} {testMessage}
        </p>
      )}

      {/* Inline edit form */}
      {editing && (
        <EditForm
          instance={instance}
          onSave={() => { setEditing(false); onRefetch(); }}
          onCancel={() => setEditing(false)}
        />
      )}

      {/* Delete confirmation */}
      <ConfirmModal
        isOpen={showConfirm}
        title="Delete instance"
        message={`Are you sure you want to delete "${instance.name}"? This cannot be undone.`}
        onConfirm={handleDelete}
        onCancel={() => setShowConfirm(false)}
      />
    </div>
  );
};

// ---------------------------------------------------------------------------
// Add form
// ---------------------------------------------------------------------------

interface AddFormProps {
  onAdded: () => void;
}

const AddForm: React.FC<AddFormProps> = ({ onAdded }) => {
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [isDefault, setIsDefault] = useState(false);
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState('');

  const handleAdd = async () => {
    if (!name || !url || !apiKey) {
      setError('Name, URL, and API key are required.');
      return;
    }
    setAdding(true);
    setError('');
    try {
      await stashInstancesApi.create({ name, url, api_key: apiKey, is_default: isDefault });
      setName('');
      setUrl('');
      setApiKey('');
      setIsDefault(false);
      onAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add instance');
      setAdding(false);
    }
  };

  const inputClass =
    'block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500';

  return (
    <div className="rounded-xl border border-blue-200 dark:border-blue-700 bg-blue-50 dark:bg-blue-900/20 p-4">
      <h3 className="text-sm font-medium text-blue-900 dark:text-blue-300 mb-3">Add Stash Instance</h3>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1">Name</label>
          <input
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder="My Stash"
            className={inputClass}
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1">URL</label>
          <input
            type="url"
            value={url}
            onChange={e => setUrl(e.target.value)}
            placeholder="http://stash:9999"
            className={inputClass}
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1">API Key</label>
          <input
            type="password"
            value={apiKey}
            onChange={e => setApiKey(e.target.value)}
            placeholder="••••••••"
            className={inputClass}
          />
        </div>
        <div className="flex items-center gap-2 pt-5">
          <input
            type="checkbox"
            id="add-default"
            checked={isDefault}
            onChange={e => setIsDefault(e.target.checked)}
            className="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
          />
          <label htmlFor="add-default" className="text-sm text-gray-700 dark:text-gray-300">Default</label>
        </div>
      </div>
      {error && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{error}</p>}
      <div className="mt-3">
        <button
          onClick={handleAdd}
          disabled={adding}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
        >
          {adding ? 'Adding…' : 'Add Instance'}
        </button>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function StashInstances() {
  const { data, isLoading, isError, error, refetch } = useQuery<StashInstance[]>({
    queryKey: ['stash-instances'],
    queryFn: () => stashInstancesApi.list(),
  });

  const handleRefetch = () => { refetch(); };

  if (isLoading) {
    return (
      <div className="p-8 text-sm text-gray-500 dark:text-gray-400 animate-pulse">Loading Stash instances…</div>
    );
  }

  if (isError) {
    return (
      <div className="p-8 text-sm text-red-600">
        Failed to load instances: {error instanceof Error ? error.message : 'Unknown error'}
      </div>
    );
  }

  const instances = data ?? [];

  return (
    <div className="max-w-3xl mx-auto px-4 py-8 space-y-6">
      <div>
        <h1 className="text-xl font-bold text-gray-900 dark:text-gray-100">Stash Instances</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Manage the Stash server connections used by Stasharr.
        </p>
      </div>

      <AddForm onAdded={handleRefetch} />

      {instances.length === 0 ? (
        <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-8 text-center text-sm text-gray-500 dark:text-gray-400">
          No Stash instances configured. Add one above.
        </div>
      ) : (
        <div className="space-y-3">
          {instances.map(instance => (
            <InstanceRow
              key={instance.id}
              instance={instance}
              isOnly={instances.length === 1}
              onRefetch={handleRefetch}
            />
          ))}
        </div>
      )}
    </div>
  );
}
