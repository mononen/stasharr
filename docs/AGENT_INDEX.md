# Stasharr — Agent Reference Index

> **For AI models:** Start here. This file is a dense, single-page reference covering the entire system.
> Read this file first, then consult individual topic files only when you need deeper detail on a specific area.

---

## What Is Stasharr?

A self-hosted Go/React pipeline that connects StashDB (community adult scene metadata) → Prowlarr (NZB indexer search) → SABnzbd (downloader) → filesystem → StashApp (media manager). Users submit StashDB URLs via a browser userscript or the web UI; the system automates the full acquire-and-import workflow.

**Not an ORM-backed REST service.** All job coordination runs through Postgres (`SELECT ... FOR UPDATE SKIP LOCKED`). No Redis, no external queue, no message broker.

---

## Repository Layout

```
stasharr/
├── cmd/stasharr/main.go             # entrypoint — boot sequence
├── internal/
│   ├── api/                         # Fiber HTTP handlers
│   │   ├── jobs.go                  # job CRUD + actions
│   │   ├── batches.go               # batch CRUD + actions
│   │   ├── config.go                # config + stash instances + aliases
│   │   ├── events.go                # SSE handlers
│   │   └── middleware.go            # auth, logging, CORS
│   ├── worker/
│   │   ├── supervisor.go            # worker lifecycle manager
│   │   ├── resolver.go              # ResolverWorker (submitted → resolved)
│   │   ├── search.go                # SearchWorker (resolved → approved/review)
│   │   ├── download.go              # DownloadWorker (approved → downloading)
│   │   ├── monitor.go               # MonitorWorker (polls SABnzbd)
│   │   ├── mover.go                 # MoveWorker (download_complete → moved)
│   │   ├── scanner.go               # ScanWorker (moved → complete)
│   │   └── local_watcher.go         # LocalWatcherWorker (filesystem events)
│   ├── matcher/
│   │   ├── normalize.go             # string normalization (unicode, punctuation)
│   │   ├── score.go                 # NZB confidence scoring algorithm
│   │   └── template.go              # directory path template engine
│   ├── clients/
│   │   ├── stashdb/client.go        # StashDB GraphQL client (rate-limited)
│   │   ├── stashapp/client.go       # StashApp GraphQL client
│   │   ├── prowlarr/client.go       # Prowlarr REST client
│   │   ├── sabnzbd/client.go        # SABnzbd REST client
│   │   └── myjdownloader/client.go  # MyJDownloader client (not yet active)
│   ├── config/
│   │   ├── env.go                   # env var loading
│   │   └── dbconfig.go              # DB-backed config read/write
│   ├── db/
│   │   ├── migrations/              # *.up.sql sequential migration files
│   │   └── queries/                 # sqlc-generated Go query code
│   └── models/models.go             # App dependency container struct
├── web/src/
│   ├── App.tsx                      # router setup
│   ├── api/client.ts                # typed API client (~all endpoints)
│   ├── pages/                       # page components (see Frontend Routes below)
│   ├── components/                  # shared UI components
│   └── hooks/                       # useStore, useGlobalEvents, useJobEvents
├── scripts/tampermonkey/stasharr.user.js  # browser userscript
├── docker/api.Dockerfile
├── docker/ui.Dockerfile
├── docker-compose.yml               # production
├── docker-compose.dev.yml           # development (hot reload)
└── .env.example
```

---

## Containers

| Container | Image | Port | Purpose |
|-----------|-------|------|---------|
| `stasharr-api` | Go/Fiber binary | 8080 | REST API + SSE + all workers |
| `stasharr-ui` | React + nginx | 3000 (→ :80) | SPA frontend |
| `postgres` | postgres:16-alpine | 5432 | Primary datastore |

External dependencies (user-provided): **Prowlarr**, **SABnzbd**, **StashApp**, **StashDB** (cloud).

---

## Boot Sequence (`cmd/stasharr/main.go`)

1. Load env vars (`STASHARR_DB_DSN`, `STASHARR_SECRET_KEY`, `STASHARR_LISTEN_PORT`, `STASHARR_LOG_LEVEL`, `STASHARR_DEV`)
2. Connect pgxpool, run health check
3. Apply pending SQL migrations from `internal/db/migrations/`
4. Load `config` table → `Config` struct
5. Instantiate `models.App` (DI container)
6. Call `app.RefreshClients()` → build Prowlarr/SABnzbd/StashApp/StashDB clients
7. Start `worker.Supervisor` → launches all worker goroutines
8. Register Fiber routes, start HTTP server on configured port
9. On SIGTERM: stop accepting, drain workers (30s timeout), close DB pool

---

## Database Tables (quick reference)

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `jobs` | Every submitted URL | `id`, `type`, `status`, `stashdb_url`, `stashdb_id`, `parent_batch_id`, `error_message`, `retry_count` |
| `batch_jobs` | Performer/studio batch metadata | `id`, `job_id`, `type`, `stashdb_entity_id`, `entity_name`, `total_scene_count`, `enqueued_count`, `pending_count`, `duplicate_count`, `confirmed`, `last_checked_at` |
| `scenes` | Resolved StashDB metadata | `id`, `job_id`, `stashdb_scene_id`, `title`, `studio_name`, `studio_slug`, `release_date`, `duration_seconds`, `performers` (JSONB), `tags` (JSONB) |
| `search_results` | Prowlarr NZB candidates | `id`, `job_id`, `indexer_name`, `release_title`, `size_bytes`, `publish_date`, `download_url`, `confidence_score`, `score_breakdown` (JSONB), `is_selected`, `selected_by` |
| `downloads` | Active/completed SABnzbd jobs | `id`, `job_id`, `sabnzbd_nzo_id`, `status`, `filename`, `source_path`, `final_path`, `size_bytes`, `completed_at` |
| `job_events` | Append-only state log (BIGSERIAL) | `id`, `job_id`, `event_type`, `payload` (JSONB), `created_at` |
| `stash_instances` | StashApp instance configs | `id`, `name`, `url`, `external_url`, `api_key`, `is_default` |
| `config` | Runtime key-value config | `key`, `value`, `description`, `updated_at` |
| `studio_aliases` | Studio name normalization | `id`, `canonical`, `alias` (UNIQUE) |
| `schema_migrations` | Migration tracking | `version`, `applied_at` |

---

## Job Status State Machine

```
submitted
  └─(ResolverWorker)→ resolving
      ├─(error)→ resolve_failed  ──(retry)──┐
      └─(ok)→ resolved                      │
          └─(SearchWorker)→ searching        │
              ├─(error)→ search_failed       │
              ├─(score≥auto_threshold)→ auto_approved ──┐
              └─(score≥review_threshold)→ awaiting_review│
                  └─(user selects)─────────────────────┘
                                    ↓
                                 approved
                       └─(DownloadWorker)→ downloading
                           ├─(error)→ download_failed
                           └─(MonitorWorker: SABnzbd complete)→ download_complete
                               └─(MoveWorker)→ moving
                                   ├─(error)→ move_failed
                                   └─(ok)→ moved
                                       └─(ScanWorker)→ scanning
                                           ├─(error)→ scan_failed
                                           └─(ok)→ complete

Any status → cancelled  (via DELETE /api/v1/jobs/:id)
```

**Retry routing** (from `internal/api/jobs.go`):
- `resolve_failed` → requeue as `submitted`
- `search_failed` → requeue as `resolved`
- `download_failed` → requeue as `approved`
- `move_failed` → requeue as `download_complete`
- `scan_failed` → requeue as `moved`
- In-progress stuck states can also be force-reset with the same mapping

**Advance routing** (skip a stuck step):
- `downloading` → `download_complete`
- `moving` → `moved`
- `scanning` → `complete`

---

## API Endpoints

All endpoints under `/api/v1`. Auth: `X-Api-Key` header (= `STASHARR_SECRET_KEY`).
SSE endpoints use `?api_key=` query param (browsers can't set headers on EventSource).

### Jobs

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/jobs` | Submit StashDB URL `{url, type}` |
| `GET` | `/jobs` | List jobs (params: `status`, `type`, `batch_id`, `limit`, `before`) |
| `GET` | `/jobs/stats` | Job counts grouped by status |
| `GET` | `/jobs/:id` | Full job detail (scene + results + download + events) |
| `GET` | `/jobs/:id/neighbors` | Prev/next job IDs for navigation |
| `POST` | `/jobs/:id/approve` | Select result `{result_id}` and proceed to download |
| `POST` | `/jobs/:id/retry` | Re-queue from last successful state (resets retry_count) |
| `POST` | `/jobs/:id/advance` | Skip a stuck step (downloading→download_complete, etc.) |
| `POST` | `/jobs/:id/search` | Custom Prowlarr search `{query}` |
| `POST` | `/jobs/:id/local-match` | Import from local filesystem `{source_path?}` |
| `PATCH` | `/jobs/:id/status` | Force a specific status `{status}` |
| `DELETE` | `/jobs/:id` | Cancel job (also removes from SABnzbd if active) |

### Batches

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/batches` | List batch jobs |
| `GET` | `/batches/:id` | Batch detail + child job summary |
| `POST` | `/batches/:id/approve` | Approve scenes for download |
| `POST` | `/batches/:id/deny` | Reject scenes |
| `POST` | `/batches/:id/next` | Queue next page of pending scenes |
| `POST` | `/batches/:id/auto-start` | Auto-start all scenes meeting score threshold |
| `POST` | `/batches/:id/check-latest` | Re-query StashDB for new scenes since last check |
| `DELETE` | `/batches/:id` | Cancel batch |

### Review

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/review` | Jobs in `awaiting_review` status (same shape as `/jobs`) |

### Config

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/config` | All config key-values (secrets masked with `***`) |
| `PUT` | `/config` | Bulk update config keys |
| `POST` | `/config/test/:service` | Test connectivity (`prowlarr`, `sabnzbd`, `stashdb`) |

### Stash Instances

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/stash-instances` | List all Stash instances |
| `POST` | `/stash-instances` | Create instance |
| `PUT` | `/stash-instances/:id` | Update instance |
| `DELETE` | `/stash-instances/:id` | Delete instance |
| `POST` | `/stash-instances/:id/test` | Test connectivity |

### Aliases

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/aliases` | List studio aliases |
| `POST` | `/aliases` | Create alias `{canonical, alias}` |
| `DELETE` | `/aliases/:id` | Delete alias |

### System

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| `GET` | `/health` | None | Unauthenticated liveness check |
| `GET` | `/status` | Header | Worker pool + service health |
| `GET` | `/events` | Query param | SSE — global event stream |
| `GET` | `/jobs/:id/events` | Query param | SSE — per-job event stream |

---

## Workers

All workers are goroutines managed by `worker.Supervisor`. Job claiming pattern:
```sql
SELECT * FROM jobs WHERE status = $1 ORDER BY created_at ASC
FOR UPDATE SKIP LOCKED LIMIT 1
```

| Worker | Input Status | Output Status | Pool Config Key | Default |
|--------|-------------|---------------|-----------------|---------|
| ResolverWorker | `submitted` | `resolved` / `resolve_failed` | `pipeline.resolver_pool_size` | 1 |
| SearchWorker | `resolved` | `auto_approved` / `awaiting_review` / `search_failed` | `pipeline.search_pool_size` | 2 |
| DownloadWorker | `approved` | `downloading` / `download_failed` | `pipeline.download_pool_size` | 2 |
| MonitorWorker | (polls all `downloading`) | `download_complete` / `download_failed` | singleton | 1 |
| MoveWorker | `download_complete` | `moved` / `move_failed` | `pipeline.move_pool_size` | 2 |
| ScanWorker | `moved` | `complete` / `scan_failed` | `pipeline.scan_pool_size` | 2 |
| LocalWatcherWorker | (filesystem events) | triggers job state | singleton | 1 |

**Supervisor behavior:** Panics are recovered; workers restart with exponential backoff (1s → 30s max). Changing pool sizes requires container restart.

---

## Scoring Algorithm (internal/matcher/)

Scores each Prowlarr NZB result against StashDB scene metadata (0–100 total):

| Field | Max Points | Method |
|-------|-----------|--------|
| Title | 40 | Normalized Levenshtein similarity: ≥0.95→40, ≥0.85→30, ≥0.70→15, else 0 |
| Studio | 20 | Exact match after normalization + alias lookup |
| Date | 20 | Exact calendar date match (extracted from NZB title) |
| Duration | 15 | Within ±60 seconds (extracted from NZB title) |
| Performer | 5 | ≥1 exact normalized name match |

**Thresholds:**
- Score ≥ `matching.auto_threshold` (default 85) → `auto_approved`
- Score ≥ `matching.review_threshold` (default 50) → `awaiting_review`
- Score < `matching.review_threshold` → `search_failed`

---

## Runtime Configuration Keys

All stored in `config` table and loaded into memory at boot. Updated via `PUT /api/v1/config`.

```
prowlarr.url                     → Prowlarr base URL
prowlarr.api_key                 → Prowlarr API key
prowlarr.search_limit            → Max results per indexer (default: 10)

sabnzbd.url                      → SABnzbd base URL
sabnzbd.api_key                  → SABnzbd API key
sabnzbd.category                 → SABnzbd category name (default: stasharr)
sabnzbd.complete_dir             → Path to SABnzbd complete folder

stashdb.api_key                  → StashDB API key

matching.auto_threshold          → Auto-approve if score ≥ this (default: 85)
matching.review_threshold        → Send to review if score ≥ this (default: 50)

pipeline.resolver_pool_size      → default: 1
pipeline.search_pool_size        → default: 2
pipeline.download_pool_size      → default: 2
pipeline.move_pool_size          → default: 2
pipeline.scan_pool_size          → default: 2
pipeline.monitor_poll_interval   → SABnzbd poll interval in seconds (default: 30)
pipeline.stashdb_rate_limit      → StashDB requests/sec (default: 5)
pipeline.batch_auto_threshold    → Scene count before batch confirmation required (default: 40)
pipeline.max_retries_resolver    → default: 3
pipeline.max_retries_search      → default: 2
pipeline.max_retries_move        → default: 3
pipeline.max_retries_scan        → default: 5

directory.template               → Path template (default: {studio}/{year}/{performers}/{title} ({year}).{ext})
directory.performer_max          → Max performers before truncation (default: 3)
directory.missing_field_value    → Fallback for null fields (default: 1unknown)

stash_library_path               → Root path of Stash media library
```

---

## Environment Variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `STASHARR_DB_DSN` | Yes | PostgreSQL DSN |
| `STASHARR_SECRET_KEY` | Yes | API auth key (32-byte hex recommended) |
| `STASHARR_LISTEN_PORT` | No | HTTP port (default: 8080) |
| `STASHARR_LOG_LEVEL` | No | Log level (default: info) |
| `STASHARR_DEV` | No | Enable dev mode: open CORS, verbose logging |

---

## External API Summary

| Service | Protocol | Auth | Used For |
|---------|----------|------|---------|
| StashDB | GraphQL (HTTPS) | `ApiKey` header | Scene/performer/studio metadata resolution |
| StashApp | GraphQL (user-configured) | `ApiKey` header | Duplicate detection, trigger scan |
| Prowlarr | REST (user-configured) | `X-Api-Key` header | NZB search across all indexers |
| SABnzbd | REST (user-configured) | `apikey` query param | NZB submission, queue polling |

**StashDB rate limiting:** token bucket, shared across all ResolverWorker instances, default 5 req/sec.

---

## Frontend Routes

| Path | Page | Purpose |
|------|------|---------|
| `/` | `Dashboard.tsx` | Job stats, active jobs, worker health, service status |
| `/queue` | `Queue.tsx` | Paginated job list with status/type/search filters |
| `/queue/:id` | `JobDetail.tsx` | Full job detail: metadata, scored results, timeline |
| `/review` | `ReviewQueue.tsx` | Jobs in `awaiting_review` for manual selection |
| `/batches` | `Batches.tsx` | Performer/studio batch jobs |
| `/batches/:id` | `BatchDetail.tsx` | Batch detail + child job summary |
| `/config` | `Config.tsx` | All runtime config (Prowlarr, SABnzbd, matching, pipeline) |
| `/config/stash` | `StashInstances.tsx` | Stash instance CRUD |
| `/config/template` | `TemplateBuilder.tsx` | Directory template editor with live preview |
| `/config/aliases` | `Aliases.tsx` | Studio name alias management |

**Frontend stack:** React 19, TypeScript 5.9, Vite 8, Tailwind CSS 4.2, TanStack Query 5, Zustand 5, React Router 7.

**Global state (Zustand, `useStore.ts`):** `apiKey` (persisted to localStorage), `theme` (dark/light), `safeMode` (disables destructive UI actions).

**Real-time updates:** SSE via `useGlobalEvents` (dashboard/queue) and `useJobEvents` (job detail). Auth passed as `?api_key=` query param.

---

## SSE Event Types

All events emitted to `job_events` table and streamed via SSE:

```
job_submitted         resolve_started      resolve_complete     resolve_failed
search_started        search_complete      search_failed        results_found
auto_approved         sent_to_review       user_approved
download_submitted    download_progress    download_complete    download_failed
move_started          move_complete        move_failed
scan_triggered        scan_complete        scan_failed
job_complete          job_cancelled
```

SSE format:
```
event: job_event
data: {"job_id":"uuid","event_type":"download_progress","payload":{"percentage":47},"created_at":"..."}

event: ping
data: {}
```

---

## Directory Template Tokens

```
{title}               → Scene title
{studio}              → Studio name (alias-resolved)
{performers}          → Comma-separated performer names (truncated at performer_max)
{year}                → Release year (YYYY)
{month}               → Release month (01–12)
{day}                 → Release day (01–31)
{date}                → Full date (YYYY-MM-DD)
{ext}                 → Video file extension (.mp4, .mkv, etc.)
{missing_field_value} → Fallback when a field is null/empty
```

Default template: `{studio}/{year}/{performers}/{title} ({year}).{ext}`

---

## Batch Job Flow

```
POST /api/v1/jobs { type: "performer" | "studio", url }
  → Creates jobs + batch_jobs records
  → ResolverWorker queries StashDB for all scenes
  → Checks each scene against Stash for duplicates
  → If total_non_duplicate ≤ batch_auto_threshold (40):
      create all scene jobs → submitted
  → Else:
      create first 40 → submitted
      store remainder in batch_jobs.pending (JSONB)
      surface confirmation in UI (/batches/:id)

POST /batches/:id/approve  → enqueue next batch of pending scenes
POST /batches/:id/next     → queue next page
POST /batches/:id/auto-start → auto-start all scenes meeting score threshold
POST /batches/:id/check-latest → re-query StashDB for scenes added since last check
```

---

## Key Design Decisions

1. **No external queue** — Postgres `FOR UPDATE SKIP LOCKED` is the entire job queue mechanism
2. **Inline scoring** — ScorerWorker runs inside SearchWorker (same goroutine), not a separate pool
3. **Append-only events** — `job_events` is never updated, only inserted; provides full audit trail
4. **Single binary** — HTTP server and all workers run in the same process
5. **Fire-and-forget scan** — ScanWorker sends the Stash scan mutation and immediately marks `complete`; Stash handles the scan asynchronously
6. **No file overwrites** — MoveWorker appends `_1`, `_2`, etc. on collision
7. **Secrets masking** — API keys returned as `***` in GET /config; never written to logs

---

## Topic Files

| File | Content |
|------|---------|
| `00_PROJECT_OVERVIEW.md` | Goals, non-goals, tech stack, high-level data flow |
| `01_ARCHITECTURE.md` | Container diagram, backend/frontend structure, directory layout |
| `02_DATABASE_SCHEMA.md` | Full SQL schema for all tables + indexes + migration strategy |
| `03_API_DESIGN.md` | Full endpoint reference with request/response shapes |
| `04_PIPELINE.md` | Worker specifications, state machine, error handling, concurrency |
| `05_MATCHING.md` | Scoring algorithm detail, normalization rules, threshold configuration |
| `06_CONFIGURATION.md` | All env vars + config table keys with defaults and descriptions |
| `07_DIRECTORY_TEMPLATE.md` | Template token reference, path sanitization, collision handling |
| `08_TAMPERMONKEY.md` | Browser userscript usage and installation |
| `09_DOCKER_DEPLOYMENT.md` | Compose setup, networking, volume management |
| `10_FRONTEND_UI.md` | UI routes, components, state management, SSE integration |
| `FUTURE_IMPROVEMENTS.md` | Roadmap items |
