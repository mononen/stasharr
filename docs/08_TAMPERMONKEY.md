# Stasharr — Tampermonkey Script

## Overview

The Tampermonkey userscript (`scripts/tampermonkey/stasharr.user.js`) is a first-class deliverable. It runs in the user's browser on `stashdb.org` pages and injects a submission panel that sends scene, performer, and studio URLs to the Stasharr API.

---

## Script Metadata Block

```javascript
// ==UserScript==
// @name         Stasharr
// @namespace    github.com/mononen/stasharr
// @version      0.1.0
// @description  Send StashDB content to Stasharr for automated acquisition
// @author       mononen
// @match        https://stashdb.org/*
// @grant        GM_xmlhttpRequest
// @grant        GM_getValue
// @grant        GM_setValue
// @grant        GM_registerMenuCommand
// @connect      localhost
// @connect      *
// @run-at       document-idle
// ==/UserScript==
```

`GM_xmlhttpRequest` is required instead of `fetch` because the script makes cross-origin requests to the local Stasharr container. `@connect *` allows the user to configure any host (including non-localhost deployments).

---

## User Configuration

The script stores configuration in Tampermonkey's persistent storage (`GM_getValue`/`GM_setValue`). Configuration is accessible via the Tampermonkey menu command ("Stasharr Settings").

| Key | Description | Default |
|---|---|---|
| `stasharr_url` | Base URL of the Stasharr API | `http://localhost:8080` |
| `stasharr_api_key` | The `STASHARR_SECRET_KEY` value | *(empty — must be configured)* |

If either value is empty, the script renders a warning state instead of the submit panel.

---

## Supported URL Patterns

The script detects the current page type and extracts the entity type and StashDB ID from the URL:

| URL Pattern | Detected Type | Example |
|---|---|---|
| `/scenes/:id` | `scene` | `https://stashdb.org/scenes/abc-123-def` |
| `/performers/:id` | `performer` | `https://stashdb.org/performers/abc-123-def` |
| `/studios/:id` | `studio` | `https://stashdb.org/studios/abc-123-def` |

All other URL patterns (`/`, `/search`, `/tags`, etc.) render the panel in a neutral "unsupported page" state.

---

## Injected Panel

The script injects a fixed-position floating panel into the bottom-right corner of the page. It is non-intrusive and collapsible.

### Panel States

**Unconfigured:**
```
┌────────────────────────────┐
│ 🎬 Stasharr                │
│ ⚠ Not configured           │
│ Open Tampermonkey settings │
└────────────────────────────┘
```

**Unsupported page:**
```
┌────────────────────────────┐
│ 🎬 Stasharr           [−]  │
│ Browse to a scene,         │
│ performer, or studio       │
└────────────────────────────┘
```

**Ready (scene page):**
```
┌────────────────────────────┐
│ 🎬 Stasharr           [−]  │
│ Scene: "Scene Title"       │
│                            │
│ [  Send to Stasharr  ]     │
└────────────────────────────┘
```

**Ready (performer page):**
```
┌────────────────────────────┐
│ 🎬 Stasharr           [−]  │
│ Performer: "Jane Doe"      │
│ (all scenes will queue)    │
│                            │
│ [  Send to Stasharr  ]     │
└────────────────────────────┘
```

**Submitting:**
```
┌────────────────────────────┐
│ 🎬 Stasharr           [−]  │
│ ⏳ Submitting...           │
└────────────────────────────┘
```

**Success:**
```
┌────────────────────────────┐
│ 🎬 Stasharr           [−]  │
│ ✓ Queued!                  │
│ Job ID: abc-123            │
│ [View in Stasharr →]       │
└────────────────────────────┘
```

**Error:**
```
┌────────────────────────────┐
│ 🎬 Stasharr           [−]  │
│ ✗ Failed: <error message>  │
│ [Retry]                    │
└────────────────────────────┘
```

---

## Panel Injection

StashDB is a React SPA. The DOM mutates on navigation without full page reloads. The script handles this by:

1. Injecting the panel container div once on initial load
2. Using a `MutationObserver` on `document.body` to detect URL changes (React Router pushState)
3. On URL change: re-evaluate the current page type and update the panel state

The panel itself is a plain DOM element (no React, no framework dependency). Styles are injected as a `<style>` tag scoped to the panel's root class to avoid conflicts with StashDB's CSS.

---

## API Submission

On button click, the script POSTs to `POST /api/v1/jobs`:

```javascript
GM_xmlhttpRequest({
  method: 'POST',
  url: `${config.url}/api/v1/jobs`,
  headers: {
    'Content-Type': 'application/json',
    'X-Api-Key': config.apiKey
  },
  data: JSON.stringify({
    url: window.location.href,
    type: detectedType  // 'scene' | 'performer' | 'studio'
  }),
  onload: (response) => {
    if (response.status === 202) {
      const body = JSON.parse(response.responseText);
      showSuccess(body.job_id);
    } else {
      showError(parseError(response));
    }
  },
  onerror: () => showError('Could not reach Stasharr. Is it running?')
});
```

---

## "View in Stasharr" Link

On successful submission, the panel shows a link to the job in the Stasharr UI. The URL is constructed as:

- Scene job: `{stasharr_url}/queue?job={job_id}`
- Batch job (performer/studio): `{stasharr_url}/batches?batch={batch_job_id}`

---

## Entity Name Detection

The panel displays a friendly name (e.g., `"Scene Title"` or `"Jane Doe"`) instead of just the URL. Since StashDB is a React app, the entity name is read from the page's `<h1>` or `<title>` tag at submission time. This is best-effort — if reading fails, the URL is displayed instead. The name shown in the panel is cosmetic only; the actual submission uses the URL, and authoritative metadata comes from the StashDB API server-side.

---

## Collapsible State

The `[−]` button in the panel header collapses the panel to just the header bar. Collapse state is persisted in `GM_setValue` so it survives page navigation.

---

## Script Distribution

The script file is committed to the repository at `scripts/tampermonkey/stasharr.user.js`. Installation instructions in the README point users to either:

1. Copy-paste the raw GitHub URL into Tampermonkey's "Install from URL" dialog
2. Copy the script contents directly into a new Tampermonkey script

There is no auto-update mechanism in v1. Users re-install manually on new releases. A `@updateURL` and `@downloadURL` pointing to the raw GitHub file can be added in a future release to enable Tampermonkey's native auto-update.
