package matcher

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mononen/stasharr/internal/models"
)

var knownTokens = map[string]bool{
	"title": true, "title_slug": true,
	"studio": true, "studio_slug": true,
	"performer": true, "performers": true, "performers_slug": true,
	"year": true, "date": true, "month": true,
	"resolution": true, "ext": true,
}

var resolutionPatterns = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`(?i)\b(2160p|4k|uhd)\b`), "2160p"},
	{regexp.MustCompile(`(?i)\b1080p\b`), "1080p"},
	{regexp.MustCompile(`(?i)\b720p\b`), "720p"},
	{regexp.MustCompile(`(?i)\b480p\b`), "480p"},
	{regexp.MustCompile(`(?i)\bsd\b`), "SD"},
}

var tokenPattern = regexp.MustCompile(`\{(\w+)\}`)
var multiDash = regexp.MustCompile(`-{2,}`)

// ValidationError describes a problem found during template validation.
type ValidationError struct {
	Token   string // empty for structural errors
	Message string
	IsError bool // false = warning
}

// ValidateTemplate validates a directory template string and returns any errors or warnings.
func ValidateTemplate(template string) []ValidationError {
	var errs []ValidationError

	// 1. Must not start or end with /.
	if strings.HasPrefix(template, "/") || strings.HasSuffix(template, "/") {
		errs = append(errs, ValidationError{
			Message: "template must not start or end with /",
			IsError: true,
		})
	}

	// 2. Unknown tokens → warning.
	for _, m := range tokenPattern.FindAllStringSubmatch(template, -1) {
		token := m[1]
		if !knownTokens[token] {
			errs = append(errs, ValidationError{
				Token:   token,
				Message: fmt.Sprintf("unknown token {%s}", token),
				IsError: false,
			})
		}
	}

	// 3. {ext} must appear in the filename segment (last segment after final /).
	segments := strings.Split(template, "/")
	last := segments[len(segments)-1]
	if !strings.Contains(last, "{ext}") {
		errs = append(errs, ValidationError{
			Token:   "ext",
			Message: "{ext} must appear in the filename portion (last segment after /)",
			IsError: true,
		})
	}

	// 4. Last segment must contain at least one non-token character.
	stripped := tokenPattern.ReplaceAllString(last, "")
	if strings.TrimSpace(stripped) == "" {
		errs = append(errs, ValidationError{
			Message: "filename portion must contain at least one non-token character",
			IsError: true,
		})
	}

	return errs
}

// Render resolves all tokens in a template string and returns a relative filesystem path.
// missingValue is substituted for any null/empty metadata field.
// performerMax caps the number of performers in {performers} / {performers_slug}.
func Render(template string, scene models.Scene, filename string, missingValue string, performerMax int) (string, error) {
	// Unmarshal performers.
	var performers []models.Performer
	if len(scene.Performers) > 0 {
		_ = json.Unmarshal(scene.Performers, &performers)
	}
	sorted := sortedPerformers(performers)

	// Detect resolution from filename.
	resolution := detectResolution(filename)

	// Split into path segments first; sanitization is applied per-segment.
	rawSegments := strings.Split(template, "/")
	out := make([]string, 0, len(rawSegments))

	for _, seg := range rawSegments {
		resolved := tokenPattern.ReplaceAllStringFunc(seg, func(match string) string {
			key := match[1 : len(match)-1]
			val := resolveToken(key, scene, sorted, resolution, filename, missingValue, performerMax)
			return sanitizeSegment(val)
		})
		out = append(out, resolved)
	}

	return strings.Join(out, "/"), nil
}

// resolveToken returns the value for a single template token.
func resolveToken(
	key string,
	scene models.Scene,
	sortedPerfs []models.Performer,
	resolution, filename, missingValue string,
	performerMax int,
) string {
	switch key {
	case "title":
		if scene.Title == "" {
			return missingValue
		}
		return scene.Title

	case "title_slug":
		if scene.Title == "" {
			return missingValue
		}
		return toSlug(scene.Title)

	case "studio":
		if !scene.StudioName.Valid || scene.StudioName.String == "" {
			return missingValue
		}
		return scene.StudioName.String

	case "studio_slug":
		if !scene.StudioSlug.Valid || scene.StudioSlug.String == "" {
			return missingValue
		}
		return scene.StudioSlug.String

	case "performer":
		if len(sortedPerfs) == 0 {
			return missingValue
		}
		return sortedPerfs[0].Name

	case "performers":
		if len(sortedPerfs) == 0 {
			return missingValue
		}
		n := performerMax
		if n > len(sortedPerfs) {
			n = len(sortedPerfs)
		}
		names := make([]string, n)
		for i := range names {
			names[i] = sortedPerfs[i].Name
		}
		return strings.Join(names, ", ")

	case "performers_slug":
		if len(sortedPerfs) == 0 {
			return missingValue
		}
		n := performerMax
		if n > len(sortedPerfs) {
			n = len(sortedPerfs)
		}
		parts := make([]string, n)
		for i := range parts {
			parts[i] = toSlug(sortedPerfs[i].Name)
		}
		return strings.Join(parts, "-")

	case "year":
		if !scene.ReleaseDate.Valid {
			return missingValue
		}
		return fmt.Sprintf("%04d", scene.ReleaseDate.Time.Year())

	case "date":
		if !scene.ReleaseDate.Valid {
			return missingValue
		}
		return scene.ReleaseDate.Time.Format("2006-01-02")

	case "month":
		if !scene.ReleaseDate.Valid {
			return missingValue
		}
		return fmt.Sprintf("%02d", int(scene.ReleaseDate.Time.Month()))

	case "resolution":
		if resolution == "" {
			return missingValue
		}
		return resolution

	case "ext":
		if idx := strings.LastIndex(filename, "."); idx >= 0 && idx < len(filename)-1 {
			return filename[idx+1:]
		}
		return missingValue

	default:
		// Unknown token: leave it in the output with braces so it's visible.
		return "{" + key + "}"
	}
}

// sanitizeSegment applies filesystem character sanitization rules to a single
// token value (not a full path segment) before it is embedded in the path.
func sanitizeSegment(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, `\`, "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, ".")
	s = multiDash.ReplaceAllString(s, "-")
	return s
}

// detectResolution returns the canonical resolution label from the filename, or "".
func detectResolution(filename string) string {
	for _, p := range resolutionPatterns {
		if p.pattern.MatchString(filename) {
			return p.label
		}
	}
	return ""
}

// toSlug converts a string to a URL/filesystem-safe slug (lowercase, hyphens).
func toSlug(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// sortedPerformers returns a copy of performers sorted by surname-first ordering.
// Names are sorted by last-space-delimited token (surname), displayed as-is.
func sortedPerformers(performers []models.Performer) []models.Performer {
	out := make([]models.Performer, len(performers))
	copy(out, performers)
	sort.Slice(out, func(i, j int) bool {
		return surnameKey(out[i].Name) < surnameKey(out[j].Name)
	})
	return out
}

// surnameKey builds a sort key of "SURNAME givennames" from a display name.
func surnameKey(name string) string {
	parts := strings.Fields(name)
	if len(parts) <= 1 {
		return name
	}
	surname := parts[len(parts)-1]
	given := strings.Join(parts[:len(parts)-1], " ")
	return surname + " " + given
}
