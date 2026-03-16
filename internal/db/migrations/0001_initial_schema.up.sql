-- Schema migrations tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Jobs
CREATE TABLE jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT NOT NULL CHECK (type IN ('scene', 'performer', 'studio')),
    status          TEXT NOT NULL DEFAULT 'submitted',
    stashdb_url     TEXT NOT NULL,
    stashdb_id      TEXT,
    parent_batch_id UUID,
    error_message   TEXT,
    retry_count     INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_parent_batch_id ON jobs(parent_batch_id);
CREATE INDEX idx_jobs_created_at ON jobs(created_at DESC);

-- Batch jobs
CREATE TABLE batch_jobs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id              UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    type                TEXT NOT NULL CHECK (type IN ('performer', 'studio')),
    stashdb_entity_id   TEXT NOT NULL,
    entity_name         TEXT,
    total_scene_count   INT,
    enqueued_count      INT NOT NULL DEFAULT 0,
    pending_count       INT NOT NULL DEFAULT 0,
    duplicate_count     INT NOT NULL DEFAULT 0,
    confirmed           BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_batch_jobs_job_id ON batch_jobs(job_id);

-- Add foreign key for jobs.parent_batch_id now that batch_jobs exists
ALTER TABLE jobs ADD CONSTRAINT fk_jobs_parent_batch_id
    FOREIGN KEY (parent_batch_id) REFERENCES batch_jobs(id) ON DELETE SET NULL;

-- Scenes
CREATE TABLE scenes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id              UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    stashdb_scene_id    TEXT NOT NULL,
    title               TEXT NOT NULL,
    studio_name         TEXT,
    studio_slug         TEXT,
    release_date        DATE,
    duration_seconds    INT,
    performers          JSONB NOT NULL DEFAULT '[]',
    tags                JSONB NOT NULL DEFAULT '[]',
    raw_response        JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_scenes_job_id ON scenes(job_id);
CREATE INDEX idx_scenes_stashdb_scene_id ON scenes(stashdb_scene_id);

-- Search results
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
    score_breakdown     JSONB NOT NULL DEFAULT '{}',
    is_selected         BOOLEAN NOT NULL DEFAULT FALSE,
    selected_by         TEXT CHECK (selected_by IN ('auto', 'user')),
    selected_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_search_results_job_id ON search_results(job_id);
CREATE INDEX idx_search_results_confidence ON search_results(confidence_score DESC);

-- Downloads
CREATE TABLE downloads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    sabnzbd_nzo_id  TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued',
    filename        TEXT,
    source_path     TEXT,
    final_path      TEXT,
    size_bytes      BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_downloads_job_id ON downloads(job_id);
CREATE INDEX idx_downloads_sabnzbd_nzo_id ON downloads(sabnzbd_nzo_id);

-- Job events
CREATE TABLE job_events (
    id          BIGSERIAL PRIMARY KEY,
    job_id      UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    event_type  TEXT NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_events_job_id ON job_events(job_id);
CREATE INDEX idx_job_events_created_at ON job_events(created_at DESC);

-- Stash instances
CREATE TABLE stash_instances (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    api_key     TEXT NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Config
CREATE TABLE config (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    description TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Studio aliases
CREATE TABLE studio_aliases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical   TEXT NOT NULL,
    alias       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (alias)
);

CREATE INDEX idx_studio_aliases_alias ON studio_aliases(LOWER(alias));
