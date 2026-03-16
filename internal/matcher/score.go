package matcher

// ScoreResult holds the confidence score for a search result.
type ScoreResult struct {
	TotalScore     int
	ScoreBreakdown map[string]interface{}
}

// ScoreResults scores a list of NZB results against scene metadata.
func ScoreResults(sceneTitle, studioName, releaseDate string, durationSeconds int, performers []string, results []string) []ScoreResult {
	// TODO: implement
	return nil
}
