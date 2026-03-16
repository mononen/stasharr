# Stasharr — Directory Template Engine

## Overview

The directory template engine converts a user-defined template string and a resolved scene's metadata into an absolute filesystem path. It runs inside MoveWorker immediately before the file is relocated.

The engine lives in `internal/matcher/template.go`.

---

## Tokens

All tokens are wrapped in curly braces: `{token_name}`.

| Token | Source | Example Output |
|---|---|---|
| `{title}` | Scene title | `My Scene Title` |
| `{title_slug}` | URL-safe title | `my-scene-title` |
| `{studio}` | Studio name | `Studio Name` |
| `{studio_slug}` | URL-safe studio | `studio-name` |
| `{performer}` | First performer (lastname-firstname sort) | `Doe Jane` |
| `{performers}` | All performers, comma-separated, max 3 | `Jane Doe, John Smith` |
| `{performers_slug}` | Slug version of `{performers}` | `jane-doe-john-smith` |
| `{year}` | 4-digit year from release date | `2024` |
| `{date}` | Full release date | `2024-03-15` |
| `{month}` | 2-digit month | `03` |
| `{resolution}` | Detected from filename post-download | `1080p` |
| `{ext}` | File extension (no leading dot) | `mp4` |

---

## Performer Sorting

`{performer}` returns a single performer name, sorted by a "lastname-firstname" heuristic: the last space-delimited token of the name is treated as the surname and sorted first for directory ordering purposes. The name is **displayed** as-is (not reversed), but the *sort order* for which performer appears when there are multiple uses the surname sort.

Example: performers `["Jane Doe", "Alice Smith"]` → sorted `["Doe Jane" order → display "Jane Doe", "Smith Alice" order → display "Alice Smith"]` → `{performer}` = `Jane Doe` (alphabetically first by surname).

`{performers}` returns up to `directory.performer_max` (default 3) performers in the same sort order, comma-separated. If the scene has more performers than the limit, the string is truncated at the limit with no ellipsis — it just stops at 3 names.

---

## Missing Field Fallback

If a metadata field required by a token is `null` or empty, the `directory.missing_field_value` config value is substituted (default: `1unknown`).

The value `1unknown` is intentional: it sorts before alphabetic characters in most filesystem directory listings, making incomplete metadata visible at the top of a directory rather than buried.

Examples:
- Scene has no studio → `{studio}` → `1unknown`
- Scene has no release date → `{year}` → `1unknown`
- Scene has no performers → `{performer}` → `1unknown`

The fallback value is user-configurable but must pass filesystem character validation (see below).

---

## Resolution Detection

`{resolution}` is extracted from the downloaded filename (not from StashDB metadata) using a pattern match against the filename after SABnzbd completes:

```go
var resolutionPatterns = []struct {
    pattern *regexp.Regexp
    label   string
}{
    {regexp.MustCompile(`(?i)\b(2160p|4k|uhd)\b`), "2160p"},
    {regexp.MustCompile(`(?i)\b1080p\b`),           "1080p"},
    {regexp.MustCompile(`(?i)\b720p\b`),            "720p"},
    {regexp.MustCompile(`(?i)\b480p\b`),            "480p"},
    {regexp.MustCompile(`(?i)\b(sd)\b`),            "SD"},
}
```

If no resolution is detected, falls back to `directory.missing_field_value`.

---

## Filesystem Character Sanitization

Applied to every token value **after** substitution and **before** path assembly. Ensures the resulting path is valid on Linux/ext4 (the container filesystem).

Rules:
1. Replace `/` with `-` (prevents unintended path segments within a token value)
2. Replace null bytes with empty string
3. Replace `\` with `-`
4. Strip leading/trailing whitespace from each path segment
5. Strip leading dots from path segments (prevents hidden directories)
6. Replace `:` with `-`
7. Collapse multiple consecutive `-` into one

The template is split on `/` **first** (to identify path segments from literal slashes in the template), then sanitization is applied to each segment's token substitutions independently. This means a `/` in the template is always a path separator, never a filename character.

---

## Path Assembly

The template is resolved as a **relative path**. A base directory is prepended at move time. The base directory is the filesystem root that Stash is configured to scan.

Example:
- Base: `/data/stash`
- Template: `{studio}/{year}/{title} ({year}).{ext}`
- Resolved: `/data/stash/Studio Name/2024/My Scene Title (2024).mp4`

The base directory is derived from the Stash instance config — specifically, the library path configured in Stash. This is a required field on the Stash instance configuration form.

---

## Example Templates

```
# Default — studio > year > title
{studio}/{year}/{title} ({year}).{ext}
→ Studio Name/2024/My Scene Title (2024).mp4

# Performer-first library
{performer}/{studio}/{title} ({date}).{ext}
→ Jane Doe/Studio Name/My Scene Title (2024-03-15).mp4

# Flat — all in one directory with rich filename
{studio} - {title} - {performers} ({year}) [{resolution}].{ext}
→ Studio Name - My Scene Title - Jane Doe, John Smith (2024) [1080p].mp4

# Date-based organization
{year}/{month}/{studio}/{title}.{ext}
→ 2024/03/Studio Name/My Scene Title.mp4
```

---

## Template Validation

The config UI validates the template on input:
1. All `{tokens}` are recognized (unknown tokens are flagged as warnings, not errors)
2. At least one non-token character exists in the filename portion (the last segment after the final `/`)
3. `{ext}` is present in the last segment (required — a file without an extension will not scan correctly in Stash)
4. The template does not start or end with `/`

A **live preview** is shown in the template builder using a synthetic scene with placeholder values:
```
Studio: Example Studio
Title: Example Scene Title
Performers: Jane Doe, John Smith
Date: 2024-03-15
Duration: 47:27
Resolution: 1080p
Extension: mp4
```
