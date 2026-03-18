package matcher

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mononen/stasharr/internal/models"
)

func makeScene(title, studio, studioSlug, releaseDate string, durationSec int32, performers []models.Performer) models.Scene {
	s := models.Scene{
		ID:    uuid.New(),
		JobID: uuid.New(),
		Title: title,
	}
	if studio != "" {
		s.StudioName = pgtype.Text{String: studio, Valid: true}
	}
	if studioSlug != "" {
		s.StudioSlug = pgtype.Text{String: studioSlug, Valid: true}
	}
	if releaseDate != "" {
		t, err := time.Parse("2006-01-02", releaseDate)
		if err == nil {
			s.ReleaseDate = pgtype.Date{Time: t, Valid: true}
		}
	}
	if durationSec > 0 {
		s.DurationSeconds = pgtype.Int4{Int32: durationSec, Valid: true}
	}
	if len(performers) > 0 {
		b, _ := json.Marshal(performers)
		s.Performers = b
	}
	return s
}

func makeResult(title string) ProwlarrResult {
	return ProwlarrResult{
		Title:       title,
		IndexerName: "TestIndexer",
	}
}

// TestScoreResult_ExactMatch verifies that an NZB title containing studio, scene title,
// release date, duration, and a performer scores 95+.
//
// Title contains "scene" as a literal word so we use a title without it.
// Studio is single-word so extractStudioFromTitle picks it up cleanly.
// NZB format: Studio.Title.Performer.Date.Duration.Quality
func TestScoreResult_ExactMatch(t *testing.T) {
	performers := []models.Performer{{Name: "Jane Doe"}}
	// Title: "My Amazing Title" — no filler words, normalises to "my amazing title"
	// Studio: "BestStudios" — single word, normalises to "beststudios"
	scene := makeScene("My Amazing Title", "BestStudios", "best-studios", "2024-03-15", 2847, performers)

	// NZB: all five scoring fields encoded.
	// After NormalizeString: "beststudios my amazing title jane doe 2024 03 15 47 27"
	// After studio prefix strip: "my amazing title jane doe 2024 03 15 47 27"
	// Substring check: "my amazing title" IS in haystack → similarity = 1.0 → 40 pts
	nzbTitle := "BestStudios.My.Amazing.Title.Jane.Doe.2024-03-15.47:27.1080p.WEB"

	result := makeResult(nzbTitle)
	bd := ScoreResult(scene, result, nil)

	total := bd.Total()
	if total < 95 {
		t.Errorf(
			"exact-match scored %d, want >= 95\ntitle=%d(sim=%.2f) studio=%d date=%d performer=%d",
			total,
			bd.Title.Score, bd.Title.Similarity,
			bd.Studio.Score, bd.Date.Score, bd.Performer.Score,
		)
	}
	if !bd.Title.Matched {
		t.Errorf("title should be matched (similarity=%.2f haystack=%q needle=%q)",
			bd.Title.Similarity, bd.Title.Haystack, bd.Title.Needle)
	}
}

func TestScoreResult_TitleOnlyMatch(t *testing.T) {
	// Correct title, wrong studio, no date/duration.
	scene := makeScene("My Amazing Title", "CorrectStudio", "correct-studio", "", 0, nil)

	// NZB: wrong studio prefix, title present. After studio strip fails (wrong prefix),
	// haystack = "wrongstudio my amazing title". Substring check finds "my amazing title" → match.
	result := makeResult("WrongStudio.My.Amazing.Title.1080p.WEB")

	bd := ScoreResult(scene, result, nil)

	if !bd.Title.Matched {
		t.Errorf("title should be matched (similarity=%.2f haystack=%q)", bd.Title.Similarity, bd.Title.Haystack)
	}
	if bd.Studio.Matched {
		t.Errorf("studio should not match (needle=%q haystack=%q)", bd.Studio.Needle, bd.Studio.Haystack)
	}
	if bd.Date.Matched {
		t.Error("date should not match — none in NZB title")
	}
}

func TestScoreResult_NoMatch(t *testing.T) {
	scene := makeScene("My Amazing Title", "BestStudios", "best-studios", "2024-03-15", 2847, nil)
	// Entirely different title and studio.
	result := makeResult("OtherStudio.Completely.Different.Content.2019-01-01.720p")

	bd := ScoreResult(scene, result, nil)
	total := bd.Total()

	// "my amazing title" should not appear in "otherstudio completely different content 2019 01 01"
	// as a substring, and Levenshtein similarity should be low.
	if bd.Title.Score > 15 {
		t.Errorf("no-match title scored %d, want <= 15 (similarity=%.2f haystack=%q)",
			bd.Title.Score, bd.Title.Similarity, bd.Title.Haystack)
	}
	if total > 40 {
		t.Errorf("no-match total = %d, want <= 40", total)
	}
}

func TestScoreResult_TitleStudioDate(t *testing.T) {
	// Title + studio + date should score 90 (40+25+25) with no performers.
	scene := makeScene("My Amazing Title", "BestStudios", "best-studios", "2024-03-15", 2847, nil)
	result := makeResult("BestStudios.My.Amazing.Title.2024-03-15.1080p.WEB")

	bd := ScoreResult(scene, result, nil)

	total := bd.Title.Score + bd.Studio.Score + bd.Date.Score + bd.Performer.Score
	if total < 85 {
		t.Errorf("title+studio+date scored only %d, expected >= 85 (title=%d studio=%d date=%d)",
			total, bd.Title.Score, bd.Studio.Score, bd.Date.Score)
	}
}

func TestScoreResults_StudioAliasResolution(t *testing.T) {
	// "TeamSkeet" in NZB title maps via alias to "team skeet" → matches StashDB studio.
	scene := makeScene("Hot Title", "Team Skeet", "team-skeet", "", 0, nil)
	results := []ProwlarrResult{
		makeResult("TeamSkeet.Hot.Title.1080p"),
	}
	// alias: NormalizeString("TeamSkeet") → NormalizeString("Team Skeet")
	aliases := map[string]string{
		"teamskeet": "team skeet",
	}

	scored := ScoreResults(scene, results, aliases)
	if len(scored) == 0 {
		t.Fatal("expected at least one scored result")
	}

	bd := scored[0].Breakdown
	if !bd.Studio.Matched {
		t.Errorf("studio alias resolution failed: needle=%q haystack=%q",
			bd.Studio.Needle, bd.Studio.Haystack)
	}
	if bd.Studio.Score != 25 {
		t.Errorf("studio score = %d, want 25", bd.Studio.Score)
	}
}

func TestScoreResults_SortedHighestFirst(t *testing.T) {
	scene := makeScene("My Amazing Title", "BestStudios", "best-studios", "", 0, nil)
	results := []ProwlarrResult{
		makeResult("Unrelated.Garbage.Content.1080p"),
		makeResult("BestStudios.My.Amazing.Title.1080p"),
	}

	scored := ScoreResults(scene, results, nil)

	if len(scored) < 2 {
		t.Fatal("expected 2 scored results")
	}
	if scored[0].Score < scored[1].Score {
		t.Errorf("results not sorted descending: scored[0]=%d scored[1]=%d",
			scored[0].Score, scored[1].Score)
	}
}

func TestScoreBreakdown_JSONShape(t *testing.T) {
	scene := makeScene("My Amazing Title", "BestStudios", "best-studios", "2024-03-15", 2847,
		[]models.Performer{{Name: "Jane Doe"}})

	result := makeResult("BestStudios.My.Amazing.Title.Jane.Doe.2024-03-15.1080p")
	bd := ScoreResult(scene, result, nil)

	b, err := json.Marshal(bd)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// All top-level keys must be present (duration removed, resolution added).
	for _, key := range []string{"title", "studio", "date", "performer", "resolution"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in score_breakdown JSON", key)
		}
	}
	if _, ok := m["duration"]; ok {
		t.Error("unexpected key \"duration\" in score_breakdown JSON — duration was removed")
	}

	// Title sub-fields.
	titleMap, ok := m["title"].(map[string]interface{})
	if !ok {
		t.Fatal("title field is not an object")
	}
	for _, key := range []string{"score", "max", "matched", "similarity", "needle", "haystack"} {
		if _, ok := titleMap[key]; !ok {
			t.Errorf("missing key %q in title breakdown", key)
		}
	}

	// Resolution value field must be present.
	resMap, ok := m["resolution"].(map[string]interface{})
	if !ok {
		t.Fatal("resolution field is not an object")
	}
	if _, ok := resMap["value"]; !ok {
		t.Error("missing value in resolution breakdown")
	}
}

func TestScoreResult_StudioSpaceCollapse(t *testing.T) {
	// "Ass Parade" normalises to "ass parade"; NZB encodes it as "AssParade" → "assparade".
	// The space-stripped comparison should still produce a studio match.
	performers := []models.Performer{{Name: "Avery Jane"}}
	scene := makeScene("Surprises Fan With Booty", "Ass Parade", "ass-parade", "2026-03-16", 0, performers)
	result := makeResult("AssParade.26.03.16.Avery.Jane.XXX.1080p.MP4-WRB")

	bd := ScoreResult(scene, result, nil)

	if !bd.Studio.Matched {
		t.Errorf("studio should match via space-collapse: needle=%q haystack=%q",
			bd.Studio.Needle, bd.Studio.Haystack)
	}
	if bd.Studio.Score != 25 {
		t.Errorf("studio score = %d, want 25", bd.Studio.Score)
	}
	// date + studio + performer = 25 + 25 + 10 = 60
	if bd.Total() < 60 {
		t.Errorf("total = %d, want >= 60 (date+studio+performer)", bd.Total())
	}
}

func TestScoreResult_DateScoring(t *testing.T) {
	scene := makeScene("My Title", "BestStudios", "", "2024-03-15", 0, nil)

	tests := []struct {
		name       string
		nzbTitle   string
		wantPoints int
	}{
		{"exact date match", "BestStudios.My.Title.2024-03-15.1080p", 25},
		{"YY.MM.DD date match", "BestStudios.24.03.15.My.Title.1080p", 25},
		{"wrong date", "BestStudios.My.Title.2020-01-01.1080p", 0},
		{"no date in title", "BestStudios.My.Title.1080p", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd := ScoreResult(scene, makeResult(tt.nzbTitle), nil)
			if bd.Date.Score != tt.wantPoints {
				t.Errorf("date score = %d, want %d", bd.Date.Score, tt.wantPoints)
			}
		})
	}
}

func TestScoreResult_ResolutionExtracted(t *testing.T) {
	scene := makeScene("My Title", "BestStudios", "", "", 0, nil)

	tests := []struct {
		name       string
		nzbTitle   string
		wantValue  string
	}{
		{"2160p", "BestStudios.My.Title.2160p.MP4-P2P", "2160p"},
		{"1080p", "BestStudios.My.Title.1080p.WEB", "1080p"},
		{"no resolution", "BestStudios.My.Title.MP4-P2P", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd := ScoreResult(scene, makeResult(tt.nzbTitle), nil)
			if bd.Resolution.Value != tt.wantValue {
				t.Errorf("resolution value = %q, want %q", bd.Resolution.Value, tt.wantValue)
			}
			if bd.Resolution.Max != 0 {
				t.Errorf("resolution max = %d, want 0 (informational only)", bd.Resolution.Max)
			}
		})
	}
}
