# Stasharr — Configuration

## Configuration Model

Configuration is split into two tiers:

| Tier | Storage | Purpose |
|---|---|---|
| **Secrets / Infra** | Environment variables (`.env` file) | Database DSN, API auth key — values that must exist before the app can boot |
| **App Config** | PostgreSQL `config` table | All application behaviour, tunable at runtime via the UI |

No application behaviour is controlled by environment variables except the two secrets below. Everything else is in the database.

---

## Environment Variables

### Required

```bash
# PostgreSQL connection string
STASHARR_DB_DSN=postgres://stasharr:password@postgres:5432/stasharr?sslmode=disable

# Shared secret for API authentication (used by the UI and Tampermonkey script)
# Generate with: openssl rand -hex 32
STASHARR_SECRET_KEY=your-secret-key-here
```

### Optional

```bash
# API server port (default: 8080)
STASHARR_PORT=8080

# Log level: debug, info, warn, error (default: info)
STASHARR_LOG_LEVEL=info

# Dev mode: enables open CORS, verbose logging, and loads dev.env defaults (default: false)
STASHARR_DEV=false
```

---

## App Config Keys (stored in `config` table)

### Prowlarr

| Key | Default | Description |
|---|---|---|
| `prowlarr.url` | *(required)* | Prowlarr base URL e.g. `http://prowlarr:9696` |
| `prowlarr.api_key` | *(required)* | Prowlarr API key |
| `prowlarr.search_limit` | `10` | Max results per indexer per search |

### SABnzbd

| Key | Default | Description |
|---|---|---|
| `sabnzbd.url` | *(required)* | SABnzbd base URL e.g. `http://sabnzbd:8080` |
| `sabnzbd.api_key` | *(required)* | SABnzbd API key |
| `sabnzbd.category` | `stasharr` | SABnzbd category to assign downloads |
| `sabnzbd.complete_dir` | *(required)* | Absolute path to SABnzbd complete directory (must be accessible to the stasharr container) |

### StashDB

| Key | Default | Description |
|---|---|---|
| `stashdb.api_key` | *(required)* | StashDB API key |

### Matching

| Key | Default | Description |
|---|---|---|
| `matching.auto_threshold` | `85` | Confidence score at or above which a result is auto-approved |
| `matching.review_threshold` | `50` | Confidence score below which a search is considered failed |

### Pipeline

| Key | Default | Description |
|---|---|---|
| `pipeline.worker_resolver_pool` | `5` | Number of concurrent resolver goroutines |
| `pipeline.worker_search_pool` | `5` | Number of concurrent search goroutines |
| `pipeline.worker_download_pool` | `3` | Number of concurrent download submission goroutines |
| `pipeline.worker_move_pool` | `3` | Number of concurrent file move goroutines |
| `pipeline.worker_scan_pool` | `3` | Number of concurrent scan trigger goroutines |
| `pipeline.monitor_poll_interval` | `30` | Seconds between SABnzbd queue polls |
| `pipeline.stashdb_rate_limit` | `5` | Max StashDB requests per second |
| `pipeline.batch_auto_threshold` | `40` | Scenes before batch confirmation is required |
| `pipeline.max_retries_resolver` | `3` | Max retry attempts for resolve failures |
| `pipeline.max_retries_search` | `2` | Max retry attempts for search failures |
| `pipeline.max_retries_move` | `3` | Max retry attempts for move failures |
| `pipeline.max_retries_scan` | `5` | Max retry attempts for scan failures |

### Directory

| Key | Default | Description |
|---|---|---|
| `directory.template` | `{studio}/{year}/{title} ({year}).{ext}` | Directory and filename template |
| `directory.performer_max` | `3` | Max performers in `{performers}` token before truncation |
| `directory.missing_field_value` | `1unknown` | Substituted when a metadata field is null |

---

## Config Load Sequence

1. On startup, the application connects to Postgres using `STASHARR_DB_DSN`
2. All `config` table rows are loaded into a `map[string]string` in memory
3. Required keys are validated — missing required keys cause startup to fail with a clear error message listing what's missing
4. On any `PUT /api/v1/config` write, the in-memory config is refreshed atomically

Workers read config from the in-memory map. Pool size changes require a restart; all other config changes take effect on the next job processed.

---

## First-Run Setup

On first boot with a fresh database:

1. Migrations run automatically
2. `config` table is seeded with all defaults
3. The UI detects missing required config and redirects to `/config` with a setup checklist
4. Required fields are highlighted until populated
5. A "Test Connection" button verifies each external service before allowing the user to proceed

---

## Dev Configuration

`docker-compose.dev.yml` extends the base compose file with:
- `STASHARR_DEV=true`
- A volume mount for hot-reload (Air for Go, Vite HMR for React)
- Postgres with a preconfigured test DB

`dev.env.example` is committed to the repository with placeholder values pointing to Docker Compose service names:

```bash
STASHARR_DB_DSN=postgres://stasharr:stasharr@postgres:5432/stasharr?sslmode=disable
STASHARR_SECRET_KEY=dev-secret-key-do-not-use-in-production
STASHARR_PORT=8080
STASHARR_LOG_LEVEL=debug
STASHARR_DEV=true
```

`dev.env.example` is committed. `dev.env` and `.env` are in `.gitignore`. The developer copies `dev.env.example` → `dev.env` and populates real values.

When `STASHARR_DEV=true`, the config table is seeded with additional dev defaults:
```
prowlarr.url = http://prowlarr:9696
sabnzbd.url  = http://sabnzbd:8080
sabnzbd.complete_dir = /downloads/complete
```

These dev defaults are skipped if the key already has a non-empty value in the DB.
