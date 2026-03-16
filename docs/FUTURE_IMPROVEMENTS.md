**Architecture & Pipeline**
- Webhook-based download completion from SABnzbd (replacing the polling monitor — the interface is already designed for this)
- Additional download clients: qBittorrent, NZBGet — the SABnzbd client is the template
- Torrent indexer support alongside Usenet
- Distributed advisory lock on MonitorWorker to support API replica scaling in K8s
- Auto-update for the Tampermonkey script via `@updateURL` and `@downloadURL` pointing to raw GitHub

**Matching & Scoring**
- User-tunable per-field weights (currently hardcoded 40/20/20/15/5)
- Optional digit-to-word normalization in the string normalizer (flag exists in the design, marked configurable but not wired)
- Batch approve in the review queue UI (deliberately cut from v1)
- Multi-scene selection in the Tampermonkey panel (cut in favor of performer/studio submission)

**File Management**
- Upgrade logic: replace existing files with higher quality releases (this was explicitly cut as fire-and-forget, but noted as a natural next step)
- Post-import metadata writeback: use StashDB data to populate performer, studio, and tag metadata in Stash via its GraphQL API after scanning — this was flagged as something that would make the tool significantly more valuable
- Full swiss-army file manager mode: query multiple Stash instances, reorganize and redistribute existing library files using the template engine

**Multi-Instance & Scale**
- Multi-Stash instance management UI (the DB schema already supports it via indexed instances, but the UI only exposes one in v1)
- Tag and performer-based automation rules: "auto-download all new scenes from Studio X" without browser interaction
- Priority lanes in the job queue (batch jobs vs. single scene submissions currently share the same queue)

**Automation**
- Studio and performer watchlists: monitor StashDB for new scene additions and auto-submit without user interaction
- Configurable per-studio or per-performer quality profiles (resolution preference, size limits)

**Ops & Observability**
- Prometheus metrics endpoint for worker throughput, queue depth, match confidence distribution
- Retention policy for completed job records and job_events (the table will grow indefinitely in v1)
- Backup/restore tooling for the config and alias tables specifically