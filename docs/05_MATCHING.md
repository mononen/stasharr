# Stasharr — Matching & Confidence Scoring

## Overview

The scorer compares a resolved StashDB scene (the "needle") against a list of Prowlarr NZB results (the "haystack"). Each result receives a confidence score from 0–100. The score is the sum of per-field match scores. The highest-scoring result drives the auto-approve vs. review decision.

All scoring logic lives in `internal/matcher/`.

---

## Scoring Fields

| Field | Max Points | Match Type |
|---|---|---|
| Title | 40 | Fuzzy (normalized Levenshtein similarity) |
| Studio | 20 | Exact after normalization |
| Release Date | 20 | Exact calendar date |
| Duration | 15 | Within ±60 seconds |
| Performer | 5 | At least one exact match after normalization |
| **Total** | **100** | |

---

## Thresholds

| Score Range | Action |
|---|---|
| ≥ `auto_threshold` (default 85) | Auto-approve, proceed to download |
| ≥ `review_threshold` (default 50) | Sent to review queue |
| < `review_threshold` | `search_failed` — no usable candidates |

Both thresholds are user-configurable in the config table. Setting `auto_threshold` to 100 disables auto-approval entirely (all matches require human review). Setting it to 0 approves everything.

---

## Normalization

Normalization is applied to both the StashDB metadata fields and the extracted fields from the NZB release title before any comparison. Normalization is deterministic — the same input always produces the same output.

### String Normalization Pipeline (`normalize.NormalizeString`)

Applied in this order:

1. **Unicode normalization** — NFD decompose, strip combining characters (removes diacritics: `é` → `e`, `ñ` → `n`)
2. **Lowercase**
3. **Common substitutions:**
   - `&` → `and`
   - `+` → `and`
   - `@` → `at`
   - Digits spelled out for single digits: `1` → `one`, `2` → `two`, ... `9` → `nine` *(optional, configurable)*
4. **Punctuation stripping** — remove all characters not in `[a-z0-9 ]`
5. **Whitespace collapse** — multiple spaces → single space, trim
6. **Common filler removal** — strip known NZB suffixes/prefixes that carry no semantic meaning:

```go
var fillerPatterns = []string{
    `\b(hd|sd|fhd|uhd)\b`,
    `\b(1080p|720p|480p|2160p|4k)\b`,
    `\b(x264|x265|h264|h265|hevc|avc)\b`,
    `\b(aac|mp3|ac3|dts|flac)\b`,
    `\b(web|webrip|webdl|web-dl|bluray|bdrip)\b`,
    `\b(xxx|adult|18)\b`,
    `\b(complete|full|scene|clip|part\s?\d+)\b`,
    `\[\w+\]`,        // [GroupName]
    `-\w+$`,          // trailing -GROUPNAME
}
```

### Date Normalization (`normalize.NormalizeDate`)

Extracts a `time.Time` from common NZB date patterns:
- `YYYY-MM-DD`
- `YYYY.MM.DD`
- `DD.MM.YYYY`
- `MMDDYYYY`
- `Month DD YYYY` (e.g. `March 15 2024`)

Returns `nil` if no parseable date is found.

### Duration Extraction (`normalize.ExtractDuration`)

Extracts duration from NZB title patterns:
- `Xmin`, `Xm`, `X:XX`, `XX:XX` (MM:SS or HH:MM)
- Returns seconds as `int`, or `0` if not found

Duration in the NZB title is uncommon. This field contributes 15 points when available and 0 when not — it never penalizes a match for absence.

---

## Title Scoring

The title comparison uses normalized Levenshtein similarity:

```
similarity = 1 - (levenshtein_distance / max(len(a), len(b)))
```

Score mapping:
- similarity ≥ 0.95 → 40 points (near-exact)
- similarity ≥ 0.85 → 30 points (strong match)
- similarity ≥ 0.70 → 15 points (possible match)
- similarity < 0.70 → 0 points

The threshold for "fuzzy match qualifies" is 0.70. Anything below is treated as a title miss. This is intentionally permissive since NZB titles routinely abbreviate, truncate, or reorder words.

**Title extraction from NZB release title:**

NZB release titles follow rough patterns like:
```
Studio.Name.Scene.Title.XXX.1080p.WEB-GROUP
Studio-Name_Scene_Title_2024_1080p
```

The scorer attempts to strip the studio name (if known) and quality/group suffixes to isolate the scene title portion. This extraction is best-effort — if extraction is uncertain, the full normalized title is used as the comparison input.

---

## Studio Scoring

After normalization, the StashDB studio name is compared against:
1. The studio name as extracted from the NZB title (first dot-separated or dash-separated segment)
2. The studio alias table (user-managed canonical name mappings)

Match is binary: 20 points or 0. No partial credit.

The studio alias table allows users to map e.g. `"TeamSkeet"` → `"team skeet"` so that variations in how studios name themselves in NZB releases are handled gracefully.

---

## Date Scoring

Both the StashDB `release_date` and the NZB extracted date are normalized to `YYYY-MM-DD`. An exact match on the calendar date awards 20 points.

No fuzzy date matching (a one-day-off date is 0 points, not 10). Dates in NZB titles are typically accurate when present.

---

## Duration Scoring

If a duration is extractable from the NZB title, it is compared to the StashDB `duration_seconds`. A delta of ≤ 60 seconds awards 15 points, otherwise 0.

If no duration is found in the NZB title, this field contributes 0 to the score but does not penalize.

---

## Performer Scoring

At least one performer name from StashDB must appear (after normalization) as a substring of the normalized NZB title to score 5 points.

This is intentionally weak (5 points, substring match) because performers are inconsistently named in NZB releases and should not drive the match decision. It exists as a tiebreaker.

---

## `score_breakdown` Output

Every scored result persists a `score_breakdown` JSONB to `search_results`:

```json
{
  "title": {
    "score":      40,
    "max":        40,
    "matched":    true,
    "similarity": 0.97,
    "needle":     "scene title name",
    "haystack":   "scene title name"
  },
  "studio": {
    "score":   20,
    "max":     20,
    "matched": true,
    "needle":  "studio name",
    "haystack": "studio name"
  },
  "date": {
    "score":   20,
    "max":     20,
    "matched": true,
    "needle":  "2024-03-15",
    "haystack": "2024-03-15"
  },
  "duration": {
    "score":         15,
    "max":           15,
    "matched":       true,
    "delta_seconds": 12,
    "needle":        2847,
    "haystack":      2859
  },
  "performer": {
    "score":   0,
    "max":     5,
    "matched": false,
    "needle":  ["performer a", "performer b"],
    "haystack": "performer c"
  }
}
```

This breakdown is displayed in the review queue UI, explaining to the user exactly why a match scored the way it did.

---

## Review Queue Behavior

When a job enters `awaiting_review`, the UI displays:

1. The StashDB scene metadata (title, studio, date, duration, performers, cover art)
2. All search results sorted by `confidence_score` descending
3. For each result: release title, indexer, size, publish date, confidence score, and expandable `score_breakdown`
4. An "Approve" button per result
5. A "Skip / Mark No Match" button to set the job to `search_failed` manually

The top result is visually highlighted. If the top result's score is above `review_threshold` but below `auto_threshold`, it is considered "likely correct" and visually differentiated from lower-scoring results.

Jobs in the review queue sit indefinitely — there is no expiry. They will appear in the UI until actioned or manually cancelled.

---

## Tuning Guide (for documentation / user-facing help)

| If you're seeing... | Try... |
|---|---|
| Good matches being sent to review | Lower `auto_threshold` |
| Bad matches being auto-approved | Raise `auto_threshold` |
| Too many `search_failed` (no candidates) | Lower `review_threshold` |
| Wrong studios matching | Add studio aliases |
| Titles not matching due to encoding | The normalizer handles Unicode; check if the release group uses unusual transliterations |
