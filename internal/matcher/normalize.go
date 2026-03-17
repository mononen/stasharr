package matcher

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// fillerPatterns are applied (in order) after substitutions, before punctuation strip,
// so bracket and trailing-dash patterns still have their anchoring characters.
var fillerPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(hd|sd|fhd|uhd)\b`),
	regexp.MustCompile(`(?i)\b(1080p|720p|480p|2160p|4k)\b`),
	regexp.MustCompile(`(?i)\b(x264|x265|h264|h265|hevc|avc)\b`),
	regexp.MustCompile(`(?i)\b(aac|mp3|ac3|dts|flac)\b`),
	regexp.MustCompile(`(?i)\b(web|webrip|webdl|web-dl|bluray|bdrip)\b`),
	regexp.MustCompile(`(?i)\b(xxx|adult|18)\b`),
	regexp.MustCompile(`(?i)\b(complete|full|scene|clip|part\s?\d+)\b`),
	regexp.MustCompile(`\[\w+\]`),  // [GroupName]
	regexp.MustCompile(`-\w+$`),   // trailing -GROUPNAME
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9 ]`)
var multiSpace = regexp.MustCompile(`\s+`)

// NormalizeString applies the full normalization pipeline:
// NFD diacritic strip → lowercase → substitutions → filler removal → punct strip → whitespace collapse.
func NormalizeString(s string) string {
	// 1. NFD decompose, strip combining characters (diacritics).
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}))
	s, _, _ = transform.String(t, s)

	// 2. Lowercase.
	s = strings.ToLower(s)

	// 3. Common substitutions.
	s = strings.ReplaceAll(s, "&", " and ")
	s = strings.ReplaceAll(s, "+", " and ")
	s = strings.ReplaceAll(s, "@", " at ")

	// 4. Filler pattern removal (before punct strip so bracket/dash anchors work).
	for _, p := range fillerPatterns {
		s = p.ReplaceAllString(s, " ")
	}

	// 5. Punctuation stripping — keep only a–z, 0–9, space.
	s = nonAlphaNum.ReplaceAllString(s, " ")

	// 6. Whitespace collapse.
	s = strings.TrimSpace(multiSpace.ReplaceAllString(s, " "))

	return s
}

// dateFormats lists the NZB date patterns we attempt to parse, in priority order.
var dateFormats = []struct {
	re     *regexp.Regexp
	layout string
}{
	// YYYY-MM-DD
	{regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`), "2006-01-02"},
	// YYYY.MM.DD
	{regexp.MustCompile(`\b(\d{4}\.\d{2}\.\d{2})\b`), "2006.01.02"},
	// DD.MM.YYYY
	{regexp.MustCompile(`\b(\d{2}\.\d{2}\.\d{4})\b`), "02.01.2006"},
	// YY.MM.DD (e.g. 26.03.12 → 2026-03-12); checked after 4-digit-year patterns.
	{regexp.MustCompile(`\b(\d{2}\.\d{2}\.\d{2})\b`), "06.01.02"},
	// MMDDYYYY (8 digits, no separators)
	{regexp.MustCompile(`(?:^|[^\d])(\d{8})(?:$|[^\d])`), "01022006"},
	// Month DD YYYY
	{regexp.MustCompile(`\b((?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2}\s+\d{4})\b`), "January 2 2006"},
}

// NormalizeDate extracts a date from common NZB title patterns.
// Returns nil if no parseable date is found.
func NormalizeDate(s string) *time.Time {
	for _, f := range dateFormats {
		m := f.re.FindStringSubmatch(s)
		if len(m) < 2 {
			continue
		}
		t, err := time.Parse(f.layout, m[1])
		if err != nil {
			continue
		}
		return &t
	}
	return nil
}

// durationPatterns list patterns to extract a scene duration, in priority order.
var durationPatterns = []struct {
	re      *regexp.Regexp
	convert func([]string) int
}{
	// HH:MM:SS
	{
		regexp.MustCompile(`\b(\d{1,2}):(\d{2}):(\d{2})\b`),
		func(m []string) int {
			h, _ := strconv.Atoi(m[1])
			mn, _ := strconv.Atoi(m[2])
			sc, _ := strconv.Atoi(m[3])
			return h*3600 + mn*60 + sc
		},
	},
	// MM:SS (most scene durations are 20-80 min, so treat as minutes:seconds)
	{
		regexp.MustCompile(`\b(\d{1,2}):(\d{2})\b`),
		func(m []string) int {
			mn, _ := strconv.Atoi(m[1])
			sc, _ := strconv.Atoi(m[2])
			return mn*60 + sc
		},
	},
	// Xmin or X min
	{
		regexp.MustCompile(`(?i)\b(\d+)\s*min\b`),
		func(m []string) int {
			n, _ := strconv.Atoi(m[1])
			return n * 60
		},
	},
	// Xm (standalone, e.g. "47m")
	{
		regexp.MustCompile(`(?i)\b(\d+)m\b`),
		func(m []string) int {
			n, _ := strconv.Atoi(m[1])
			return n * 60
		},
	},
}

// ExtractDuration extracts duration in seconds from NZB title patterns.
// Returns 0 if no duration is found.
func ExtractDuration(s string) int {
	for _, p := range durationPatterns {
		m := p.re.FindStringSubmatch(s)
		if m != nil {
			return p.convert(m)
		}
	}
	return 0
}

// NormalizeStudio normalizes a studio name and resolves it against the alias map.
// The alias map keys and values should already be NormalizeString'd.
func NormalizeStudio(s string, aliases map[string]string) string {
	normalized := NormalizeString(s)
	if canonical, ok := aliases[normalized]; ok {
		return canonical
	}
	return normalized
}

// ExtractResolution returns the canonical resolution string found in an NZB title
// (e.g. "2160p", "1080p", "720p") or an empty string if none is found.
// Uses the shared resolutionPatterns defined in template.go.
func ExtractResolution(s string) string {
	for _, p := range resolutionPatterns {
		if p.pattern.MatchString(s) {
			return p.label
		}
	}
	return ""
}
