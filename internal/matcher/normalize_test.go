package matcher

import (
	"testing"
	"time"
)

func TestNormalizeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Diacritics
		{"cafe accent", "café", "cafe"},
		{"mixed diacritics", "Ñoño", "nono"},
		{"german umlauts", "Über", "uber"},
		{"multiple diacritics", "Ação", "acao"},

		// Common substitutions
		{"ampersand", "A & B", "a and b"},
		{"plus sign", "A + B", "a and b"},
		{"at sign", "Meet @ Home", "meet at home"},

		// Punctuation stripping — apostrophe becomes space, not collapsed to nothing
		{"punctuation strip", "Hello, World!", "hello world"},
		{"apostrophe becomes space", "It's a test", "it s a test"},
		{"parentheses", "Title (2024)", "title 2024"},

		// Dots/dashes: filler "-GROUPNAME" stripping and "scene" filler removal.
		// "Studio.Name.Scene-Title" → lower → scene filler removes "scene" →
		// "-title" is then the trailing dash pattern → stripped → "studio.name." → punct → "studio name"
		{"dots and dashes", "Studio.Name.Scene-Title", "studio name"},
		{"underscores", "Studio_Name_Scene_Title", "studio name scene title"},
		// ^ underscores: \b doesn't fire around _, so "scene" is not stripped mid-word-chars.

		// Filler patterns
		// "scene" IS a filler word — it is stripped wherever it appears as a standalone word.
		{"1080p stripped", "Title 1080p", "title"},
		{"720p and WEB stripped", "Title 720p WEB", "title"},
		{"codec hevc aac stripped", "Title.h265.AAC", "title"},
		{"xxx stripped, title kept", "Title XXX 1080p", "title"},
		// After filler, "scene" removed, "[GroupName]" removed → empty
		{"scene and bracket group both stripped", "Scene [GroupName]", ""},
		// "Scene-GROUPNAME": -GROUPNAME stripped → "scene" → scene stripped → empty
		{"trailing group dash stripped", "Scene-GROUPNAME", ""},
		// "Scene WEBRip": webrip stripped, scene stripped → empty
		{"webrip stripped", "Scene WEBRip", ""},
		// "Scene 4K UHD": 4k/uhd stripped, scene stripped → empty
		{"4k uhd stripped", "Scene 4K UHD", ""},
		// "Scene AAC MP3": aac/mp3 stripped, scene stripped → empty
		{"aac mp3 stripped", "Scene AAC MP3", ""},

		// Whitespace collapse
		{"multi space", "hello   world", "hello world"},
		{"leading trailing space", "  hello  ", "hello"},

		// Filler removes "part N" and "scene"; standalone digits preserved
		{"part number stripped scene stripped", "Scene 2 Part 3", "2"},

		// Non-filler content intact
		{"non-filler title", "My Amazing Title", "my amazing title"},
		{"word scene in underscore context not stripped", "Studio_Name_Scene_Title", "studio name scene title"},

		// Edge cases
		{"empty string", "", ""},
		{"all filler", "XXX 1080p HEVC AAC WEB", ""},
		{"single char", "a", "a"},
		{"digits only", "2024", "2024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeString(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeDate(t *testing.T) {
	mustDate := func(y, m, d int) *time.Time {
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return &t
	}

	tests := []struct {
		name  string
		input string
		want  *time.Time
	}{
		{"YYYY-MM-DD", "Scene.2024-03-15.1080p", mustDate(2024, 3, 15)},
		{"YYYY.MM.DD", "Scene.2024.03.15.1080p", mustDate(2024, 3, 15)},
		{"DD.MM.YYYY", "15.03.2024 Scene", mustDate(2024, 3, 15)},
		{"MMDDYYYY", "Scene 03152024 1080p", mustDate(2024, 3, 15)},
		{"Month DD YYYY", "Scene March 15 2024 1080p", mustDate(2024, 3, 15)},
		{"YY.MM.DD", "Deeper.26.03.12.Scene.Title.2160p", mustDate(2026, 3, 12)},
		{"no date", "Scene Title Without Date", nil},
		{"empty string", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeDate(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("NormalizeDate(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Errorf("NormalizeDate(%q) = nil, want %v", tt.input, tt.want)
				return
			}
			if !got.Equal(*tt.want) {
				t.Errorf("NormalizeDate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractDuration(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"MM:SS format 47m27s", "Title 47:27 1080p", 47*60 + 27},
		{"HH:MM:SS format", "Title 1:20:30 1080p", 1*3600 + 20*60 + 30},
		{"Xmin format", "Title 45min 1080p", 45 * 60},
		{"Xm format", "Title 30m 1080p", 30 * 60},
		{"no duration", "Title 1080p", 0},
		{"empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDuration(tt.input)
			if got != tt.want {
				t.Errorf("ExtractDuration(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractResolution(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"2160p", "Studio.Title.2160p.MP4-P2P", "2160p"},
		{"1080p", "Studio.Title.1080p.WEB", "1080p"},
		{"720p", "Studio.Title.720p.WEB", "720p"},
		{"480p", "Studio.Title.480p.WEB", "480p"},
		{"4k via UHD", "Studio.Title.UHD.WEB", "2160p"},
		{"no resolution", "Studio.Title.MP4-P2P", ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractResolution(tt.input)
			if got != tt.want {
				t.Errorf("ExtractResolution(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeStudio(t *testing.T) {
	aliases := map[string]string{
		"teamskeet": "team skeet",
		"brazzers":  "brazzers",
	}

	tests := []struct {
		name    string
		input   string
		aliases map[string]string
		want    string
	}{
		{"no alias", "Reality Kings", nil, "reality kings"},
		{"alias resolves", "TeamSkeet", aliases, "team skeet"},
		{"already normalized alias", "Brazzers", aliases, "brazzers"},
		{"unknown studio", "Unknown Studio", aliases, "unknown studio"},
		{"nil aliases", "Studio Name", nil, "studio name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeStudio(tt.input, tt.aliases)
			if got != tt.want {
				t.Errorf("NormalizeStudio(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
