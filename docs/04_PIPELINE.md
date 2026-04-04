# Stasharr — Pipeline & Worker Design

## Overview

The pipeline is a sequential state machine. Every job progresses through a series of statuses, with each transition owned by a specific worker. Workers are goroutine pools that continuously poll Postgres for jobs in their input status using `SELECT ... FOR UPDATE SKIP LOCKED`.

All state transitions are recorded in `job_events`, which feeds the SSE stream and the UI timeline.

---

## State Machine

```
                        [submitted]
                             │
                    ResolverWorker picks up
                             │
                        [resolving]
                        /         \
              (error)  /           \ (success)
                      /             \
              [resolve_failed]    [resolved]
                      │                │
                   (retry)      SearchWorker picks up
                                        │
                                   [searching]
                                   /         \
                         (error)  /           \ (results returned)
                                 /             \
                        [search_failed]    ScorerWorker runs
                                                │
                              ┌─────────────────┴──────────────────┐
                              │                                     │
                       score >= auto_threshold            score < auto_threshold
                              │                                     │
                        [auto_approved]                    [awaiting_review]
                              │                                     │
                              │                          (user selects result)
                              │                                     │
                              └──────────────┬──────────────────────┘
                                             │
                                        [approved]
                                             │
                                  DownloadWorker picks up
                                             │
                                        [downloading]
                                        /          \
                              (failure) /            \ (SABnzbd complete)
                                       /              \
                              [download_failed]  [download_complete]
                                       │                │
                                    (retry)      MoveWorker picks up
                                                        │
                                                   [moving]
                                                   /       \
                                         (error)  /         \ (success)
                                                 /           \
                                          [move_failed]    [moved]
                                                 │              │
                                              (retry)   ScanWorker picks up
                                                                │
                                                          [scanning]
                                                          /         \
                                                (error)  /           \ (success)
                                                        /             \
                                               [scan_failed]       [complete]
```

---

## Worker Specifications

### ResolverWorker

**Input status:** `submitted`
**Output status:** `resolving` → `resolved` | `resolve_failed`

**Responsibilities:**
1. Claim a `submitted` job, set status to `resolving`
2. Parse the `stashdb_url` to extract entity type and ID
3. Query StashDB GraphQL API for scene metadata
4. Insert record into `scenes` table
5. Set job status to `resolved`

**Error handling:**
- Network errors and 5xx: mark `resolve_failed`, increment `retry_count`
- 404 (scene not found): mark `resolve_failed` with descriptive error, do not retry
- 429 (rate limited): backoff and re-queue without incrementing retry count
- Max retries (configurable, default 3): leave in `resolve_failed`, surface in UI

**StashDB GraphQL query (scene):**
```graphql
query FindScene($id: ID!) {
  findScene(id: $id) {
    id
    title
    date
    duration
    studio { name slug }
    performers { performer { name slug disambiguation } }
    tags { name }
    urls { url type }
  }
}
```

**Rate limiting:** Token bucket, max `STASHDB_RATE_LIMIT` requests/second (default 5). Shared across all ResolverWorker instances via a package-level rate limiter.

---

### SearchWorker

**Input status:** `resolved`
**Output status:** `searching` → hands off to ScorerWorker inline

**Responsibilities:**
1. Claim a `resolved` job, set status to `searching`
2. Build a search query from scene metadata (see Query Construction below)
3. Call Prowlarr `/api/v1/search` with the query
4. Receive all results
5. Pass results to ScorerWorker inline (same goroutine)
6. Persist all scored results to `search_results`
7. Transition job to `auto_approved` or `awaiting_review` based on top score

**Query Construction:**

The search query is assembled from available metadata in priority order:
```
"{title} {studio}" if both available
"{title}" if studio unavailable
```

Titles are sanitized before query: punctuation stripped, common filler words removed. The query is intentionally broad — precision comes from scoring, not from the search query.

**Prowlarr API call:**
```
GET /api/v1/search?query={q}&type=search&limit={search_limit}
```

All configured indexers are searched. `search_limit` is a config value (default 10 results per indexer).

**No-results handling:**
- Zero results: mark `search_failed` with message "No results found across all indexers"
- All results score < `review_threshold`: mark `search_failed` with message "Results found but no confident matches"

---

### ScorerWorker

Not an independent worker — runs inline within SearchWorker after results are fetched. Extracted as a separate package (`internal/matcher`) for testability.

**Responsibilities:**
1. Normalize both the StashDB scene metadata and each Prowlarr result title
2. Score each result (see `05_MATCHING.md` for full algorithm)
3. Sort results by score descending
4. Determine auto-approval vs. review based on top score
5. Return scored results to SearchWorker for persistence

---

### DownloadWorker

**Input status:** `approved` (set by either auto-approval or user review action)
**Output status:** `downloading` | `download_failed`

**Responsibilities:**
1. Claim an `approved` job
2. Retrieve the selected `search_result` record
3. Fetch the NZB file from the result's `download_url` (via Prowlarr proxy)
4. Submit the NZB to SABnzbd via `/api?mode=addfile`
5. Store the returned `nzo_id` in the `downloads` table
6. Set job status to `downloading`

**SABnzbd submission:**
```
POST /api?mode=addfile&apikey={key}&cat={category}&nzbname={title}
Body: multipart/form-data with NZB file content
```

The `category` corresponds to `sabnzbd.category` config (default: `stasharr`). The SABnzbd category should be configured in SABnzbd to use a specific download directory that Stasharr has filesystem access to.

---

### MonitorWorker

**Singleton** — one instance, polls on a configurable interval (default 30 seconds).

**Responsibilities:**
1. Fetch all jobs currently in `downloading` status
2. For each, retrieve the `sabnzbd_nzo_id` from `downloads`
3. Query SABnzbd `/api?mode=queue` for active jobs
4. Query SABnzbd `/api?mode=history` for completed/failed jobs
5. Update `downloads.status` to reflect current SABnzbd state
6. Emit `download_progress` events with percentage
7. When a job transitions to SABnzbd `Completed`: update job status to `download_complete`, populate `downloads.filename` and `downloads.source_path`
8. When a job transitions to SABnzbd `Failed`: update job status to `download_failed`

**SABnzbd status mapping:**

| SABnzbd Status | Internal Status |
|---|---|
| Queued | `queued` |
| Downloading | `downloading` |
| Verifying | `verifying` |
| Repairing | `repairing` |
| Extracting | `unpacking` |
| Completed | `complete` → triggers job → `download_complete` |
| Failed | `failed` → triggers job → `download_failed` |

---

### MoveWorker

**Input status:** `download_complete`
**Output status:** `moving` → `moved` | `move_failed`

**Responsibilities:**
1. Claim a `download_complete` job
2. Retrieve scene metadata and `downloads.source_path`
3. Resolve the destination path using the directory template engine (see `08_DIRECTORY_TEMPLATE.md`)
4. Detect file type — handle both single-file and multi-file SABnzbd output (take the largest video file in a directory)
5. Create destination directory if it does not exist
6. Move the file (OS rename where possible, copy+delete across filesystems)
7. Update `downloads.final_path`
8. Set job status to `moved`

**File selection from SABnzbd output:**
SABnzbd may deliver a directory with multiple files (e.g., after par2 repair). MoveWorker selects the file with the largest size matching a known video extension (`.mp4`, `.mkv`, `.avi`, `.wmv`, `.mov`). All other files (`.nfo`, `.jpg`, `.nzb`, par2 fragments) are deleted after the primary file is moved.

**Filesystem safety:**
- Destination path is sanitized (see `08_DIRECTORY_TEMPLATE.md` for character rules)
- If a file already exists at the destination, append `_1`, `_2`, etc. rather than overwriting
- Move is atomic where possible (same filesystem rename). Cross-filesystem moves copy first, verify size matches, then delete source.

---

### ScanWorker

**Input status:** `moved`
**Output status:** `scanning` → `complete` | `scan_failed`

**Responsibilities:**
1. Claim a `moved` job
2. Retrieve `downloads.final_path` and the default Stash instance config
3. Check if a scene with this path already exists in Stash (duplicate guard)
4. Trigger a Stash scan on the parent directory of the final path
5. Set job status to `complete`

**Stash GraphQL mutations:**
```graphql
# Check for existing
query FindSceneByPath($path: String!) {
  findScenes(scene_filter: { path: { value: $path, modifier: EQUALS } }) {
    count
  }
}

# Trigger scan
mutation MetadataScan($input: ScanMetadataInput!) {
  metadataScan(input: $input)
}
```

ScanWorker does not wait for the Stash scan to complete — it fires the mutation and transitions to `complete`. The scan runs asynchronously within Stash.

---

### LocalWatcherWorker

**Singleton** — one instance, monitors a configured filesystem directory for new files.

**Responsibilities:**
1. Watch a configured directory for file creation/move events
2. When a new video file appears, match it to an eligible job
3. Transition the matched job to `download_complete` with the file path
4. The normal MoveWorker and ScanWorker phases then proceed as usual

This allows importing files that already exist on disk (e.g., manually downloaded or transferred) without going through SABnzbd.

The same flow can be triggered manually via `POST /api/v1/jobs/:id/local-match`.

---

## Batch Job Pipeline

Performer and studio submissions follow a separate pre-pipeline before individual scene jobs are created.

```
POST /api/v1/jobs (type: performer|studio)
        │
        ▼
  ResolverWorker (batch mode)
        │
        ├── Query StashDB for all scenes associated with entity
        ├── For each scene, check Stash for existing file (duplicate detection)
        ├── Count non-duplicate scenes
        │
        ├── If count <= batch_auto_threshold (default 40):
        │     create all scene jobs immediately → submitted
        │
        └── If count > batch_auto_threshold:
              ├── Create first 40 scene jobs (status: submitted)
              ├── Store remaining scene IDs in batch_jobs.pending (JSONB)
              ├── Set batch_jobs.pending_count
              └── Surface confirmation request in UI (/batches/:id)

Batch actions (via UI or API):
  POST /batches/:id/approve     → enqueue pending scenes as submitted
  POST /batches/:id/deny        → cancel pending scenes
  POST /batches/:id/next        → queue next page of pending scenes
  POST /batches/:id/auto-start  → auto-start qualifying scenes
  POST /batches/:id/check-latest → re-query StashDB for new scenes since last check
```

Each child scene job proceeds through the normal pipeline independently.

---

## Concurrency Model

Workers claim jobs with:
```sql
SELECT * FROM jobs
WHERE status = $1
ORDER BY created_at ASC
FOR UPDATE SKIP LOCKED
LIMIT 1
```

This pattern allows multiple worker instances to safely process jobs in parallel without a separate locking mechanism. `SKIP LOCKED` means a worker that finds a locked row (being processed by another goroutine) immediately moves on rather than waiting.

**Worker pool sizes** are configurable via the config table. Changing pool sizes requires a container restart (workers are started at boot).

---

## Error Handling & Retries

| Worker | Max Retries | Retry Strategy |
|---|---|---|
| ResolverWorker | 3 | Immediate re-queue on network error; no retry on 404 |
| SearchWorker | 2 | Immediate re-queue |
| DownloadWorker | 1 | No retry — SABnzbd handles its own retry logic |
| MoveWorker | 3 | 30s delay between retries (filesystem may be temporarily locked) |
| ScanWorker | 5 | 60s delay (Stash may be busy scanning) |

Failed jobs with `retry_count >= max_retries` are left in their `*_failed` status. They are visible in the UI and can be manually retried via `POST /api/v1/jobs/:id/retry`, which resets `retry_count` to 0 and re-queues from the last successful status.

---

## Graceful Shutdown

On `SIGTERM`:
1. HTTP server stops accepting new connections
2. Worker supervisor signals all workers to stop claiming new jobs
3. In-flight workers are allowed to complete their current job (or time out after 30s)
4. Postgres connection pool is closed
5. Process exits

Jobs that were in a transitional status (`resolving`, `searching`, `moving`, `scanning`) when the process was interrupted will be re-claimed by workers on next startup. The `SELECT ... FOR UPDATE` lock is released when the connection closes, so no manual cleanup is needed.
