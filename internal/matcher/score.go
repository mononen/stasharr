package matcher

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/agnivade/levenshtein"
	"github.com/mononen/stasharr/internal/models"
)

// ProwlarrResult represents a single result returned from a Prowlarr search.
// Used by the search worker — fields mirror the Prowlarr /api/v1/search response.
type ProwlarrResult struct {
	Title       string
	SizeBytes   int64
	PublishDate *time.Time
	IndexerName string
	DownloadURL string
	NzbID       string
	InfoURL     string
}

// --- Score breakdown types ---
// Each type serialises cleanly to the JSON shape stored in search_results.score_breakdown.

type TitleBreakdown struct {
	Score      int     `json:"score"`
	Max        int     `json:"max"`
	Matched    bool    `json:"matched"`
	Similarity float64 `json:"similarity"`
	Needle     string  `json:"needle"`
	Haystack   string  `json:"haystack"`
}

type StudioBreakdown struct {
	Score    int    `json:"score"`
	Max      int    `json:"max"`
	Matched  bool   `json:"matched"`
	Needle   string `json:"needle"`
	Haystack string `json:"haystack"`
}

type DateBreakdown struct {
	Score    int    `json:"score"`
	Max      int    `json:"max"`
	Matched  bool   `json:"matched"`
	Needle   string `json:"needle"`
	Haystack string `json:"haystack"`
}

type DurationBreakdown struct {
	Score        int  `json:"score"`
	Max          int  `json:"max"`
	Matched      bool `json:"matched"`
	DeltaSeconds int  `json:"delta_seconds"`
	Needle       int  `json:"needle"`
	Haystack     int  `json:"haystack"`
}

type PerformerBreakdown struct {
	Score    int      `json:"score"`
	Max      int      `json:"max"`
	Matched  bool     `json:"matched"`
	Needle   []string `json:"needle"`
	Haystack string   `json:"haystack"`
}

// ScoreBreakdown holds per-field match scores for a single Prowlarr result.
// It serialises directly to the JSONB stored in search_results.score_breakdown.
type ScoreBreakdown struct {
	Title     TitleBreakdown     `json:"title"`
	Studio    StudioBreakdown    `json:"studio"`
	Date      DateBreakdown      `json:"date"`
	Duration  DurationBreakdown  `json:"duration"`
	Performer PerformerBreakdown `json:"performer"`
}

// Total returns the sum of all field scores.
func (b ScoreBreakdown) Total() int {
	return b.Title.Score + b.Studio.Score + b.Date.Score + b.Duration.Score + b.Performer.Score
}

// ScoredResult pairs a ProwlarrResult with its computed score and breakdown.
// The search worker persists this to search_results.
type ScoredResult struct {
	Result    ProwlarrResult
	Score     int
	Breakdown ScoreBreakdown
}

// ScoreResult scores a single Prowlarr result against a resolved scene.
// aliases is a map of NormalizeString(alias) → NormalizeString(canonical) used for studio matching.
func ScoreResult(scene models.Scene, result ProwlarrResult, aliases map[string]string) ScoreBreakdown {
	var bd ScoreBreakdown

	// Unmarshal performers from the scene's JSONB column.
	var performers []models.Performer
	if len(scene.Performers) > 0 {
		_ = json.Unmarshal(scene.Performers, &performers)
	}

	normalizedResultTitle := NormalizeString(result.Title)

	// ── Title (40 pts) ───────────────────────────────────────────────────────
	bd.Title.Max = 40

	needleTitle := NormalizeString(scene.Title)
	haystackTitle := normalizedResultTitle

	// Best-effort: strip the known studio prefix from the NZB title to isolate the scene title.
	if scene.StudioName.Valid && scene.StudioName.String != "" {
		normalizedStudio := NormalizeString(scene.StudioName.String)
		if strings.HasPrefix(haystackTitle, normalizedStudio+" ") {
			haystackTitle = strings.TrimSpace(strings.TrimPrefix(haystackTitle, normalizedStudio+" "))
		}
	}

	bd.Title.Needle = needleTitle
	bd.Title.Haystack = haystackTitle

	if len(needleTitle) == 0 && len(haystackTitle) == 0 {
		bd.Title.Similarity = 1.0
	} else if len(needleTitle) == 0 || len(haystackTitle) == 0 {
		bd.Title.Similarity = 0
	} else if strings.Contains(haystackTitle, needleTitle) {
		// Needle found verbatim inside haystack — the NZB title contains the full
		// scene title plus metadata noise (date, duration, performers). Treat as
		// near-exact rather than penalising for the extra characters.
		bd.Title.Similarity = 1.0
	} else {
		dist := levenshtein.ComputeDistance(needleTitle, haystackTitle)
		maxLen := len([]rune(needleTitle))
		if h := len([]rune(haystackTitle)); h > maxLen {
			maxLen = h
		}
		bd.Title.Similarity = 1.0 - float64(dist)/float64(maxLen)
	}

	switch {
	case bd.Title.Similarity >= 0.95:
		bd.Title.Score = 40
		bd.Title.Matched = true
	case bd.Title.Similarity >= 0.85:
		bd.Title.Score = 30
		bd.Title.Matched = true
	case bd.Title.Similarity >= 0.70:
		bd.Title.Score = 15
		bd.Title.Matched = true
	default:
		bd.Title.Score = 0
		bd.Title.Matched = false
	}

	// ── Studio (20 pts) ──────────────────────────────────────────────────────
	bd.Studio.Max = 20

	if scene.StudioName.Valid && scene.StudioName.String != "" {
		needleStudio := NormalizeString(scene.StudioName.String)
		haystackStudio := extractStudioFromTitle(result.Title, aliases)

		bd.Studio.Needle = needleStudio
		bd.Studio.Haystack = haystackStudio

		if needleStudio == haystackStudio {
			bd.Studio.Score = 20
			bd.Studio.Matched = true
		}
	}

	// ── Date (20 pts) ────────────────────────────────────────────────────────
	bd.Date.Max = 20

	if scene.ReleaseDate.Valid {
		needleDate := scene.ReleaseDate.Time.Format("2006-01-02")
		bd.Date.Needle = needleDate

		if extracted := NormalizeDate(result.Title); extracted != nil {
			haystackDate := extracted.Format("2006-01-02")
			bd.Date.Haystack = haystackDate
			if needleDate == haystackDate {
				bd.Date.Score = 20
				bd.Date.Matched = true
			}
		}
	}

	// ── Duration (15 pts) ────────────────────────────────────────────────────
	// No duration in NZB title → 0 pts, no penalty.
	bd.Duration.Max = 15

	if scene.DurationSeconds.Valid {
		needleDuration := int(scene.DurationSeconds.Int32)
		bd.Duration.Needle = needleDuration

		haystackDuration := ExtractDuration(result.Title)
		if haystackDuration > 0 {
			bd.Duration.Haystack = haystackDuration
			delta := needleDuration - haystackDuration
			if delta < 0 {
				delta = -delta
			}
			bd.Duration.DeltaSeconds = delta
			if delta <= 60 {
				bd.Duration.Score = 15
				bd.Duration.Matched = true
			}
		}
	}

	// ── Performer (5 pts) ────────────────────────────────────────────────────
	bd.Performer.Max = 5
	bd.Performer.Haystack = normalizedResultTitle

	performerNames := make([]string, 0, len(performers))
	for _, p := range performers {
		performerNames = append(performerNames, NormalizeString(p.Name))
	}
	bd.Performer.Needle = performerNames

	for _, name := range performerNames {
		if name != "" && strings.Contains(normalizedResultTitle, name) {
			bd.Performer.Score = 5
			bd.Performer.Matched = true
			break
		}
	}

	return bd
}

// ScoreResults scores and sorts a slice of Prowlarr results against a scene.
// aliases maps NormalizeString(alias) → NormalizeString(canonical).
// Returns results sorted by total confidence score, highest first.
func ScoreResults(scene models.Scene, results []ProwlarrResult, aliases map[string]string) []ScoredResult {
	scored := make([]ScoredResult, 0, len(results))
	for _, r := range results {
		bd := ScoreResult(scene, r, aliases)
		scored = append(scored, ScoredResult{
			Result:    r,
			Score:     bd.Total(),
			Breakdown: bd,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored
}

// extractStudioFromTitle extracts the studio name from the first dot- or dash-separated
// segment of the NZB release title, normalises it, and resolves aliases.
func extractStudioFromTitle(title string, aliases map[string]string) string {
	seg := title
	if idx := strings.IndexAny(title, ".-_"); idx > 0 {
		seg = title[:idx]
	}
	return NormalizeStudio(seg, aliases)
}
