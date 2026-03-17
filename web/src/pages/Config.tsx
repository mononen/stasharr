import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { configApi } from '../api/client';
import type { ConfigResponse } from '../api/client';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type TestStatus = 'idle' | 'testing' | 'ok' | 'error';
type SaveStatus = 'idle' | 'saving' | 'ok' | 'error';

interface InlineFeedbackProps {
  status: SaveStatus | TestStatus;
  errorMessage?: string;
  okLabel?: string;
}

const InlineFeedback: React.FC<InlineFeedbackProps> = ({ status, errorMessage, okLabel = 'Saved' }) => {
  if (status === 'idle') return null;
  if (status === 'saving' || status === 'testing') {
    return <span className="text-sm text-gray-500 dark:text-gray-400 animate-pulse">{status === 'saving' ? 'Saving…' : 'Testing…'}</span>;
  }
  if (status === 'ok') {
    return <span className="text-sm text-green-600 dark:text-green-400 font-medium">✓ {okLabel}</span>;
  }
  return <span className="text-sm text-red-600 dark:text-red-400 font-medium">✗ {errorMessage ?? 'Error'}</span>;
};

// ---------------------------------------------------------------------------
// Section: Connections
// ---------------------------------------------------------------------------

interface ConnectionSectionProps {
  config: ConfigResponse;
  onSaved: (updated: ConfigResponse) => void;
}

const ConnectionsSection: React.FC<ConnectionSectionProps> = ({ config, onSaved }) => {
  const [prowlarrUrl, setProwlarrUrl] = useState(config?.prowlarr?.url ?? '');
  const [prowlarrKey, setProwlarrKey] = useState(config?.prowlarr?.api_key ?? '');
  const [sabnzbdUrl, setSabnzbdUrl] = useState(config?.sabnzbd?.url ?? '');
  const [sabnzbdKey, setSabnzbdKey] = useState(config?.sabnzbd?.api_key ?? '');
  const [stashdbKey, setStashdbKey] = useState(config?.stashdb?.api_key ?? '');

  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle');
  const [saveError, setSaveError] = useState('');
  const [testStatus, setTestStatus] = useState<Record<string, TestStatus>>({});
  const [testMessages, setTestMessages] = useState<Record<string, string>>({});

  useEffect(() => {
    setProwlarrUrl(config?.prowlarr?.url ?? '');
    setProwlarrKey(config?.prowlarr?.api_key ?? '');
    setSabnzbdUrl(config?.sabnzbd?.url ?? '');
    setSabnzbdKey(config?.sabnzbd?.api_key ?? '');
    setStashdbKey(config?.stashdb?.api_key ?? '');
  }, [config]);

  const handleSave = async () => {
    setSaveStatus('saving');
    setSaveError('');
    try {
      const updated = await configApi.update({
        'prowlarr.url': prowlarrUrl,
        'prowlarr.api_key': prowlarrKey,
        'sabnzbd.url': sabnzbdUrl,
        'sabnzbd.api_key': sabnzbdKey,
        'stashdb.api_key': stashdbKey,
      });
      onSaved(updated);
      setSaveStatus('ok');
      setTimeout(() => setSaveStatus('idle'), 3000);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save');
      setSaveStatus('error');
    }
  };

  const handleTest = async (service: 'prowlarr' | 'sabnzbd' | 'stashdb') => {
    setTestStatus(prev => ({ ...prev, [service]: 'testing' }));
    setTestMessages(prev => ({ ...prev, [service]: '' }));
    try {
      const payload: { url?: string; api_key?: string } = {};
      if (service === 'prowlarr') {
        payload.url = prowlarrUrl;
        payload.api_key = prowlarrKey;
      } else if (service === 'sabnzbd') {
        payload.url = sabnzbdUrl;
        payload.api_key = sabnzbdKey;
      } else if (service === 'stashdb') {
        payload.api_key = stashdbKey;
      }

      const result = await configApi.testService(service, payload);
      setTestStatus(prev => ({ ...prev, [service]: result.ok ? 'ok' : 'error' }));
      setTestMessages(prev => ({ ...prev, [service]: result.message }));
    } catch (err) {
      setTestStatus(prev => ({ ...prev, [service]: 'error' }));
      setTestMessages(prev => ({ ...prev, [service]: err instanceof Error ? err.message : 'Test failed' }));
    }
  };

  const inputClass = 'block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500';
  const labelClass = 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1';

  return (
    <section className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-5">Connections</h2>

      {/* Prowlarr */}
      <div className="mb-6">
        <h3 className="text-sm font-medium text-gray-800 dark:text-gray-200 mb-3">Prowlarr</h3>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label className={labelClass}>URL</label>
            <input
              type="url"
              value={prowlarrUrl}
              onChange={e => setProwlarrUrl(e.target.value)}
              placeholder="http://prowlarr:9696"
              className={inputClass}
            />
          </div>
          <div>
            <label className={labelClass}>API Key</label>
            <input
              type="password"
              value={prowlarrKey}
              onChange={e => setProwlarrKey(e.target.value)}
              placeholder="••••••••"
              className={inputClass}
            />
          </div>
        </div>
        <div className="mt-2 flex items-center gap-3">
          <button
            onClick={() => handleTest('prowlarr')}
            disabled={testStatus.prowlarr === 'testing'}
            className="px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-lg hover:bg-blue-100 dark:hover:bg-blue-900/30 disabled:opacity-50 transition"
          >
            Test Prowlarr
          </button>
          <InlineFeedback
            status={testStatus.prowlarr ?? 'idle'}
            errorMessage={testMessages.prowlarr}
            okLabel={testMessages.prowlarr || 'Connected'}
          />
        </div>
      </div>

      {/* SABnzbd */}
      <div className="mb-6">
        <h3 className="text-sm font-medium text-gray-800 dark:text-gray-200 mb-3">SABnzbd</h3>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label className={labelClass}>URL</label>
            <input
              type="url"
              value={sabnzbdUrl}
              onChange={e => setSabnzbdUrl(e.target.value)}
              placeholder="http://sabnzbd:8080"
              className={inputClass}
            />
          </div>
          <div>
            <label className={labelClass}>API Key</label>
            <input
              type="password"
              value={sabnzbdKey}
              onChange={e => setSabnzbdKey(e.target.value)}
              placeholder="••••••••"
              className={inputClass}
            />
          </div>
        </div>
        <div className="mt-2 flex items-center gap-3">
          <button
            onClick={() => handleTest('sabnzbd')}
            disabled={testStatus.sabnzbd === 'testing'}
            className="px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-lg hover:bg-blue-100 dark:hover:bg-blue-900/30 disabled:opacity-50 transition"
          >
            Test SABnzbd
          </button>
          <InlineFeedback
            status={testStatus.sabnzbd ?? 'idle'}
            errorMessage={testMessages.sabnzbd}
            okLabel={testMessages.sabnzbd || 'Connected'}
          />
        </div>
      </div>

      {/* StashDB */}
      <div className="mb-6">
        <h3 className="text-sm font-medium text-gray-800 dark:text-gray-200 mb-3">StashDB</h3>
        <div className="max-w-sm">
          <label className={labelClass}>API Key</label>
          <input
            type="password"
            value={stashdbKey}
            onChange={e => setStashdbKey(e.target.value)}
            placeholder="••••••••"
            className={inputClass}
          />
        </div>
        <div className="mt-2 flex items-center gap-3">
          <button
            onClick={() => handleTest('stashdb')}
            disabled={testStatus.stashdb === 'testing'}
            className="px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-lg hover:bg-blue-100 dark:hover:bg-blue-900/30 disabled:opacity-50 transition"
          >
            Test StashDB
          </button>
          <InlineFeedback
            status={testStatus.stashdb ?? 'idle'}
            errorMessage={testMessages.stashdb}
            okLabel={testMessages.stashdb || 'Connected'}
          />
        </div>
      </div>

      <div className="flex items-center gap-4 pt-2 border-t border-gray-100 dark:border-gray-800">
        <button
          onClick={handleSave}
          disabled={saveStatus === 'saving'}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
        >
          Save Connections
        </button>
        <InlineFeedback status={saveStatus} errorMessage={saveError} />
      </div>
    </section>
  );
};

// ---------------------------------------------------------------------------
// Section: Matching
// ---------------------------------------------------------------------------

interface MatchingSectionProps {
  config: ConfigResponse;
  onSaved: (updated: ConfigResponse) => void;
}

const MatchingSection: React.FC<MatchingSectionProps> = ({ config, onSaved }) => {
  const [autoThreshold, setAutoThreshold] = useState(
    parseInt(config?.matching?.auto_threshold ?? '85', 10)
  );
  const [reviewThreshold, setReviewThreshold] = useState(
    parseInt(config?.matching?.review_threshold ?? '50', 10)
  );
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle');
  const [saveError, setSaveError] = useState('');

  const handleSave = async () => {
    setSaveStatus('saving');
    setSaveError('');
    try {
      const updated = await configApi.update({
        'matching.auto_threshold': String(autoThreshold),
        'matching.review_threshold': String(reviewThreshold),
      });
      onSaved(updated);
      setSaveStatus('ok');
      setTimeout(() => setSaveStatus('idle'), 3000);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save');
      setSaveStatus('error');
    }
  };

  const clampedReview = Math.min(reviewThreshold, autoThreshold);

  return (
    <section className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-5">Matching</h2>

      <div className="space-y-6">
        {/* Auto threshold */}
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-sm font-medium text-gray-700 dark:text-gray-300">Auto-download threshold</label>
            <span className="text-sm font-semibold text-blue-600 dark:text-blue-400 w-8 text-right">{autoThreshold}</span>
          </div>
          <input
            type="range"
            min={0}
            max={100}
            step={1}
            value={autoThreshold}
            onChange={e => setAutoThreshold(parseInt(e.target.value, 10))}
            className="w-full accent-blue-600"
          />
        </div>

        {/* Review threshold */}
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-sm font-medium text-gray-700 dark:text-gray-300">Review threshold</label>
            <span className="text-sm font-semibold text-blue-600 dark:text-blue-400 w-8 text-right">{reviewThreshold}</span>
          </div>
          <input
            type="range"
            min={0}
            max={100}
            step={1}
            value={reviewThreshold}
            onChange={e => setReviewThreshold(parseInt(e.target.value, 10))}
            className="w-full accent-blue-600"
          />
        </div>

        {/* Live explanation */}
        <div className="rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 px-4 py-3 text-sm text-gray-600 dark:text-gray-400 space-y-1">
          <p>
            Scores &ge; <span className="font-semibold text-green-700 dark:text-green-400">{autoThreshold}</span>{' '}
            auto-download
          </p>
          <p>
            Scores{' '}
            <span className="font-semibold text-amber-600 dark:text-amber-400">{clampedReview}</span>
            {autoThreshold - 1 >= clampedReview ? (
              <>–<span className="font-semibold text-amber-600 dark:text-amber-400">{autoThreshold - 1}</span></>
            ) : null}{' '}
            go to review
          </p>
          <p>
            Scores &lt; <span className="font-semibold text-red-600 dark:text-red-400">{clampedReview}</span>{' '}
            fail
          </p>
        </div>
      </div>

      <div className="flex items-center gap-4 pt-4 mt-4 border-t border-gray-100 dark:border-gray-800">
        <button
          onClick={handleSave}
          disabled={saveStatus === 'saving'}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
        >
          Save Matching
        </button>
        <InlineFeedback status={saveStatus} errorMessage={saveError} />
      </div>
    </section>
  );
};

// ---------------------------------------------------------------------------
// Section: Pipeline
// ---------------------------------------------------------------------------

interface PipelineSectionProps {
  config: ConfigResponse;
  onSaved: (updated: ConfigResponse) => void;
}

interface PipelineField {
  key: string;
  label: string;
  description: string;
  requiresRestart?: boolean;
}

const PIPELINE_FIELDS: PipelineField[] = [
  { key: 'pipeline.worker_resolver_pool', label: 'Resolver pool size', description: 'Concurrent resolver goroutines', requiresRestart: true },
  { key: 'pipeline.worker_search_pool', label: 'Searcher pool size', description: 'Concurrent search goroutines', requiresRestart: true },
  { key: 'pipeline.worker_download_pool', label: 'Downloader pool size', description: 'Concurrent download submissions', requiresRestart: true },
  { key: 'pipeline.worker_move_pool', label: 'Mover pool size', description: 'Concurrent file move goroutines', requiresRestart: true },
  { key: 'pipeline.worker_scan_pool', label: 'Scanner pool size', description: 'Concurrent scan trigger goroutines', requiresRestart: true },
  { key: 'pipeline.monitor_poll_interval', label: 'Monitor poll interval (s)', description: 'Seconds between SABnzbd queue polls' },
  { key: 'pipeline.stashdb_rate_limit', label: 'StashDB rate limit (req/s)', description: 'Max StashDB requests per second' },
  { key: 'pipeline.batch_auto_threshold', label: 'Batch auto threshold', description: 'Scenes before batch confirmation required' },
  { key: 'pipeline.max_retries_resolver', label: 'Max retries: resolver', description: 'Max retry attempts for resolve failures' },
  { key: 'pipeline.max_retries_search', label: 'Max retries: search', description: 'Max retry attempts for search failures' },
  { key: 'pipeline.max_retries_move', label: 'Max retries: move', description: 'Max retry attempts for move failures' },
  { key: 'pipeline.max_retries_scan', label: 'Max retries: scan', description: 'Max retry attempts for scan failures' },
];

const DEFAULTS: Record<string, string> = {
  'pipeline.worker_resolver_pool': '5',
  'pipeline.worker_search_pool': '5',
  'pipeline.worker_download_pool': '3',
  'pipeline.worker_move_pool': '3',
  'pipeline.worker_scan_pool': '3',
  'pipeline.monitor_poll_interval': '30',
  'pipeline.stashdb_rate_limit': '5',
  'pipeline.batch_auto_threshold': '40',
  'pipeline.max_retries_resolver': '3',
  'pipeline.max_retries_search': '2',
  'pipeline.max_retries_move': '3',
  'pipeline.max_retries_scan': '5',
};

const PipelineSection: React.FC<PipelineSectionProps> = ({ config, onSaved }) => {
  const initialValues = () =>
    Object.fromEntries(
      PIPELINE_FIELDS.map(f => {
        const [section, key] = f.key.split('.');
        return [f.key, config?.[section]?.[key] ?? DEFAULTS[f.key] ?? ''];
      })
    );

  const [values, setValues] = useState<Record<string, string>>(initialValues);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle');
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    setValues(initialValues());
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config]);

  const handleSave = async () => {
    setSaveStatus('saving');
    setSaveError('');
    try {
      const flat: Record<string, string> = {};
      for (const f of PIPELINE_FIELDS) {
        flat[f.key] = values[f.key];
      }
      const updated = await configApi.update(flat);
      onSaved(updated);
      setSaveStatus('ok');
      setTimeout(() => setSaveStatus('idle'), 3000);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save');
      setSaveStatus('error');
    }
  };

  return (
    <section className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-1">Pipeline</h2>
      <p className="text-xs text-amber-600 dark:text-amber-400 mb-5">Pool size changes require a container restart.</p>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {PIPELINE_FIELDS.map(field => (
          <div key={field.key}>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {field.label}
              {field.requiresRestart && (
                <span className="ml-1 text-xs text-amber-500" title="Requires restart">*</span>
              )}
            </label>
            <input
              type="number"
              min={0}
              value={values[field.key] ?? ''}
              onChange={e => setValues(prev => ({ ...prev, [field.key]: e.target.value }))}
              className="block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{field.description}</p>
          </div>
        ))}
      </div>

      <div className="flex items-center gap-4 pt-4 mt-4 border-t border-gray-100 dark:border-gray-800">
        <button
          onClick={handleSave}
          disabled={saveStatus === 'saving'}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
        >
          Save Pipeline
        </button>
        <InlineFeedback status={saveStatus} errorMessage={saveError} />
      </div>
    </section>
  );
};

// ---------------------------------------------------------------------------
// Section: Directory
// ---------------------------------------------------------------------------

interface DirectorySectionProps {
  config: ConfigResponse;
  onSaved: (updated: ConfigResponse) => void;
}

const DirectorySection: React.FC<DirectorySectionProps> = ({ config, onSaved }) => {
  const [missingFieldValue, setMissingFieldValue] = useState(
    config?.directory?.missing_field_value ?? '1unknown'
  );
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle');
  const [saveError, setSaveError] = useState('');

  const handleSave = async () => {
    setSaveStatus('saving');
    setSaveError('');
    try {
      const updated = await configApi.update({
        'directory.missing_field_value': missingFieldValue,
      });
      onSaved(updated);
      setSaveStatus('ok');
      setTimeout(() => setSaveStatus('idle'), 3000);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save');
      setSaveStatus('error');
    }
  };

  return (
    <section className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-5">Directory</h2>

      <div className="mb-6">
        <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
          Configure the directory template for downloaded files.
        </p>
        <Link
          to="/config/template"
          className="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-medium text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-lg hover:bg-blue-100 dark:hover:bg-blue-900/30 transition"
        >
          Open Template Builder →
        </Link>
      </div>

      <div className="max-w-sm">
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          Missing field value
        </label>
        <input
          type="text"
          value={missingFieldValue}
          onChange={e => setMissingFieldValue(e.target.value)}
          placeholder="1unknown"
          className="block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Substituted when a metadata field is null (e.g. missing studio or date).
        </p>
      </div>

      <div className="flex items-center gap-4 pt-4 mt-4 border-t border-gray-100 dark:border-gray-800">
        <button
          onClick={handleSave}
          disabled={saveStatus === 'saving'}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
        >
          Save Directory
        </button>
        <InlineFeedback status={saveStatus} errorMessage={saveError} />
      </div>
    </section>
  );
};

// ---------------------------------------------------------------------------
// Main Config page
// ---------------------------------------------------------------------------

export default function Config() {
  const { data, isLoading, isError, error, refetch } = useQuery<ConfigResponse>({
    queryKey: ['config'],
    queryFn: () => configApi.get(),
  });

  const [liveConfig, setLiveConfig] = useState<ConfigResponse | null>(null);

  const handleSaved = (updated: ConfigResponse) => {
    setLiveConfig(updated);
    refetch();
  };

  if (isLoading) {
    return (
      <div className="p-8 text-sm text-gray-500 dark:text-gray-400 animate-pulse">Loading configuration…</div>
    );
  }

  if (isError) {
    return (
      <div className="p-8 text-sm text-red-600">
        Failed to load configuration: {error instanceof Error ? error.message : 'Unknown error'}
      </div>
    );
  }

  const cfg = liveConfig ?? data ?? {};

  return (
    <div className="max-w-4xl mx-auto px-4 py-8 space-y-6">
      <div>
        <h1 className="text-xl font-bold text-gray-900 dark:text-gray-100">Configuration</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Each section saves independently. Changes take effect immediately except pool sizes.
        </p>
      </div>

      <ConnectionsSection config={cfg} onSaved={handleSaved} />
      <MatchingSection
        key={`${cfg?.matching?.auto_threshold}-${cfg?.matching?.review_threshold}`}
        config={cfg}
        onSaved={handleSaved}
      />
      <PipelineSection config={cfg} onSaved={handleSaved} />
      <DirectorySection
        key={cfg?.directory?.missing_field_value ?? ''}
        config={cfg}
        onSaved={handleSaved}
      />
    </div>
  );
}
