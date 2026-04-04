# Stasharr — Project Overview

## What Is Stasharr?

Stasharr is a self-hosted, containerized pipeline tool that bridges StashDB (the community scene metadata database) with StashApp (the self-hosted media manager) via an *arr-style automated download workflow. It allows a user to browse StashDB in their browser, submit scenes, performers, or studios for acquisition, and have the full pipeline — search, download, file organization, and Stash import — run automatically.

It is not trying to replace Whisparr. It is purpose-built for the StashDB/StashApp ecosystem, using Prowlarr's indexer infrastructure and SABnzbd as the download client.

---

## Goals

- Submit scenes, performers, and studios from StashDB via a Tampermonkey browser script
- Resolve submitted URLs to structured StashDB metadata
- Search configured Prowlarr indexers for matching NZB releases
- Auto-approve high-confidence matches; surface low-confidence matches for manual review
- Submit approved results to SABnzbd for download
- Move completed files into a user-configured directory structure
- Trigger StashApp to scan and import the new file
- Provide a clean React web UI for configuration and monitoring

---

## Non-Goals (v1)

- Managing files after initial import (no upgrade logic, no deletion)
- Supporting download clients other than SABnzbd (designed for future extension)
- Supporting torrent clients in v1
- Multi-user authentication (single-user, self-hosted)
- Cloud/hosted deployment

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend API | Go, Fiber v2 |
| Frontend | React 19 (Vite 8, TypeScript 5.9) |
| Database | PostgreSQL 16 |
| Job Queue | Postgres-backed (`SELECT ... FOR UPDATE SKIP LOCKED`) |
| Browser Integration | Tampermonkey userscript |
| Indexer Integration | Prowlarr API |
| Download Client | SABnzbd API |
| Media Manager | StashApp GraphQL API |
| Metadata Source | StashDB GraphQL API |

---

## Repository

`github.com/mononen/stasharr`

Go module path: `github.com/mononen/stasharr`

---

## Container Summary

| Container | Description |
|---|---|
| `stasharr-api` | Go/Fiber backend. Runs all workers, exposes REST + SSE API |
| `stasharr-ui` | React frontend served via nginx |
| `postgres` | PostgreSQL database (user-provided or compose-managed) |

The user is expected to already have Prowlarr, SABnzbd, and StashApp running in their stack. Stasharr connects to these as external services.

---

## High-Level Data Flow

```
[StashDB in Browser]
        |
        | (Tampermonkey script POSTs URL + type)
        ▼
[stasharr-api] ──── PostgreSQL (job queue + config + state)
        |
        ├── ResolverWorker      → StashDB GraphQL API
        ├── SearchWorker        → Prowlarr API
        ├── ScorerWorker        → internal (confidence scoring, runs inline in SearchWorker)
        ├── DownloadWorker      → SABnzbd API
        ├── MonitorWorker       → SABnzbd API (poll)
        ├── MoveWorker          → filesystem
        ├── ScanWorker          → StashApp GraphQL API
        └── LocalWatcherWorker  → filesystem (local file import events)
        
[stasharr-ui] ──── REST + SSE ──── [stasharr-api]
```

---

## Future Roadmap (Out of Scope for v1)

- Additional download clients (qBittorrent, NZBGet)
- Multi-Stash instance management and file distribution
- Swiss-army file manager for Stash (reorganize existing libraries)
- Upgrade logic (replace existing files with better quality)
- Tag/performer-based automation rules ("auto-download all scenes from Studio X")
