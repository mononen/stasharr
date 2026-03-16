# Stasharr — Database Schema

All tables use UUIDs as primary keys. Timestamps are `TIMESTAMPTZ` stored in UTC. Migrations are managed via sequential SQL files in `internal/db/migrations/`.

---

## Tables

### `jobs`

The central table. Every submission — scene, performer, or studio — creates a job record. Performer and studio submissions also create a `batch_job` record that references child scene jobs.

```sql
CREATE TABLE jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT NOT NULL CHECK (type IN ('scene', 'performer', 'studio')),
    status          TEXT NOT NULL DEFAULT 'submitted',
    stashdb_url     TEXT NOT NULL,
    stashdb_id      TEXT,                          -- populated after resolve
    parent_batch_id UUID REFERENCES batch_jobs(id) ON DELETE SET NULL,
    error_message   TEXT,
    retry_count     INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_parent_batch_id ON jobs(parent_batch_id);
CREATE INDEX idx_jobs_created_at ON jobs(created_at DESC);
```

**Valid `status` values** (enforced at application layer, documented here):

| Status | Description |
|---|---|
| `submitted` | Job received, not yet picked up |
| `resolving` | ResolverWorker is querying StashDB |
| `resolve_failed` | StashDB lookup failed (retryable) |
| `resolved` | Scene metadata obtained, ready to search |
| `searching` | SearchWorker is querying Prowlarr |
| `search_failed` | Prowlarr search failed or returned no results |
| `awaiting_review` | Results found, confidence below threshold, needs human |
| `approved` | Result selected (auto or manual), ready to submit to SABnzbd |
| `downloading` | NZB submitted to SABnzbd, monitoring |
| `download_failed` | SABnzbd reported failure |
| `download_complete` | SABnzbd finished, file ready to move |
| `moving` | MoveWorker processing file |
| `move_failed` | File move failed |
| `moved` | File in final location, ready to scan |
| `scanning` | ScanWorker has triggered Stash scan |
| `scan_failed` | Stash scan trigger failed |
| `complete` | Pipeline finished successfully |
| `cancelled` | Manually cancelled by user |

---

### `scenes`

Resolved scene metadata from StashDB. One record per scene job, populated by ResolverWorker.

```sql
CREATE TABLE scenes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id              UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    stashdb_scene_id    TEXT NOT NULL,
    title               TEXT NOT NULL,
    studio_name         TEXT,
    studio_slug         TEXT,
    release_date        DATE,
    duration_seconds    INT,
    performers          JSONB NOT NULL DEFAULT '[]',  -- [{name, slug, disambiguation}]
    tags                JSONB NOT NULL DEFAULT '[]',  -- [string]
    raw_response        JSONB,                        -- full StashDB response, for debugging
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_scenes_job_id ON scenes(job_id);
CREATE INDEX idx_scenes_stashdb_scene_id ON scenes(stashdb_scene_id);
```

---

### `search_results`

NZB candidates returned by Prowlarr for a given job. Multiple records per job. Populated by SearchWorker/ScorerWorker.

```sql
CREATE TABLE search_results (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id              UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    indexer_name        TEXT NOT NULL,
    release_title       TEXT NOT NULL,
    size_bytes          BIGINT,
    publish_date        TIMESTAMPTZ,
    download_url        TEXT,
    nzb_id              TEXT,
    confidence_score    INT NOT NULL DEFAULT 0,
    score_breakdown     JSONB NOT NULL DEFAULT '{}', -- per-field scores
    is_selected         BOOLEAN NOT NULL DEFAULT FALSE,
    selected_by         TEXT CHECK (selected_by IN ('auto', 'user', NULL)),
    selected_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_search_results_job_id ON search_results(job_id);
CREATE INDEX idx_search_results_confidence ON search_results(confidence_score DESC);
```

**`score_breakdown` shape:**
```json
{
  "title":     { "score": 40, "max": 40, "matched": true, "similarity": 0.97 },
  "studio":    { "score": 20, "max": 20, "matched": true },
  "date":      { "score": 20, "max": 20, "matched": true },
  "duration":  { "score": 15, "max": 15, "matched": true, "delta_seconds": 12 },
  "performer": { "score":  0, "max":  5, "matched": false }
}
```

---

### `downloads`

Tracks an active or completed SABnzbd download. One record per job (created when job transitions to `downloading`).

```sql
CREATE TABLE downloads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    sabnzbd_nzo_id  TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued',
    filename        TEXT,                  -- populated on completion
    source_path     TEXT,                  -- SABnzbd complete path
    final_path      TEXT,                  -- populated after move
    size_bytes      BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_downloads_job_id ON downloads(job_id);
CREATE INDEX idx_downloads_sabnzbd_nzo_id ON downloads(sabnzbd_nzo_id);
```

**Valid `status` values:** `queued`, `downloading`, `verifying`, `repairing`, `unpacking`, `complete`, `failed`

---

### `batch_jobs`

Tracks performer and studio submissions. A batch job spawns N child scene jobs. Created alongside the parent `jobs` record.

```sql
CREATE TABLE batch_jobs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id              UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    type                TEXT NOT NULL CHECK (type IN ('performer', 'studio')),
    stashdb_entity_id   TEXT NOT NULL,
    entity_name         TEXT,
    total_scene_count   INT,               -- populated after StashDB resolution
    enqueued_count      INT NOT NULL DEFAULT 0,
    pending_count       INT NOT NULL DEFAULT 0,  -- scenes waiting past threshold
    duplicate_count     INT NOT NULL DEFAULT 0,  -- scenes already in Stash
    confirmed           BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_batch_jobs_job_id ON batch_jobs(job_id);
```

---

### `job_events`

Append-only event log for all state transitions and notable actions. Source of truth for SSE streams and the UI timeline view.

```sql
CREATE TABLE job_events (
    id          BIGSERIAL PRIMARY KEY,
    job_id      UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    event_type  TEXT NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_events_job_id ON job_events(job_id);
CREATE INDEX idx_job_events_created_at ON job_events(created_at DESC);
```

**Event types:**

| Event Type | Payload Fields |
|---|---|
| `job_submitted` | `url`, `type` |
| `resolve_started` | — |
| `resolve_complete` | `stashdb_id`, `title` |
| `resolve_failed` | `error` |
| `search_started` | `query` |
| `search_complete` | `result_count`, `top_score` |
| `search_failed` | `error` |
| `auto_approved` | `result_id`, `score` |
| `sent_to_review` | `result_count`, `top_score` |
| `user_approved` | `result_id`, `release_title` |
| `download_submitted` | `nzo_id`, `release_title` |
| `download_progress` | `percentage`, `size_mb` |
| `download_complete` | `filename`, `source_path` |
| `download_failed` | `error` |
| `move_started` | `source`, `destination` |
| `move_complete` | `final_path` |
| `move_failed` | `error` |
| `scan_triggered` | `stash_instance`, `path` |
| `scan_complete` | — |
| `scan_failed` | `error` |
| `job_complete` | `final_path`, `duration_ms` |
| `job_cancelled` | `reason` |

---

### `stash_instances`

User-configured StashApp connections. Indexed for future multi-instance support.

```sql
CREATE TABLE stash_instances (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    api_key     TEXT NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Only one record may have `is_default = TRUE` at a time (enforced at application layer).

---

### `config`

Key-value store for all non-secret application configuration. Loaded into memory on startup and refreshed on write.

```sql
CREATE TABLE config (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    description TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Initial seed values and their keys:** see `06_CONFIGURATION.md`.

---

### `studio_aliases`

User-managed alias table to normalize studio name variations during matching.

```sql
CREATE TABLE studio_aliases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical   TEXT NOT NULL,   -- the "real" name to match against
    alias       TEXT NOT NULL,   -- the variant to normalize to canonical
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (alias)
);

CREATE INDEX idx_studio_aliases_alias ON studio_aliases(LOWER(alias));
```

---

## Migration Strategy

Migrations are plain `.sql` files in `internal/db/migrations/`, named `NNNN_description.sql`. Applied at startup in sequence using a lightweight Go migration runner. A `schema_migrations` table tracks applied migrations.

```sql
CREATE TABLE schema_migrations (
    version     INT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

No ORM. Queries are written in SQL and code-generated via `sqlc`.
