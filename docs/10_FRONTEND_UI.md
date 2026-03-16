# Stasharr — Frontend UI Design

## Overview

The Stasharr UI is a React SPA built with Vite. It communicates with the API exclusively via REST and SSE. The UI is functional-first — it does not need to be visually complex, but it needs to be efficient to work through. Users will spend most of their time in the queue view and the review queue.

---

## Tech Stack

| Library | Purpose |
|---|---|
| React 18 | UI framework |
| React Router v6 | Client-side routing |
| TanStack Query | Data fetching, caching, background refetch |
| Zustand | Lightweight global state (auth key, config cache) |
| Tailwind CSS | Styling |

No component library. Components are hand-written with Tailwind. This keeps the bundle small and avoids fighting a component library's opinions.

---

## Routes

| Path | Component | Description |
|---|---|---|
| `/` | `Dashboard` | Overview: active jobs, recent completions, worker status |
| `/queue` | `Queue` | Full job list with status filters |
| `/queue/:id` | `JobDetail` | Single job timeline and detail |
| `/review` | `ReviewQueue` | Jobs awaiting human result selection |
| `/batches` | `Batches` | Performer/studio batch jobs |
| `/batches/:id` | `BatchDetail` | Batch detail and pending confirmation |
| `/config` | `Config` | All configuration |
| `/config/stash` | `StashInstances` | Stash instance management |
| `/config/template` | `TemplateBuilder` | Directory template builder |
| `/config/aliases` | `Aliases` | Studio alias management |

---

## Page Specifications

### Dashboard (`/`)

**Layout:** Two-column. Left: job activity feed. Right: worker status panel + quick stats.

**Job activity feed:**
- Live SSE stream of recent `job_events` from `GET /api/v1/events`
- Each event renders as a compact row: icon, job title (from scene name), event description, timestamp
- Auto-scrolls to newest; user can pause scroll by scrolling up
- Clicking a row navigates to `/queue/:id`

**Worker status panel:**
- Polls `GET /api/v1/status` every 30 seconds
- Shows each worker: name, pool size, active count, running indicator (green dot / red dot)
- One row per worker type

**Quick stats:**
- Jobs today: total submitted, completed, failed
- Review queue count (with link to `/review`)
- Pending batch confirmations count (with link to `/batches`)

---

### Queue (`/queue`)

**Layout:** Full-width table with filter bar.

**Filters (in filter bar):**
- Status: multi-select checkboxes (all statuses)
- Type: scene / performer / studio
- Date range
- Text search (searches scene title)

**Table columns:**
- Type icon (scene/performer/studio)
- Title (from resolved scene metadata, or URL if unresolved)
- Studio
- Status badge (color-coded)
- Created at
- Duration since last update
- Actions: Cancel (if active), Retry (if failed), View

**Behaviour:**
- Infinite scroll (cursor-based pagination via TanStack Query)
- Row clicking navigates to `/queue/:id`
- Status badges are color-coded:

| Status | Color |
|---|---|
| `submitted`, `resolving`, `searching` | Blue (in-progress) |
| `awaiting_review` | Amber |
| `approved`, `downloading`, `moving`, `scanning` | Green (active) |
| `complete` | Gray (done) |
| `*_failed` | Red |
| `cancelled` | Gray |

---

### Job Detail (`/queue/:id`)

**Layout:** Two-column. Left: scene metadata + search results. Right: timeline.

**Scene metadata panel:**
- Scene title, studio, performers, release date, duration
- StashDB link (external)
- Current status badge

**Search results panel** (shown when results exist):
- Table of all search results sorted by confidence score
- Each row: release title, indexer, size, publish date, confidence score badge
- Expandable row showing `score_breakdown` — per-field table showing score/max and match detail
- Approve button on each row (only active when job is in `awaiting_review`)
- Selected result is visually distinguished

**Timeline (right column):**
- Chronological list of all `job_events` for this job
- Each event: icon, description, timestamp
- SSE stream via `GET /api/v1/jobs/:id/events` — new events append in real time
- Download progress events show a progress bar that updates live

---

### Review Queue (`/review`)

This is the highest-priority view. Users should be able to work through items quickly.

**Layout:** Master-detail. Left: list of jobs awaiting review (compact). Right: currently selected job's scene + results.

**Left panel:**
- Jobs sorted oldest-first (these have been waiting longest)
- Compact row: scene title, studio, confidence score of top result, age
- Clicking a row loads the detail on the right without navigating away

**Right panel (same data as Job Detail but optimized for rapid review):**
- Scene metadata at top (compact)
- Search results table with Approve buttons
- Keyboard shortcuts:
  - `1`–`9`: approve result by rank position
  - `→` / `→` arrows: navigate between review items
  - `s`: skip (mark no match)
  - `?`: toggle keyboard shortcut help

**Batch approve:** Not supported in v1. Each result requires deliberate individual approval.

---

### Batches (`/batches`)

**Layout:** Table of all batch jobs.

**Table columns:**
- Type (performer/studio)
- Entity name
- Total scenes
- Enqueued / Pending / Duplicates
- Confirmed status
- Actions

**Confirmation flow:**
When a batch has `pending_count > 0` and `confirmed = false`, a banner is shown:

```
"Performer Name" has 44 more scenes waiting.
First 40 have been queued. [Queue remaining 44 scenes]
```

Clicking "Queue remaining" calls `POST /api/v1/batches/:id/confirm` and removes the banner.

---

### Configuration (`/config`)

**Layout:** Settings page with section headings.

**Sections:**

1. **Connections** — Prowlarr, SABnzbd, StashDB API key. Each has a "Test" button that calls `POST /api/v1/config/test/:service` and shows inline success/failure feedback.

2. **Matching** — `auto_threshold` and `review_threshold` sliders with live threshold explanation:
   - "Scores ≥ 85 will auto-download"
   - "Scores 50–84 will go to review"
   - "Scores < 50 will fail"

3. **Pipeline** — Worker pool sizes, monitor interval, rate limits. Shown as number inputs with descriptions. Note displayed: "Pool size changes require a container restart."

4. **Directory** — Template builder link, missing field value input.

**Save behaviour:** Each section has its own Save button. Changes call `PUT /api/v1/config` with only the changed keys. A success toast confirms the save.

---

### Template Builder (`/config/template`)

**Layout:** Two-column. Left: template input and token reference. Right: live preview.

**Left panel:**
- Text input for the template string
- Token reference table: all available `{tokens}` with descriptions
- Clicking a token in the reference inserts it at cursor position in the input
- Validation errors shown inline

**Right panel:**
- Live preview using synthetic scene data (updates as user types)
- Shows the resolved path string
- Character count on the filename segment (warn if > 200 chars)
- Filesystem safety indicator (flags any dangerous characters)

---

## API Client

All API calls are made through a typed client at `web/src/api/client.ts`. The client:
- Reads `VITE_API_URL` from the Vite environment
- Attaches `X-Api-Key` header to every request (key stored in Zustand)
- Throws typed errors that TanStack Query can surface

**SSE hook:**
```typescript
function useJobEvents(jobId: string) {
  // Returns array of JobEvent, appended in real time
  // Connects to GET /api/v1/jobs/:id/events?api_key=...
  // Reconnects on disconnect with exponential backoff
}

function useGlobalEvents() {
  // Connects to GET /api/v1/events?api_key=...
  // Used by Dashboard
}
```

---

## Authentication State

The API key is entered once via a prompt on first load if not present in `localStorage`. It is stored in `localStorage` and injected into every request via the API client. There is no login page — if the key is wrong, the API returns `401` and the UI shows a persistent "Invalid API key" banner with a "Change key" link.

---

## Toast Notifications

A global toast system shows non-blocking feedback:
- Green: successful actions (job submitted, config saved, batch confirmed)
- Amber: warnings (batch threshold reached, review queue growing)
- Red: errors (API unreachable, action failed)

Toasts auto-dismiss after 4 seconds. Error toasts persist until dismissed.
