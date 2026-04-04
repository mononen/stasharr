# Stasharr — Architecture

## Container Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Docker Network                    │
│                                                     │
│  ┌──────────────┐       ┌──────────────────────┐   │
│  │ stasharr-ui  │       │    stasharr-api       │   │
│  │  (React/     │◄─────►│    (Go / Fiber)       │   │
│  │   nginx)     │  REST │                       │   │
│  │  :3000       │  SSE  │  :8080                │   │
│  └──────────────┘       └──────────┬────────────┘   │
│                                    │                │
│                          ┌─────────▼──────────┐    │
│                          │     PostgreSQL      │    │
│                          │     :5432           │    │
│                          └────────────────────┘    │
└─────────────────────────────────────────────────────┘

External Services (user's existing stack):
  - Prowlarr        (indexer aggregator)
  - SABnzbd         (download client)
  - StashApp        (media manager / scan target)
  - StashDB         (metadata source, via HTTPS)
```

---

## Backend Structure (`stasharr-api`)

The API container runs two logical subsystems in a single binary:

1. **HTTP Server** — Fiber-based REST API and SSE event streams consumed by the UI and Tampermonkey script
2. **Worker Pool** — goroutine-based workers that process the job pipeline

Both subsystems share a single Postgres connection pool. Workers communicate state exclusively through the database — there is no in-memory shared state between the HTTP layer and workers.

### Worker Architecture

Workers are discrete goroutines managed by a supervisor. Each worker type runs a configurable number of concurrent instances. Workers claim jobs using `SELECT ... FOR UPDATE SKIP LOCKED` to prevent double-processing without distributed locking overhead.

```
WorkerSupervisor
    ├── ResolverWorker      (pool: configurable, default 1)
    ├── SearchWorker        (pool: configurable, default 2)
    ├── DownloadWorker      (pool: configurable, default 2)
    ├── MonitorWorker       (singleton — polls SABnzbd on interval)
    ├── MoveWorker          (pool: configurable, default 2)
    ├── ScanWorker          (pool: configurable, default 2)
    └── LocalWatcherWorker  (singleton — watches filesystem for local file imports)
```

The supervisor is responsible for:
- Starting workers on boot
- Restarting crashed workers with exponential backoff
- Exposing worker health state via the `/api/v1/status` endpoint
- Draining workers gracefully on SIGTERM

### Dependency Injection

A single `App` struct is passed through the application, holding:
- `*pgxpool.Pool` — database connection pool
- `*config.Config` — runtime configuration (loaded from DB on boot)
- `*prowlarr.Client` — Prowlarr HTTP client
- `*sabnzbd.Client` — SABnzbd HTTP client
- `*stashapp.Client` — StashApp GraphQL client
- `*stashdb.Client` — StashDB GraphQL client
- `*worker.Supervisor` — worker supervisor handle

---

## Frontend Structure (`stasharr-ui`)

Single-page React application. Built with Vite, served in production by nginx. Communicates exclusively with `stasharr-api` over REST and SSE.

The UI is **read-heavy** — most interactions are monitoring pipeline state, reviewing match queues, and adjusting configuration. Write operations are limited to config updates, result approvals, and batch confirmations.

### Key UI Sections

| Route | Purpose |
|---|---|
| `/` | Dashboard — active jobs, recent completions, worker status |
| `/queue` | Full job queue with filters (status, type, date, search) |
| `/queue/:id` | Single job detail — metadata, search results, timeline |
| `/review` | Match review queue — jobs awaiting human result selection |
| `/batches` | Batch jobs (performer/studio submissions) and confirmations |
| `/batches/:id` | Batch detail with child job summary |
| `/config` | All application configuration |
| `/config/stash` | Stash instance management |
| `/config/template` | Directory template builder with live preview |
| `/config/aliases` | Studio name alias management |

---

## Directory Layout (Repository)

```
stasharr/
├── cmd/
│   └── stasharr/
│       └── main.go               # entrypoint
├── internal/
│   ├── api/                      # Fiber route handlers
│   │   ├── jobs.go
│   │   ├── batches.go
│   │   ├── config.go
│   │   ├── events.go             # SSE handlers
│   │   └── middleware.go         # auth, logging
│   ├── config/
│   │   ├── env.go                # env var loading + validation
│   │   └── dbconfig.go           # DB-stored config read/write
│   ├── db/
│   │   ├── migrations/           # SQL migration files
│   │   └── queries/              # sqlc-generated query files
│   ├── worker/
│   │   ├── supervisor.go
│   │   ├── resolver.go
│   │   ├── search.go          # includes inline ScorerWorker logic
│   │   ├── download.go
│   │   ├── monitor.go
│   │   ├── mover.go
│   │   ├── scanner.go
│   │   └── local_watcher.go
│   ├── matcher/
│   │   ├── normalize.go          # string normalization
│   │   ├── score.go              # confidence scoring
│   │   └── template.go           # directory template engine
│   ├── clients/
│   │   ├── stashdb/              # StashDB GraphQL client
│   │   ├── stashapp/             # StashApp GraphQL client
│   │   ├── prowlarr/             # Prowlarr REST client
│   │   └── sabnzbd/              # SABnzbd REST client
│   └── models/                   # shared domain types
├── web/                          # React frontend source
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── hooks/
│   │   └── api/                  # typed API client
│   ├── package.json
│   └── vite.config.ts
├── scripts/
│   └── tampermonkey/
│       └── stasharr.user.js      # Tampermonkey userscript
├── docker/
│   ├── api.Dockerfile
│   └── ui.Dockerfile
├── docker-compose.yml
├── docker-compose.dev.yml
├── dev.env.example               # committed example, no real values
├── .env.example
└── sqlc.yaml
```

---

## External API Dependencies

### StashDB GraphQL
- Endpoint: `https://stashdb.org/graphql`
- Auth: `ApiKey` header
- Used for: resolving scene/performer/studio URLs to structured metadata
- Rate limit strategy: token bucket, max 5 req/sec, exponential backoff on 429

### StashApp GraphQL
- Endpoint: user-configured per instance
- Auth: `ApiKey` header
- Used for: duplicate scene detection, triggering post-scan
- Multiple instances supported via indexed config

### Prowlarr REST API
- Endpoint: user-configured
- Auth: `X-Api-Key` header
- Used for: searching indexers by query string, returning NZB results

### SABnzbd REST API
- Endpoint: user-configured
- Auth: `apikey` query param
- Used for: submitting NZB jobs, polling queue status

---

## Security Boundaries

- All API endpoints require `X-Api-Key` header matching `STASHARR_SECRET_KEY` env var
- The secret key is set by the user in their `.env` file — never auto-generated or stored in DB
- No external ingress required — designed for local network only
- CORS on the API is locked to the UI container origin in production
- In dev mode, CORS is open to `localhost:*`
