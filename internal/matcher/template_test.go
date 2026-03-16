package matcher

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mononen/stasharr/internal/models"
)

const defaultMissing = "1unknown"

func makeTemplateScene(title, studio, studioSlug, releaseDate string, performers []models.Performer) models.Scene {
	s := models.Scene{ID: uuid.New(), JobID: uuid.New(), Title: title}
	if studio != "" {
		s.StudioName = pgtype.Text{String: studio, Valid: true}
		s.StudioSlug = pgtype.Text{String: studioSlug, Valid: true}
	}
	if releaseDate != "" {
		t, _ := time.Parse("2006-01-02", releaseDate)
		s.ReleaseDate = pgtype.Date{Time: t, Valid: true}
	}
	if len(performers) > 0 {
		b, _ := json.Marshal(performers)
		s.Performers = b
	}
	return s
}

func render(t *testing.T, tmpl string, scene models.Scene, filename string) string {
	t.Helper()
	out, err := Render(tmpl, scene, filename, defaultMissing, 3)
	if err != nil {
		t.Fatalf("Render(%q) error: %v", tmpl, err)
	}
	return out
}

func TestRender_TitleToken(t *testing.T) {
	scene := makeTemplateScene("My Scene Title", "Studio", "studio", "2024-03-15", nil)
	got := render(t, "{title}.{ext}", scene, "file.mp4")
	if got != "My Scene Title.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_TitleSlugToken(t *testing.T) {
	scene := makeTemplateScene("My Scene Title", "Studio", "studio", "2024-03-15", nil)
	got := render(t, "{title_slug}.{ext}", scene, "file.mp4")
	if got != "my-scene-title.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_StudioToken(t *testing.T) {
	scene := makeTemplateScene("Title", "Studio Name", "studio-name", "2024-03-15", nil)
	got := render(t, "{studio}/{title}.{ext}", scene, "file.mp4")
	if got != "Studio Name/Title.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_StudioSlugToken(t *testing.T) {
	scene := makeTemplateScene("Title", "Studio Name", "studio-name", "2024-03-15", nil)
	got := render(t, "{studio_slug}/{title}.{ext}", scene, "file.mp4")
	if got != "studio-name/Title.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_DateTokens(t *testing.T) {
	scene := makeTemplateScene("Title", "Studio", "studio", "2024-03-15", nil)

	tests := []struct {
		token string
		want  string
	}{
		{"{year}", "2024"},
		{"{month}", "03"},
		{"{date}", "2024-03-15"},
	}
	for _, tt := range tests {
		got := render(t, tt.token+".{ext}", scene, "f.mp4")
		want := tt.want + ".mp4"
		if got != want {
			t.Errorf("token %s: got %q want %q", tt.token, got, want)
		}
	}
}

func TestRender_PerformerToken(t *testing.T) {
	// Alice Smith sorts before Jane Doe by surname (Doe < Smith alphabetically).
	// Wait: "Doe" < "Smith" so Jane Doe comes first.
	performers := []models.Performer{
		{Name: "Alice Smith"},
		{Name: "Jane Doe"},
	}
	scene := makeTemplateScene("Title", "Studio", "studio", "2024-03-15", performers)
	got := render(t, "{performer}.{ext}", scene, "f.mp4")
	// Jane Doe (surname "Doe") sorts before Alice Smith (surname "Smith").
	if got != "Jane Doe.mp4" {
		t.Errorf("got %q, want \"Jane Doe.mp4\"", got)
	}
}

func TestRender_PerformersToken(t *testing.T) {
	performers := []models.Performer{
		{Name: "Alice Smith"},
		{Name: "Jane Doe"},
		{Name: "Carol Brown"},
	}
	scene := makeTemplateScene("Title", "Studio", "studio", "2024-03-15", performers)
	// Sorted by surname: Brown, Doe, Smith → Carol Brown, Jane Doe, Alice Smith.
	got := render(t, "{performers}.{ext}", scene, "f.mp4")
	if got != "Carol Brown, Jane Doe, Alice Smith.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_PerformerTruncation(t *testing.T) {
	performers := []models.Performer{
		{Name: "Carol Brown"},
		{Name: "Jane Doe"},
		{Name: "Eve Miller"},
		{Name: "Alice Smith"},
	}
	scene := makeTemplateScene("Title", "Studio", "studio", "2024-03-15", performers)

	out, err := Render("{performers}.{ext}", scene, "f.mp4", defaultMissing, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Sorted: Brown, Doe, Miller, Smith → first 2: Carol Brown, Jane Doe.
	if out != "Carol Brown, Jane Doe.mp4" {
		t.Errorf("got %q", out)
	}
}

func TestRender_PerformersSlugToken(t *testing.T) {
	performers := []models.Performer{
		{Name: "Jane Doe"},
		{Name: "John Smith"},
	}
	scene := makeTemplateScene("Title", "Studio", "studio", "2024-03-15", performers)
	got := render(t, "{performers_slug}.{ext}", scene, "f.mp4")
	// Sorted: Doe, Smith → jane-doe-john-smith.mp4
	if got != "jane-doe-john-smith.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_ResolutionToken(t *testing.T) {
	scene := makeTemplateScene("Title", "Studio", "studio", "2024-03-15", nil)

	tests := []struct {
		filename string
		want     string
	}{
		{"Scene.1080p.mp4", "1080p"},
		{"Scene.720p.mp4", "720p"},
		{"Scene.2160p.mp4", "2160p"},
		{"Scene.4K.mp4", "2160p"},
		{"Scene.UHD.mp4", "2160p"},
		{"Scene.480p.mp4", "480p"},
		{"Scene.mp4", defaultMissing},
	}
	for _, tt := range tests {
		got := render(t, "[{resolution}].{ext}", scene, tt.filename)
		want := "[" + tt.want + "].mp4"
		if got != want {
			t.Errorf("filename=%q: got %q want %q", tt.filename, got, want)
		}
	}
}

func TestRender_ExtToken(t *testing.T) {
	scene := makeTemplateScene("Title", "Studio", "studio", "2024-03-15", nil)

	tests := []struct {
		filename string
		want     string
	}{
		{"file.mp4", "Title.mp4"},
		{"file.mkv", "Title.mkv"},
		{"file.avi", "Title.avi"},
		{"noext", "Title." + defaultMissing},
	}
	for _, tt := range tests {
		got := render(t, "{title}.{ext}", scene, tt.filename)
		if got != tt.want {
			t.Errorf("filename=%q: got %q want %q", tt.filename, got, tt.want)
		}
	}
}

func TestRender_MissingFieldFallback(t *testing.T) {
	// Empty scene — all optional fields missing.
	scene := models.Scene{ID: uuid.New(), JobID: uuid.New(), Title: ""}

	tests := []struct {
		token string
		want  string
	}{
		{"{title}", defaultMissing},
		{"{title_slug}", defaultMissing},
		{"{studio}", defaultMissing},
		{"{studio_slug}", defaultMissing},
		{"{performer}", defaultMissing},
		{"{performers}", defaultMissing},
		{"{performers_slug}", defaultMissing},
		{"{year}", defaultMissing},
		{"{date}", defaultMissing},
		{"{month}", defaultMissing},
		{"{resolution}", defaultMissing},
	}
	for _, tt := range tests {
		got := render(t, tt.token+".{ext}", scene, "f.mp4")
		want := tt.want + ".mp4"
		if got != want {
			t.Errorf("token %s: got %q want %q", tt.token, got, want)
		}
	}
}

func TestRender_SanitizationIllegalChars(t *testing.T) {
	// Studio name with illegal filesystem characters.
	scene := makeTemplateScene("Title", "Studio: Name/Test", "studio", "2024-03-15", nil)
	got := render(t, "{studio}/{title}.{ext}", scene, "f.mp4")
	// / in studio → path separator is template's /, the / in the studio value becomes -
	// : in studio → -
	if got != "Studio- Name-Test/Title.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_SanitizationLeadingDot(t *testing.T) {
	// Title starting with a dot → leading dot stripped.
	scene := makeTemplateScene(".Hidden Title", "Studio", "studio", "2024-03-15", nil)
	got := render(t, "{studio}/{title}.{ext}", scene, "f.mp4")
	if got != "Studio/Hidden Title.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_SanitizationNullByte(t *testing.T) {
	scene := makeTemplateScene("Title\x00Name", "Studio", "studio", "2024-03-15", nil)
	got := render(t, "{title}.{ext}", scene, "f.mp4")
	if got != "TitleName.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRender_ExampleTemplates(t *testing.T) {
	performers := []models.Performer{
		{Name: "Jane Doe"},
		{Name: "John Smith"},
	}
	scene := makeTemplateScene("My Scene Title", "Studio Name", "studio-name", "2024-03-15", performers)

	tests := []struct {
		name     string
		tmpl     string
		filename string
		want     string
	}{
		{
			"default studio/year/title",
			"{studio}/{year}/{title} ({year}).{ext}",
			"scene.mp4",
			"Studio Name/2024/My Scene Title (2024).mp4",
		},
		{
			"performer-first",
			"{performer}/{studio}/{title} ({date}).{ext}",
			"scene.mp4",
			"Jane Doe/Studio Name/My Scene Title (2024-03-15).mp4",
		},
		{
			"date-based",
			"{year}/{month}/{studio}/{title}.{ext}",
			"scene.mp4",
			"2024/03/Studio Name/My Scene Title.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := render(t, tt.tmpl, scene, tt.filename)
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestRender_FlatExampleTemplate(t *testing.T) {
	performers := []models.Performer{
		{Name: "Jane Doe"},
		{Name: "John Smith"},
	}
	scene := makeTemplateScene("My Scene Title", "Studio Name", "studio-name", "2024-03-15", performers)

	got, err := Render("{studio} - {title} - {performers} ({year}) [{resolution}].{ext}", scene, "Scene.1080p.mp4", defaultMissing, 3)
	if err != nil {
		t.Fatal(err)
	}
	// Sorted performers: Doe, Smith → Jane Doe, John Smith
	want := "Studio Name - My Scene Title - Jane Doe, John Smith (2024) [1080p].mp4"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name       string
		tmpl       string
		wantErrors int // count of IsError=true entries
		wantWarns  int // count of IsError=false entries
	}{
		{
			"valid template",
			"{studio}/{year}/{title}.{ext}",
			0, 0,
		},
		{
			// No {ext} in last segment AND last segment strips to empty → 2 errors.
			"missing ext",
			"{studio}/{year}/{title}",
			2, 0,
		},
		{
			"starts with slash",
			"/{studio}/{title}.{ext}",
			1, 0,
		},
		{
			// Ends with /: slash error + empty last segment has no {ext} + empty filename → 3 errors.
			"ends with slash",
			"{studio}/{title}.{ext}/",
			3, 0,
		},
		{
			"unknown token is warning",
			"{studio}/{custom_token}/{title}.{ext}",
			0, 1,
		},
		{
			// "{title}{ext}" — {ext} IS present in last segment (no ext error),
			// but stripped last segment = "" → filename-only-tokens error.
			"filename is only tokens no separator",
			"{studio}/{title}{ext}",
			1, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateTemplate(tt.tmpl)
			var errCount, warnCount int
			for _, e := range errs {
				if e.IsError {
					errCount++
				} else {
					warnCount++
				}
			}
			if errCount != tt.wantErrors {
				t.Errorf("errors: got %d want %d (errs=%+v)", errCount, tt.wantErrors, errs)
			}
			if warnCount != tt.wantWarns {
				t.Errorf("warnings: got %d want %d (errs=%+v)", warnCount, tt.wantWarns, errs)
			}
		})
	}
}
