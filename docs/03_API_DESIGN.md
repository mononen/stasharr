# Stasharr — API Design

## General Conventions

- Base path: `/api/v1`
- Content-Type: `application/json` for all request/response bodies
- Auth: Every request must include `X-Api-Key: {STASHARR_SECRET_KEY}` header
- Pagination: cursor-based using `before` (UUID/timestamp) and `limit` (default 50, max 200)
- Errors: consistent error envelope

**Error Envelope:**
```json
{
  "error": {
    "code":    "JOB_NOT_FOUND",
    "message": "No job with id abc123 exists"
  }
}
```

---

## Authentication

All endpoints (including SSE) require the `X-Api-Key` header. The key is sourced from the `STASHARR_SECRET_KEY` environment variable at startup. A missing or invalid key returns `401`.

The Tampermonkey script includes this key in every request. Since deployment is local-only, there is no token refresh or expiry.

---

## Jobs

### `POST /api/v1/jobs`

Submit a StashDB URL for processing.

**Request:**
```json
{
  "url":  "https://stashdb.org/scenes/abc-123",
  "type": "scene"
}
```

`type` must be one of `scene`, `performer`, `studio`. The API validates that the URL structure matches the declared type.

**Response `202 Accepted`:**
```json
{
  "job_id":       "uuid",
  "batch_job_id": null,
  "type":         "scene",
  "status":       "submitted"
}
```

For `performer` or `studio` submissions, a `batch_job_id` is also returned. The actual scene jobs are created asynchronously after resolution.

---

### `GET /api/v1/jobs`

List jobs with filtering and pagination.

**Query params:**
| Param | Type | Description |
|---|---|---|
| `status` | string | Filter by status (comma-separated for multiple) |
| `type` | string | Filter by type: `scene`, `performer`, `studio` |
| `batch_id` | UUID | Filter to children of a batch job |
| `limit` | int | Page size (default 50, max 200) |
| `before` | UUID | Cursor (job ID) for previous page |

**Response `200`:**
```json
{
  "jobs": [
    {
      "id":           "uuid",
      "type":         "scene",
      "status":       "awaiting_review",
      "stashdb_url":  "https://stashdb.org/scenes/abc-123",
      "scene": {
        "title":        "Scene Title",
        "studio_name":  "Studio Name",
        "release_date": "2024-03-15",
        "performers":   ["Performer A", "Performer B"]
      },
      "created_at":   "2024-03-15T10:00:00Z",
      "updated_at":   "2024-03-15T10:00:45Z"
    }
  ],
  "next_cursor": "uuid",
  "total": 142
}
```

---

### `GET /api/v1/jobs/:id`

Get full job detail including scene metadata, search results, and download info.

**Response `200`:**
```json
{
  "id":            "uuid",
  "type":          "scene",
  "status":        "awaiting_review",
  "stashdb_url":   "https://stashdb.org/scenes/abc-123",
  "error_message": null,
  "retry_count":   0,
  "scene": {
    "stashdb_scene_id": "abc-123",
    "title":            "Scene Title",
    "studio_name":      "Studio Name",
    "studio_slug":      "studio-name",
    "release_date":     "2024-03-15",
    "duration_seconds": 2847,
    "performers": [
      { "name": "Performer A", "slug": "performer-a" }
    ],
    "tags": ["tag1", "tag2"]
  },
  "search_results": [
    {
      "id":               "uuid",
      "indexer_name":     "NZBGeek",
      "release_title":    "Studio.Name.Scene.Title.XXX.1080p.NZB-GROUP",
      "size_bytes":       4294967296,
      "publish_date":     "2024-03-16T00:00:00Z",
      "confidence_score": 95,
      "score_breakdown": {
        "title":     { "score": 40, "max": 40, "matched": true, "similarity": 0.97 },
        "studio":    { "score": 20, "max": 20, "matched": true },
        "date":      { "score": 20, "max": 20, "matched": true },
        "duration":  { "score": 15, "max": 15, "matched": true, "delta_seconds": 12 },
        "performer": { "score":  0, "max":  5, "matched": false }
      },
      "is_selected":  false,
      "selected_by":  null
    }
  ],
  "download": null,
  "events": [
    {
      "event_type": "resolve_complete",
      "payload":    { "stashdb_id": "abc-123", "title": "Scene Title" },
      "created_at": "2024-03-15T10:00:12Z"
    }
  ],
  "created_at": "2024-03-15T10:00:00Z",
  "updated_at": "2024-03-15T10:00:45Z"
}
```

---

### `POST /api/v1/jobs/:id/approve`

Select a search result and proceed to download. Used by both the review UI and internal auto-approval logic.

**Request:**
```json
{
  "result_id": "uuid"
}
```

**Response `200`:**
```json
{
  "job_id": "uuid",
  "status": "approved"
}
```

Returns `409` if job is not in `awaiting_review` status.

---

### `GET /api/v1/jobs/stats`

Returns job counts grouped by status.

**Response `200`:**
```json
{
  "submitted": 2,
  "resolved": 0,
  "awaiting_review": 5,
  "downloading": 3,
  "complete": 142,
  ...
}
```

---

### `GET /api/v1/jobs/:id/neighbors`

Returns the IDs of the previous and next jobs (by `created_at`) for queue navigation.

**Response `200`:**
```json
{
  "prev_id": "uuid-or-null",
  "next_id": "uuid-or-null"
}
```

---

### `POST /api/v1/jobs/:id/retry`

Re-queue a failed job from its last successful state. Also works on stuck in-progress states to force a reset.

**Retry routing:**
- `resolve_failed` / `resolving` → `submitted`
- `search_failed` / `searching` → `resolved`
- `download_failed` / `downloading` → `approved`
- `move_failed` / `moving` → `download_complete`
- `scan_failed` / `scanning` → `moved`

**Response `202`:**
```json
{
  "job_id": "uuid",
  "status": "resolved"
}
```

---

### `POST /api/v1/jobs/:id/advance`

Skip a stuck in-progress step to the next state. Used when a worker cannot progress (e.g., SABnzbd job disappeared from queue).

**Advance routing:**
- `downloading` → `download_complete`
- `moving` → `moved`
- `scanning` → `complete`

**Response `200`:**
```json
{
  "job_id": "uuid",
  "status": "download_complete"
}
```

Returns `409` if the current status is not advanceable.

---

### `POST /api/v1/jobs/:id/search`

Trigger a custom Prowlarr search with a user-provided query string. Replaces existing search results.

**Request:**
```json
{
  "query": "custom search string"
}
```

**Response `200`:**
```json
{
  "job_id": "uuid",
  "result_count": 7
}
```

---

### `POST /api/v1/jobs/:id/local-match`

Import a file from the local filesystem instead of downloading via SABnzbd. Skips the download phase entirely.

**Request (optional):**
```json
{
  "source_path": "/path/to/file.mkv"
}
```

If `source_path` is omitted, the LocalWatcherWorker's watched directory is used.

**Response `200`:**
```json
{
  "job_id": "uuid",
  "status": "download_complete"
}
```

---

### `PATCH /api/v1/jobs/:id/status`

Force a job to a specific status. Used for administrative overrides.

**Request:**
```json
{
  "status": "resolved"
}
```

**Response `200`:**
```json
{
  "job_id": "uuid",
  "status": "resolved"
}
```

---

### `DELETE /api/v1/jobs/:id`

Cancel a job. Sets status to `cancelled`. If a SABnzbd job is active, it is also deleted from SABnzbd.

**Response `204 No Content`**

---

## Batch Jobs

### `GET /api/v1/batches`

List batch jobs.

**Response `200`:**
```json
{
  "batches": [
    {
      "id":                 "uuid",
      "type":               "performer",
      "entity_name":        "Performer Name",
      "stashdb_entity_id":  "abc-123",
      "total_scene_count":  84,
      "enqueued_count":     40,
      "pending_count":      44,
      "duplicate_count":    3,
      "confirmed":          false,
      "created_at":         "2024-03-15T10:00:00Z"
    }
  ]
}
```

---

### `GET /api/v1/batches/:id`

Full batch detail including child job summary and status breakdown.

---

### `POST /api/v1/batches/:id/approve`

Approve scenes in the batch for download. Enqueues pending scenes as `submitted` jobs.

**Response `200`:**
```json
{
  "batch_id":       "uuid",
  "newly_enqueued": 44
}
```

---

### `POST /api/v1/batches/:id/deny`

Reject scenes in the batch. Cancels pending scene jobs.

**Response `200`:**
```json
{
  "batch_id":    "uuid",
  "denied":      44
}
```

---

### `POST /api/v1/batches/:id/next`

Queue the next page of pending scenes (up to `batch_auto_threshold` at a time).

**Response `200`:**
```json
{
  "batch_id":       "uuid",
  "newly_enqueued": 40
}
```

---

### `POST /api/v1/batches/:id/auto-start`

Auto-start all scenes in the batch that meet the auto-approve threshold. Triggers search and approval for qualifying scenes without manual intervention.

**Response `200`:**
```json
{
  "batch_id":   "uuid",
  "started":    18
}
```

---

### `POST /api/v1/batches/:id/check-latest`

Re-query StashDB to find scenes added to the performer/studio since the batch was last resolved. New scenes are appended to the batch. Updates `batch_jobs.last_checked_at`.

**Response `200`:**
```json
{
  "batch_id":   "uuid",
  "new_scenes": 3
}
```

---

### `DELETE /api/v1/batches/:id`

Cancel a batch and all its pending child jobs.

**Response `204 No Content`**

---

## Review Queue

### `GET /api/v1/review`

Jobs currently in `awaiting_review` status, ordered by age (oldest first). Includes full scene metadata and search results for each.

**Query params:** `limit`, `before` (same pagination as jobs)

---

## Configuration

### `GET /api/v1/config`

Returns all configuration key-value pairs grouped by category.

**Response `200`:**
```json
{
  "prowlarr": {
    "url":          "http://prowlarr:9696",
    "api_key":      "***",
    "search_limit": "10"
  },
  "sabnzbd": {
    "url":          "http://sabnzbd:8080",
    "api_key":      "***",
    "category":     "stasharr"
  },
  "stashdb": {
    "api_key":      "***"
  },
  "matching": {
    "auto_threshold":   "85",
    "review_threshold": "50"
  },
  "pipeline": {
    "resolver_pool_size":    "1",
    "search_pool_size":      "2",
    "download_pool_size":    "2",
    "move_pool_size":        "2",
    "scan_pool_size":        "2",
    "monitor_poll_interval": "30",
    "stashdb_rate_limit":    "5",
    "batch_auto_threshold":  "40"
  },
  "directory": {
    "template":             "{studio}/{year}/{performers}/{title} ({year}).{ext}",
    "performer_max":        "3",
    "missing_field_value":  "1unknown"
  }
}
```

API keys are masked with `***` in GET responses. To update a key, use PUT.

---

### `PUT /api/v1/config`

Bulk update configuration. Only provided keys are updated — omitted keys are unchanged.

**Request:**
```json
{
  "prowlarr.url":         "http://prowlarr:9696",
  "matching.auto_threshold": "90"
}
```

**Response `200`:** Updated config (same shape as GET)

---

### `POST /api/v1/config/test/:service`

Test connectivity to an external service. `service` is one of `prowlarr`, `sabnzbd`, `stashdb`.

**Response `200`:**
```json
{
  "service": "prowlarr",
  "ok":      true,
  "message": "Connected. 12 indexers available."
}
```

---

## Stash Instances

### `GET /api/v1/stash-instances`
### `POST /api/v1/stash-instances`
### `PUT /api/v1/stash-instances/:id`
### `DELETE /api/v1/stash-instances/:id`

Standard CRUD. At least one instance must exist. Deleting the default instance is rejected unless another is promoted first.

**Instance shape:**
```json
{
  "id":           "uuid",
  "name":         "Main Stash",
  "url":          "http://stash:9999",
  "external_url": "http://192.168.1.10:9999",
  "api_key":      "***",
  "is_default":   true
}
```

`external_url` is optional. When set, it is used by the UI to construct clickable links to scenes in Stash. When absent, `url` is used as fallback.

### `POST /api/v1/stash-instances/:id/test`

Pings the Stash GraphQL endpoint and returns version info.

---

## Studio Aliases

### `GET /api/v1/aliases`
### `POST /api/v1/aliases`
### `DELETE /api/v1/aliases/:id`

Simple CRUD for the studio alias table.

---

## System

### `GET /api/v1/health`

Unauthenticated. Returns `200` if the API is running.

```json
{ "status": "ok" }
```

### `GET /api/v1/status`

Authenticated. Returns current worker states, DB connectivity, and external service reachability.

```json
{
  "workers": {
    "resolver":  { "running": true, "pool_size": 5, "active": 2 },
    "search":    { "running": true, "pool_size": 5, "active": 1 },
    "download":  { "running": true, "pool_size": 3, "active": 1 },
    "monitor":   { "running": true, "last_poll": "2024-03-15T10:01:00Z" },
    "mover":     { "running": true, "pool_size": 3, "active": 0 },
    "scanner":   { "running": true, "pool_size": 3, "active": 0 }
  },
  "database":   { "ok": true },
  "prowlarr":   { "ok": true },
  "sabnzbd":    { "ok": true },
  "stash":      { "ok": true }
}
```

---

## SSE — Server-Sent Events

SSE streams require the same `X-Api-Key` auth, passed as a query param since browser `EventSource` does not support custom headers:

`GET /api/v1/events?api_key={key}`

### Global Event Stream

`GET /api/v1/events`

All job events across all jobs. Useful for the dashboard and queue views.

**Event format:**
```
event: job_event
data: {"job_id":"uuid","event_type":"download_progress","payload":{"percentage":47},"created_at":"..."}
```

Heartbeat ping every 15 seconds to keep the connection alive:
```
event: ping
data: {}
```

### Per-Job Event Stream

`GET /api/v1/jobs/:id/events`

Only events for the specified job. Used by the job detail view.

---

## Tampermonkey Submission Endpoint

The Tampermonkey script uses the standard `POST /api/v1/jobs` endpoint. No special endpoint exists for the script — it is a first-class API consumer. The script includes the `X-Api-Key` header on every request.
