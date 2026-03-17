package worker

import (
	"testing"
)

func TestApplyThreshold(t *testing.T) {
	tests := []struct {
		name            string
		topScore        int
		autoThreshold   int
		reviewThreshold int
		want            string
	}{
		// Standard thresholds: auto=85, review=50.
		{name: "above auto", topScore: 100, autoThreshold: 85, reviewThreshold: 50, want: "auto_approved"},
		{name: "at auto", topScore: 85, autoThreshold: 85, reviewThreshold: 50, want: "auto_approved"},
		{name: "one below auto", topScore: 84, autoThreshold: 85, reviewThreshold: 50, want: "awaiting_review"},
		{name: "at review", topScore: 50, autoThreshold: 85, reviewThreshold: 50, want: "awaiting_review"},
		{name: "one below review", topScore: 49, autoThreshold: 85, reviewThreshold: 50, want: "search_failed"},
		{name: "zero score", topScore: 0, autoThreshold: 85, reviewThreshold: 50, want: "search_failed"},

		// Edge case: autoThreshold=100 forces all non-perfect scores to review.
		{name: "all manual: score 99", topScore: 99, autoThreshold: 100, reviewThreshold: 50, want: "awaiting_review"},

		// Edge case: autoThreshold=0 means every score is auto-approved.
		{name: "all auto: score 0", topScore: 0, autoThreshold: 0, reviewThreshold: 0, want: "auto_approved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyThreshold(tt.topScore, tt.autoThreshold, tt.reviewThreshold)
			if got != tt.want {
				t.Errorf("applyThreshold(%d, %d, %d) = %q, want %q",
					tt.topScore, tt.autoThreshold, tt.reviewThreshold, got, tt.want)
			}
		})
	}
}
