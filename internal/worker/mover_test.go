package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"scene.mp4", true},
		{"scene.mkv", true},
		{"scene.avi", true},
		{"scene.wmv", true},
		{"scene.mov", true},
		{"SCENE.MP4", true},  // case-insensitive
		{"SCENE.MKV", true},  // case-insensitive
		{"scene.nfo", false},
		{"scene.jpg", false},
		{"scene.par2", false},
		{"", false},
		{"nodot", false},
		{"/path/to/scene.mp4", true},
		{"/path/to/scene.NFO", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isVideoFile(tt.path)
			if got != tt.want {
				t.Errorf("isVideoFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDeduplicatePath(t *testing.T) {
	t.Run("fresh path is returned unchanged", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "scene.mp4")

		got := deduplicatePath(path)

		if got != path {
			t.Errorf("deduplicatePath(%q) = %q, want unchanged path", path, got)
		}
	})

	t.Run("existing file gets _1 suffix", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "scene.mp4")

		if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
			t.Fatalf("setup: WriteFile: %v", err)
		}

		got := deduplicatePath(path)
		want := filepath.Join(dir, "scene_1.mp4")

		if got != want {
			t.Errorf("deduplicatePath(%q) = %q, want %q", path, got, want)
		}
	})

	t.Run("existing file and _1 variant gets _2 suffix", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "scene.mp4")
		path1 := filepath.Join(dir, "scene_1.mp4")

		for _, p := range []string{path, path1} {
			if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
				t.Fatalf("setup: WriteFile(%q): %v", p, err)
			}
		}

		got := deduplicatePath(path)
		want := filepath.Join(dir, "scene_2.mp4")

		if got != want {
			t.Errorf("deduplicatePath(%q) = %q, want %q", path, got, want)
		}
	})
}

func TestFindLargestVideoFile(t *testing.T) {
	t.Run("returns largest video file", func(t *testing.T) {
		dir := t.TempDir()

		files := map[string][]byte{
			"small.mp4":  make([]byte, 100),
			"large.mkv":  make([]byte, 1000),
			"readme.nfo": make([]byte, 50),
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
				t.Fatalf("setup: WriteFile(%q): %v", name, err)
			}
		}

		got, err := findLargestVideoFile(dir)
		if err != nil {
			t.Fatalf("findLargestVideoFile: unexpected error: %v", err)
		}

		wantBase := "large.mkv"
		if filepath.Base(got) != wantBase {
			t.Errorf("findLargestVideoFile returned %q, want file named %q", got, wantBase)
		}
	})

	t.Run("empty dir returns empty path", func(t *testing.T) {
		dir := t.TempDir()

		got, err := findLargestVideoFile(dir)
		if err != nil {
			t.Fatalf("findLargestVideoFile: unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("findLargestVideoFile on empty dir = %q, want empty string", got)
		}
	})

	t.Run("non-video files only returns empty path", func(t *testing.T) {
		dir := t.TempDir()

		for _, name := range []string{"readme.nfo", "cover.jpg", "checksum.par2"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644); err != nil {
				t.Fatalf("setup: WriteFile(%q): %v", name, err)
			}
		}

		got, err := findLargestVideoFile(dir)
		if err != nil {
			t.Fatalf("findLargestVideoFile: unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("findLargestVideoFile with no video files = %q, want empty string", got)
		}
	})
}
