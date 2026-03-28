package worker

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/clients/myjdownloader"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

const defaultWatcherPollInterval = 60
const defaultStableSeconds = 120
const defaultStableFallbackSeconds = 600
const defaultMatchThreshold = 60

// sizeSnapshot records a filesystem size measurement at a point in time.
type sizeSnapshot struct {
	size int64
	at   time.Time
}

// LocalWatcherWorker scans a configured directory for files/folders that match
// search_failed scenes, monitors them for download completion via MyJDownloader,
// then transitions them into the normal mover/scanner pipeline.
type LocalWatcherWorker struct {
	Base
	myjd *myjdownloader.Client

	mu          sync.Mutex
	sizeHistory map[string][]sizeSnapshot // keyed by job_id string
}

func NewLocalWatcherWorker(app *models.App, logger zerolog.Logger) *LocalWatcherWorker {
	return &LocalWatcherWorker{
		Base:        Base{db: app.DB, config: app.Config, logger: logger},
		myjd:        app.MyJDownloader,
		sizeHistory: make(map[string][]sizeSnapshot),
	}
}

func (w *LocalWatcherWorker) Name() string { return "local_watcher" }

func (w *LocalWatcherWorker) Start(ctx context.Context) {
	intervalSecs := defaultWatcherPollInterval
	if raw := w.config.Get("localwatcher.poll_interval"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			intervalSecs = n
		}
	}

	ticker := time.NewTicker(time.Duration(intervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *LocalWatcherWorker) Stop() {}

func (w *LocalWatcherWorker) tick(ctx context.Context) {
	// Always check completion so manually-matched jobs progress even when
	// auto-scanning is disabled.
	w.checkCompletion(ctx)

	if w.config.Get("localwatcher.enabled") != "true" {
		return
	}
	watchDir := w.config.Get("localwatcher.watch_dir")
	if watchDir == "" {
		return
	}
	w.matchNewScenes(ctx, watchDir)
}

// matchNewScenes scans watchDir for unmatched entries that correspond to
// search_failed scenes, and creates a local download record for each match.
func (w *LocalWatcherWorker) matchNewScenes(ctx context.Context, watchDir string) {
	entries, err := os.ReadDir(watchDir)
	if err != nil {
		w.logger.Warn().Err(err).Str("dir", watchDir).Msg("local_watcher: failed to read watch directory")
		return
	}

	q := queries.New(w.db)

	scenes, err := q.GetSearchFailedScenes(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("local_watcher: failed to load search_failed scenes")
		return
	}
	if len(scenes) == 0 {
		return
	}

	// Build a set of job IDs already in local_found (have a download record).
	localFoundRows, err := q.GetLocalFoundDownloads(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("local_watcher: failed to load local_found downloads")
		return
	}
	alreadyMatched := make(map[uuid.UUID]bool, len(localFoundRows))
	for _, row := range localFoundRows {
		alreadyMatched[row.JobID] = true
	}

	threshold := defaultMatchThreshold
	if raw := w.config.Get("localwatcher.match_threshold"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			threshold = n
		}
	}

	for _, entry := range entries {
		entryName := entry.Name()
		entryPath := filepath.Join(watchDir, entryName)

		// Only consider directories and video files at the top level.
		if !entry.IsDir() && !isVideoFile(entryPath) {
			continue
		}

		bestScore := -1
		bestIdx := -1
		tie := false

		for i, scene := range scenes {
			if alreadyMatched[scene.JobID] {
				continue
			}
			score := tokenOverlap(entryName, scene.Title)
			if score > bestScore {
				bestScore = score
				bestIdx = i
				tie = false
			} else if score == bestScore && score >= threshold {
				tie = true
			}
		}

		if bestIdx < 0 || bestScore < threshold || tie {
			continue
		}

		scene := scenes[bestIdx]

		w.logger.Info().
			Str("entry", entryName).
			Str("scene", scene.Title).
			Int("score", bestScore).
			Msg("local_watcher: matched entry to scene")

		if _, err := q.CreateLocalDownload(ctx, queries.CreateLocalDownloadParams{
			JobID:      scene.JobID,
			SourcePath: pgtype.Text{String: entryPath, Valid: true},
		}); err != nil {
			w.logger.Error().Err(err).Str("job_id", scene.JobID.String()).Msg("local_watcher: failed to create download record")
			continue
		}

		if err := w.updateJobStatus(ctx, scene.JobID, "local_found", ""); err != nil {
			w.logger.Error().Err(err).Str("job_id", scene.JobID.String()).Msg("local_watcher: failed to update job status")
			continue
		}

		_ = w.emitEvent(ctx, scene.JobID, "local_file_matched", map[string]interface{}{
			"path":  entryPath,
			"entry": entryName,
			"score": bestScore,
		})

		alreadyMatched[scene.JobID] = true
	}
}

// checkCompletion polls local_found jobs for size stability and JD package status.
func (w *LocalWatcherWorker) checkCompletion(ctx context.Context) {
	q := queries.New(w.db)

	localFound, err := q.GetLocalFoundDownloads(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("local_watcher: failed to load local_found downloads")
		return
	}
	if len(localFound) == 0 {
		return
	}

	stableSecs := defaultStableSeconds
	if raw := w.config.Get("localwatcher.stable_seconds"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			stableSecs = n
		}
	}
	fallbackSecs := defaultStableFallbackSeconds
	if raw := w.config.Get("localwatcher.stable_fallback_seconds"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			fallbackSecs = n
		}
	}

	// Fetch JD packages once for all jobs (avoids repeated API calls per tick).
	var jdPackages []myjdownloader.Package
	if w.myjd != nil && w.myjd.DeviceName != "" {
		pkgs, err := w.myjd.ListPackages(ctx)
		if err != nil {
			w.logger.Warn().Err(err).Msg("local_watcher: failed to list JD packages, will rely on fallback stability window")
		} else {
			jdPackages = pkgs
		}
	}

	now := time.Now()

	for _, row := range localFound {
		if !row.SourcePath.Valid || row.SourcePath.String == "" {
			continue
		}
		jobKey := row.JobID.String()
		sourcePath := row.SourcePath.String

		size, err := totalSize(sourcePath)
		if err != nil {
			w.logger.Warn().Err(err).Str("path", sourcePath).Msg("local_watcher: failed to stat path")
			continue
		}

		w.mu.Lock()
		history := w.sizeHistory[jobKey]
		history = append(history, sizeSnapshot{size: size, at: now})
		// Trim snapshots older than fallbackSecs * 2 to cap memory use.
		cutoff := now.Add(-time.Duration(fallbackSecs*2) * time.Second)
		trimmed := history[:0]
		for _, s := range history {
			if s.at.After(cutoff) {
				trimmed = append(trimmed, s)
			}
		}
		w.sizeHistory[jobKey] = trimmed
		w.mu.Unlock()

		// Emit a timeline event so the user can see progress.
		if len(trimmed) >= 2 {
			stableFor := int(trimmed[len(trimmed)-1].at.Sub(trimmed[0].at).Seconds())
			_ = w.emitEvent(ctx, row.JobID, "stability_check", map[string]interface{}{
				"stable_for_secs": stableFor,
				"required_secs":   stableSecs,
			})
		}

		if !isSizeStable(trimmed, stableSecs, size) {
			continue
		}

		// Size has been stable for stableSecs. Check JD for confirmed completion.
		folderName := filepath.Base(sourcePath)
		jdFound, jdFinished := findJDPackage(jdPackages, folderName)
		if jdFinished {
			w.logger.Info().Str("path", sourcePath).Msg("local_watcher: JD package finished, marking complete")
			w.markComplete(ctx, row.JobID, sourcePath, jobKey)
			continue
		}
		if jdFound {
			// Package is in JD but still downloading — size pre-allocation can make
			// files appear stable while actively downloading, so skip the fallback
			// and wait for JD to confirm the download is finished.
			continue
		}

		// Package not found in JD (cleared by user, name mismatch, or JD not
		// configured) — fall back to extended size-stability window.
		if isSizeStable(trimmed, fallbackSecs, size) {
			w.logger.Info().
				Str("path", sourcePath).
				Msg("local_watcher: JD package not found, accepting via extended stability window")
			w.markComplete(ctx, row.JobID, sourcePath, jobKey)
		}
	}
}

func (w *LocalWatcherWorker) markComplete(ctx context.Context, jobID uuid.UUID, sourcePath, jobKey string) {
	if _, err := w.db.Exec(ctx,
		`UPDATE downloads
		 SET source_path  = $1,
		     status       = 'complete',
		     completed_at = NOW(),
		     updated_at   = NOW()
		 WHERE job_id = $2`,
		sourcePath, jobID,
	); err != nil {
		w.logger.Error().Err(err).Str("job_id", jobID.String()).Msg("local_watcher: failed to update download record")
		return
	}

	if err := w.updateJobStatus(ctx, jobID, "download_complete", ""); err != nil {
		w.logger.Error().Err(err).Str("job_id", jobID.String()).Msg("local_watcher: failed to transition to download_complete")
		return
	}

	_ = w.emitEvent(ctx, jobID, "download_complete", map[string]string{
		"source_path": sourcePath,
	})

	w.mu.Lock()
	delete(w.sizeHistory, jobKey)
	w.mu.Unlock()
}

// --- Helpers ---

// tokenOverlap returns an integer percentage (0–100) of how many normalised
// title tokens appear in the normalised entry name.
// TokenOverlap returns the percentage (0–100) of title tokens that appear in entryName.
func TokenOverlap(entryName, title string) int { return tokenOverlap(entryName, title) }

func tokenOverlap(entryName, title string) int {
	titleTokens := normalizeTokens(title)
	entryTokens := normalizeTokens(entryName)

	if len(titleTokens) == 0 {
		return 0
	}

	entrySet := make(map[string]bool, len(entryTokens))
	for _, t := range entryTokens {
		entrySet[t] = true
	}

	matches := 0
	for _, t := range titleTokens {
		if entrySet[t] {
			matches++
		}
	}

	return matches * 100 / len(titleTokens)
}

// normalizeTokens lowercases s, replaces non-alphanumeric runes with spaces,
// and returns the resulting tokens.
func normalizeTokens(s string) []string {
	mapped := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)
	return strings.Fields(mapped)
}

// totalSize returns the total byte size of path. If path is a directory it
// sums all contained files recursively.
func totalSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}
	var total int64
	err = filepath.Walk(path, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

// isSizeStable returns true if history contains snapshots spanning at least
// minSecs seconds and all snapshots show the same size as current.
func isSizeStable(history []sizeSnapshot, minSecs int, current int64) bool {
	if current == 0 {
		return false
	}
	if len(history) < 2 {
		return false
	}
	span := history[len(history)-1].at.Sub(history[0].at)
	if span < time.Duration(minSecs)*time.Second {
		return false
	}
	for _, s := range history {
		if s.size != current {
			return false
		}
	}
	return true
}

// findJDPackage returns whether a JD package matching folderName (case-insensitive)
// was found, and if so, whether it reports Finished == true.
func findJDPackage(packages []myjdownloader.Package, folderName string) (found, finished bool) {
	lower := strings.ToLower(folderName)
	for _, p := range packages {
		if strings.ToLower(p.Name) == lower {
			return true, p.Finished
		}
	}
	return false, false
}
