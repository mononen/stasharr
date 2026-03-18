package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/matcher"
	"github.com/mononen/stasharr/internal/models"
)

// MoveWorker relocates completed downloads to their final path.
type MoveWorker struct {
	Base
}

func NewMoveWorker(app *models.App, logger zerolog.Logger) *MoveWorker {
	return &MoveWorker{
		Base: Base{db: app.DB, config: app.Config, logger: logger},
	}
}

func (w *MoveWorker) Name() string { return "move" }

func (w *MoveWorker) Start(ctx context.Context) {
	for {
		job, err := w.claimJob(ctx, "download_complete", "moving")
		if err != nil {
			w.logger.Error().Err(err).Msg("move: claimJob error")
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}

		if job == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		if err := w.process(ctx, job); err != nil {
			w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("move: process error")
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}
}

func (w *MoveWorker) Stop() {}

func (w *MoveWorker) process(ctx context.Context, job *models.Job) error {
	q := queries.New(w.db)

	scene, err := q.GetSceneByJobID(ctx, job.ID)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": err.Error()})
		return err
	}

	download, err := q.GetDownloadByJobID(ctx, job.ID)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": err.Error()})
		return err
	}

	if !download.SourcePath.Valid {
		msg := "no source path"
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", msg)
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": msg})
		return errors.New(msg)
	}
	sourcePath := download.SourcePath.String

	// Apply remote→local path mapping for cross-container mounts.
	if remotePath := w.config.Get("sabnzbd.remote_path"); remotePath != "" {
		if localPath := w.config.Get("sabnzbd.local_path"); localPath != "" {
			if strings.HasPrefix(sourcePath, remotePath) {
				sourcePath = localPath + strings.TrimPrefix(sourcePath, remotePath)
			}
		}
	}

	// Read config values.
	tmpl := w.config.Get("directory.template")
	if tmpl == "" {
		msg := "directory.template is not configured"
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", msg)
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": msg})
		return errors.New(msg)
	}

	missingValue := w.config.Get("directory.missing_field_value")
	if missingValue == "" {
		missingValue = "Unknown"
	}

	performerMax := 3
	if pmStr := w.config.Get("directory.performer_max"); pmStr != "" {
		if n, err := strconv.Atoi(pmStr); err == nil {
			performerMax = n
		}
	}

	libraryPath := w.config.Get("stash.library_path")
	if libraryPath == "" {
		msg := "stash.library_path is not configured"
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", msg)
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": msg})
		return errors.New(msg)
	}

	// Find video file.
	var videoFilePath string
	info, err := os.Stat(sourcePath)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": err.Error()})
		return err
	}

	if !info.IsDir() && isVideoFile(sourcePath) {
		videoFilePath = sourcePath
	} else if info.IsDir() {
		videoFilePath, err = findLargestVideoFile(sourcePath)
		if err != nil {
			_ = w.updateJobStatus(ctx, job.ID, "move_failed", err.Error())
			_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": err.Error()})
			return err
		}
	}

	if videoFilePath == "" {
		msg := "no video file found"
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", msg)
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": msg})
		return errors.New(msg)
	}

	filename := filepath.Base(videoFilePath)

	relPath, err := matcher.Render(tmpl, scene, filename, missingValue, performerMax)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": err.Error()})
		return err
	}

	destPath := filepath.Join(libraryPath, relPath)

	_ = w.emitEvent(ctx, job.ID, "move_started", map[string]string{
		"source":      videoFilePath,
		"destination": destPath,
	})

	// Deduplicate destination path if necessary.
	destPath = deduplicatePath(destPath)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": err.Error()})
		return err
	}

	// Attempt rename first; fall back to copy+verify+delete on any error.
	if err := os.Rename(videoFilePath, destPath); err != nil {
		if copyErr := crossFSCopy(videoFilePath, destPath); copyErr != nil {
			_ = w.updateJobStatus(ctx, job.ID, "move_failed", copyErr.Error())
			_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": copyErr.Error()})
			return copyErr
		}
	}

	// Update DB with final path.
	if _, err := q.UpdateDownloadFinalPath(ctx, queries.UpdateDownloadFinalPathParams{
		FinalPath: pgtype.Text{String: destPath, Valid: true},
		ID:        download.ID,
	}); err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "move_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "move_failed", map[string]string{"error": err.Error()})
		return err
	}

	_ = w.updateJobStatus(ctx, job.ID, "moved", "")
	_ = w.emitEvent(ctx, job.ID, "move_complete", map[string]string{"final_path": destPath})

	// Clean up non-video files from the source directory.
	if info.IsDir() {
		entries, err := os.ReadDir(sourcePath)
		if err == nil {
			for _, entry := range entries {
				entryPath := filepath.Join(sourcePath, entry.Name())
				if entryPath != videoFilePath {
					_ = os.RemoveAll(entryPath)
				}
			}
		}
	}

	return nil
}

// videoExtensions is the set of recognised video file extensions.
var videoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".wmv": true, ".mov": true,
}

// isVideoFile reports whether path has a known video extension.
func isVideoFile(path string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(path))]
}

// findLargestVideoFile walks dir and returns the path of the largest video file.
func findLargestVideoFile(dir string) (string, error) {
	var bestPath string
	var bestSize int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isVideoFile(path) {
			if info.Size() > bestSize {
				bestSize = info.Size()
				bestPath = path
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return bestPath, nil
}

// deduplicatePath returns path unchanged if it does not exist. Otherwise it
// appends _1, _2, … before the extension until a free name is found.
func deduplicatePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// crossFSCopy copies src to dst, verifies sizes match, then removes src.
func crossFSCopy(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	if err := dstFile.Close(); err != nil {
		return err
	}
	srcFile.Close()

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return err
	}
	if srcInfo.Size() != dstInfo.Size() {
		return fmt.Errorf("copy verification failed: src size %d != dst size %d", srcInfo.Size(), dstInfo.Size())
	}

	return os.Remove(src)
}
