import { useStore } from '../hooks/useStore';

const API_BASE = '';

// ---------------------------------------------------------------------------
// Error class
// ---------------------------------------------------------------------------

export class ApiError extends Error {
  code: string;
  constructor(code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
  }
}

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

export type JobType = 'scene' | 'performer' | 'studio';

export type JobStatus =
  | 'submitted'
  | 'resolving'
  | 'resolve_failed'
  | 'resolved'
  | 'searching'
  | 'search_failed'
  | 'awaiting_review'
  | 'approved'
  | 'downloading'
  | 'download_failed'
  | 'download_complete'
  | 'moving'
  | 'move_failed'
  | 'moved'
  | 'scanning'
  | 'scan_failed'
  | 'complete'
  | 'cancelled';

export type DownloadStatus =
  | 'queued'
  | 'downloading'
  | 'verifying'
  | 'repairing'
  | 'unpacking'
  | 'complete'
  | 'failed';

export interface Performer {
  name: string;
  slug: string;
}

export interface SceneSummary {
  title: string;
  studio_name: string | null;
  release_date: string | null;
  performers: string[];
}

export interface SceneDetail {
  stashdb_scene_id: string;
  title: string;
  studio_name: string | null;
  studio_slug: string | null;
  release_date: string | null;
  duration_seconds: number | null;
  performers: Performer[];
  tags: string[];
}

export interface FieldScore {
  score: number;
  max: number;
  matched?: boolean;
  similarity?: number;
  delta_seconds?: number;
  value?: string; // used by informational-only fields like resolution
}

export interface SearchResult {
  id: string;
  indexer_name: string;
  release_title: string;
  size_bytes: number | null;
  publish_date: string | null;
  info_url: string | null;
  confidence_score: number;
  score_breakdown: Record<string, FieldScore>;
  is_selected: boolean;
  selected_by: 'auto' | 'user' | null;
}

export interface Download {
  id: string;
  job_id: string;
  sabnzbd_nzo_id: string;
  status: DownloadStatus;
  filename: string | null;
  source_path: string | null;
  final_path: string | null;
  size_bytes: number | null;
  created_at: string;
  updated_at: string;
  completed_at: string | null;
}

export interface JobEvent {
  event_type: string;
  payload: Record<string, unknown>;
  created_at: string;
}

// Job list item (summary shape returned by GET /jobs)
export interface JobSummary {
  id: string;
  type: JobType;
  status: JobStatus;
  stashdb_url: string;
  scene: SceneSummary | null;
  created_at: string;
  updated_at: string;
}

// Full job detail returned by GET /jobs/:id
export interface JobDetail {
  id: string;
  type: JobType;
  status: JobStatus;
  stashdb_url: string;
  error_message: string | null;
  retry_count: number;
  scene: SceneDetail | null;
  search_results: SearchResult[];
  download: Download | null;
  events: JobEvent[];
  created_at: string;
  updated_at: string;
}

export interface BatchJob {
  id: string;
  type: 'performer' | 'studio';
  entity_name: string | null;
  stashdb_entity_id: string;
  total_scene_count: number | null;
  enqueued_count: number;
  pending_count: number;
  duplicate_count: number;
  confirmed: boolean;
  created_at: string;
}

export interface StashInstance {
  id: string;
  name: string;
  url: string;
  api_key: string;
  is_default: boolean;
}

export interface StudioAlias {
  id: string;
  canonical: string;
  alias: string;
  created_at: string;
}

export type ConfigResponse = Record<string, Record<string, string>>;

export interface WorkerPoolStatus {
  running: boolean;
  pool_size: number;
  active: number;
}

export interface MonitorStatus {
  running: boolean;
  last_poll: string | null;
}

export interface SystemStatus {
  workers: Record<string, { running: boolean; pool_size: number }>;
  database: { ok: boolean };
  prowlarr: { ok: boolean };
  sabnzbd: { ok: boolean };
  stash: { ok: boolean };
}

export interface ServiceTestResult {
  service: string;
  ok: boolean;
  message: string;
}

// ---------------------------------------------------------------------------
// Request / response shapes
// ---------------------------------------------------------------------------

export interface SubmitJobRequest {
  url: string;
  type: JobType;
}

export interface SubmitJobResponse {
  job_id: string;
  batch_job_id: string | null;
  type: JobType;
  status: JobStatus;
}

export interface ListJobsParams {
  status?: string;
  type?: JobType;
  batch_id?: string;
  limit?: number;
  before?: string;
}

export interface ListJobsResponse {
  jobs: JobSummary[];
  next_cursor: string | null;
  total: number;
}

export interface ApproveJobRequest {
  result_id: string;
}

export interface ApproveJobResponse {
  job_id: string;
  status: JobStatus;
}

export interface RetryJobResponse {
  job_id: string;
  status: JobStatus;
}

export interface ListBatchesResponse {
  batches: BatchJob[];
}

export interface ConfirmBatchResponse {
  batch_id: string;
  newly_enqueued: number;
}

export interface CreateStashInstanceRequest {
  name: string;
  url: string;
  api_key: string;
  is_default?: boolean;
}

export interface UpdateStashInstanceRequest {
  name?: string;
  url?: string;
  api_key?: string;
  is_default?: boolean;
}

export interface CreateAliasRequest {
  canonical: string;
  alias: string;
}

// ---------------------------------------------------------------------------
// Internal fetch helper
// ---------------------------------------------------------------------------

function getApiKey(): string {
  return useStore.getState().apiKey;
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const apiKey = getApiKey();
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'X-Api-Key': apiKey,
      ...options?.headers,
    },
  });

  if (!res.ok) {
    // Try to parse the error envelope
    let code = `HTTP_${res.status}`;
    let message = `Request failed with status ${res.status}`;
    try {
      const body = await res.json() as { error?: { code?: string; message?: string } };
      if (body?.error) {
        code = body.error.code ?? code;
        message = body.error.message ?? message;
      }
    } catch {
      // ignore parse failure
    }
    throw new ApiError(code, message);
  }

  // 204 No Content — return undefined cast as T
  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

// ---------------------------------------------------------------------------
// SSE helpers
// ---------------------------------------------------------------------------

export function createJobEventSource(jobId: string, apiKey?: string): EventSource {
  const key = apiKey ?? getApiKey();
  const url = `${API_BASE}/api/v1/jobs/${encodeURIComponent(jobId)}/events?api_key=${encodeURIComponent(key)}`;
  return new EventSource(url);
}

export function createGlobalEventSource(apiKey?: string): EventSource {
  const key = apiKey ?? getApiKey();
  const url = `${API_BASE}/api/v1/events?api_key=${encodeURIComponent(key)}`;
  return new EventSource(url);
}

// ---------------------------------------------------------------------------
// Jobs
// ---------------------------------------------------------------------------

export const jobsApi = {
  submit(req: SubmitJobRequest): Promise<SubmitJobResponse> {
    return apiFetch<SubmitJobResponse>('/api/v1/jobs', {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },

  list(params?: ListJobsParams): Promise<ListJobsResponse> {
    const qs = params ? buildQuery(params as Record<string, unknown>) : '';
    return apiFetch<ListJobsResponse>(`/api/v1/jobs${qs}`);
  },

  get(id: string): Promise<JobDetail> {
    return apiFetch<JobDetail>(`/api/v1/jobs/${encodeURIComponent(id)}`);
  },

  approve(id: string, req: ApproveJobRequest): Promise<ApproveJobResponse> {
    return apiFetch<ApproveJobResponse>(`/api/v1/jobs/${encodeURIComponent(id)}/approve`, {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },

  retry(id: string): Promise<RetryJobResponse> {
    return apiFetch<RetryJobResponse>(`/api/v1/jobs/${encodeURIComponent(id)}/retry`, {
      method: 'POST',
    });
  },

  retryFromStart(id: string): Promise<RetryJobResponse> {
    return apiFetch<RetryJobResponse>(`/api/v1/jobs/${encodeURIComponent(id)}/retry?from_start=true`, {
      method: 'POST',
    });
  },

  cancel(id: string): Promise<void> {
    return apiFetch<void>(`/api/v1/jobs/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  },
};

// ---------------------------------------------------------------------------
// Batches
// ---------------------------------------------------------------------------

export const batchesApi = {
  list(): Promise<ListBatchesResponse> {
    return apiFetch<ListBatchesResponse>('/api/v1/batches');
  },

  get(id: string): Promise<BatchJob> {
    return apiFetch<BatchJob>(`/api/v1/batches/${encodeURIComponent(id)}`);
  },

  confirm(id: string): Promise<ConfirmBatchResponse> {
    return apiFetch<ConfirmBatchResponse>(`/api/v1/batches/${encodeURIComponent(id)}/confirm`, {
      method: 'POST',
    });
  },
};

// ---------------------------------------------------------------------------
// Review queue
// ---------------------------------------------------------------------------

export const reviewApi = {
  list(params?: Pick<ListJobsParams, 'limit' | 'before'>): Promise<ListJobsResponse> {
    const qs = params ? buildQuery(params as Record<string, unknown>) : '';
    return apiFetch<ListJobsResponse>(`/api/v1/review${qs}`);
  },
};

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

export const configApi = {
  get(): Promise<ConfigResponse> {
    return apiFetch<ConfigResponse>('/api/v1/config');
  },

  update(updates: Record<string, string>): Promise<ConfigResponse> {
    return apiFetch<ConfigResponse>('/api/v1/config', {
      method: 'PUT',
      body: JSON.stringify(updates),
    });
  },

  testService(service: 'prowlarr' | 'prowlarr-apikey' | 'sabnzbd' | 'sabnzbd-apikey' | 'stashdb', payload?: { url?: string; api_key?: string }): Promise<ServiceTestResult> {
    return apiFetch<ServiceTestResult>(`/api/v1/config/test/${encodeURIComponent(service)}`, {
      method: 'POST',
      body: payload ? JSON.stringify(payload) : undefined,
    });
  },
};

// ---------------------------------------------------------------------------
// Stash instances
// ---------------------------------------------------------------------------

export const stashInstancesApi = {
  list(): Promise<StashInstance[]> {
    return apiFetch<{ instances: StashInstance[] }>('/api/v1/stash-instances').then((r) => r.instances);
  },

  create(req: CreateStashInstanceRequest): Promise<StashInstance> {
    return apiFetch<StashInstance>('/api/v1/stash-instances', {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },

  update(id: string, req: UpdateStashInstanceRequest): Promise<StashInstance> {
    return apiFetch<StashInstance>(`/api/v1/stash-instances/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    });
  },

  delete(id: string): Promise<void> {
    return apiFetch<void>(`/api/v1/stash-instances/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  },

  test(id: string): Promise<ServiceTestResult> {
    return apiFetch<ServiceTestResult>(
      `/api/v1/stash-instances/${encodeURIComponent(id)}/test`,
      { method: 'POST' },
    );
  },
};

// ---------------------------------------------------------------------------
// Studio aliases
// ---------------------------------------------------------------------------

export const aliasesApi = {
  list(): Promise<StudioAlias[]> {
    return apiFetch<{ aliases: StudioAlias[] }>('/api/v1/aliases').then((r) => r.aliases);
  },

  create(req: CreateAliasRequest): Promise<StudioAlias> {
    return apiFetch<StudioAlias>('/api/v1/aliases', {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },

  delete(id: string): Promise<void> {
    return apiFetch<void>(`/api/v1/aliases/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  },
};

// ---------------------------------------------------------------------------
// System
// ---------------------------------------------------------------------------

export const systemApi = {
  health(): Promise<{ status: string }> {
    return apiFetch<{ status: string }>('/api/v1/health');
  },

  status(): Promise<SystemStatus> {
    return apiFetch<SystemStatus>('/api/v1/status');
  },
};

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

function buildQuery(params: Record<string, unknown>): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null) {
      qs.set(k, String(v));
    }
  }
  const str = qs.toString();
  return str ? `?${str}` : '';
}
