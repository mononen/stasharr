package worker

import (
	"testing"
)

func TestParseStashDBURL(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		wantEntityType string
		wantEntityID   string
		wantErr        bool
	}{
		{
			name:           "scene URL",
			url:            "https://stashdb.org/scenes/550e8400-e29b-41d4-a716-446655440000",
			wantEntityType: "scenes",
			wantEntityID:   "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:           "performer URL",
			url:            "https://stashdb.org/performers/550e8400-e29b-41d4-a716-446655440001",
			wantEntityType: "performers",
			wantEntityID:   "550e8400-e29b-41d4-a716-446655440001",
		},
		{
			name:           "studio URL",
			url:            "https://stashdb.org/studios/550e8400-e29b-41d4-a716-446655440002",
			wantEntityType: "studios",
			wantEntityID:   "550e8400-e29b-41d4-a716-446655440002",
		},
		{
			name:    "wrong host",
			url:     "https://example.com/foo",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entityType, entityID, err := parseStashDBURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseStashDBURL(%q) expected error, got nil (entityType=%q entityID=%q)",
						tt.url, entityType, entityID)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseStashDBURL(%q) unexpected error: %v", tt.url, err)
			}
			if entityType != tt.wantEntityType {
				t.Errorf("entityType = %q, want %q", entityType, tt.wantEntityType)
			}
			if entityID != tt.wantEntityID {
				t.Errorf("entityID = %q, want %q", entityID, tt.wantEntityID)
			}
		})
	}
}

func TestBatchSplitThreshold(t *testing.T) {
	const threshold = 40

	tests := []struct {
		name      string
		count     int
		wantFirst int
		wantRest  int
	}{
		{name: "below threshold", count: 10, wantFirst: 10, wantRest: 0},
		{name: "at threshold", count: 40, wantFirst: 40, wantRest: 0},
		{name: "one over threshold", count: 41, wantFirst: 40, wantRest: 1},
		{name: "well over threshold", count: 100, wantFirst: 40, wantRest: 60},
		{name: "empty slice", count: 0, wantFirst: 0, wantRest: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scenes := make([]string, tt.count)
			for i := range scenes {
				scenes[i] = "scene"
			}

			first, rest := splitBatch(scenes, threshold)

			if len(first) != tt.wantFirst {
				t.Errorf("len(first) = %d, want %d", len(first), tt.wantFirst)
			}
			if len(rest) != tt.wantRest {
				t.Errorf("len(rest) = %d, want %d", len(rest), tt.wantRest)
			}
			if len(first)+len(rest) != tt.count {
				t.Errorf("first+rest = %d, want %d (no elements lost or duplicated)",
					len(first)+len(rest), tt.count)
			}
		})
	}
}
