package worker

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

// ScanWorker triggers StashApp to scan imported files.
type ScanWorker struct {
	Base
	stashapp *stashapp.Client
	sabnzbd  *sabnzbd.Client
}

func NewScanWorker(app *models.App, logger zerolog.Logger) *ScanWorker {
	return &ScanWorker{
		Base:     Base{db: app.DB, config: app.Config, logger: logger},
		stashapp: app.StashApp,
		sabnzbd:  app.SABnzbd,
	}
}

func (w *ScanWorker) Name() string { return "scan" }

func (w *ScanWorker) Start(ctx context.Context) {
	for {
		job, err := w.claimJob(ctx, "moved", "scanning")
		if err != nil {
			w.logger.Error().Err(err).Msg("scan: claimJob error")
			select {
			case <-ctx.Done():
				return
			case <-time.After(60 * time.Second):
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
			w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("scan: process error")
			select {
			case <-ctx.Done():
				return
			case <-time.After(60 * time.Second):
			}
		}
	}
}

func (w *ScanWorker) Stop() {}

func (w *ScanWorker) process(ctx context.Context, job *models.Job) error {
	q := queries.New(w.db)

	download, err := q.GetDownloadByJobID(ctx, job.ID)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", err.Error())
		return err
	}

	if !download.FinalPath.Valid {
		msg := "no final path"
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", msg)
		return errors.New(msg)
	}
	finalPath := download.FinalPath.String

	scene, err := q.GetSceneByJobID(ctx, job.ID)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", err.Error())
		return err
	}

	stashInstance, err := q.GetDefaultStashInstance(ctx)
	if err != nil {
		msg := "no default Stash instance configured"
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", msg)
		return errors.New(msg)
	}

	// Build a per-request client using the stored instance credentials.
	client := stashapp.New(stashInstance.Url, stashInstance.ApiKey)

	sceneID, err := client.FindSceneByPath(ctx, finalPath)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "scan_failed", map[string]string{"error": err.Error()})
		return err
	}
	if sceneID != "" {
		w.scrapeAndGenerate(ctx, job.ID, client, sceneID, scene.StashdbSceneID)
		w.cleanupSABnzbd(ctx, job.ID, download.SabnzbdNzoID)
		_ = w.updateJobStatus(ctx, job.ID, "complete", "")
		_ = w.emitEvent(ctx, job.ID, "scan_complete", nil)
		_ = w.emitEvent(ctx, job.ID, "job_complete", map[string]string{"final_path": finalPath})
		return nil
	}

	_ = w.emitEvent(ctx, job.ID, "scan_triggered", map[string]string{
		"stash_instance": stashInstance.Name,
		"path":           finalPath,
	})

	if err := client.TriggerScan(ctx, finalPath); err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "scan_failed", map[string]string{"error": err.Error()})
		return err
	}

	// Poll until Stash registers the file, then scrape and generate.
	const maxAttempts = 60
	const pollInterval = 5 * time.Second
	found := false
	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		sceneID, err = client.FindSceneByPath(ctx, finalPath)
		if err != nil {
			w.logger.Warn().Err(err).Msg("scan: poll error")
			_ = w.emitEvent(ctx, job.ID, "scan_poll_error", map[string]string{"error": err.Error()})
			continue
		}
		if sceneID != "" {
			found = true
			w.scrapeAndGenerate(ctx, job.ID, client, sceneID, scene.StashdbSceneID)
			w.cleanupSABnzbd(ctx, job.ID, download.SabnzbdNzoID)
			break
		}
	}

	if !found {
		msg := "scene not found in Stash after scan — path mismatch or scan did not complete"
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", msg)
		_ = w.emitEvent(ctx, job.ID, "scan_timeout", map[string]string{"path": finalPath})
		return errors.New(msg)
	}

	_ = w.updateJobStatus(ctx, job.ID, "complete", "")
	_ = w.emitEvent(ctx, job.ID, "scan_complete", nil)
	_ = w.emitEvent(ctx, job.ID, "job_complete", map[string]string{"final_path": finalPath})

	return nil
}

// scrapeAndGenerate attaches the StashDB ID, queues a metadata identify task
// (which creates missing performers/studios/tags and applies all metadata), and
// triggers phash generation for the scene.
func (w *ScanWorker) scrapeAndGenerate(ctx context.Context, jobID uuid.UUID, client *stashapp.Client, stashSceneID, stashdbSceneID string) {
	// Attach the stash_id first so the scene is linked even if identify fails.
	if err := client.UpdateSceneStashID(ctx, stashSceneID, stashdbSceneID); err != nil {
		w.logger.Warn().Err(err).Str("stash_scene_id", stashSceneID).Msg("scan: failed to attach stash_id")
	} else {
		_ = w.emitEvent(ctx, jobID, "stash_id_attached", map[string]string{"stash_scene_id": stashSceneID})
	}

	if err := client.RunIdentify(ctx, stashSceneID); err != nil {
		w.logger.Warn().Err(err).Str("stash_scene_id", stashSceneID).Msg("scan: identify failed")
		_ = w.emitEvent(ctx, jobID, "identify_failed", map[string]string{"error": err.Error()})
	} else {
		_ = w.emitEvent(ctx, jobID, "identify_queued", nil)
	}

	if err := client.GeneratePhash(ctx, stashSceneID); err != nil {
		w.logger.Warn().Err(err).Str("stash_scene_id", stashSceneID).Msg("scan: phash generation failed")
	} else {
		_ = w.emitEvent(ctx, jobID, "phash_queued", nil)
	}
}

// cleanupSABnzbd removes the completed download from SABnzbd history and deletes its files from disk.
func (w *ScanWorker) cleanupSABnzbd(ctx context.Context, jobID uuid.UUID, nzoID string) {
	if err := w.sabnzbd.DeleteHistoryItem(ctx, nzoID); err != nil {
		w.logger.Warn().Err(err).Str("nzo_id", nzoID).Msg("scan: failed to delete SABnzbd history item")
	} else {
		_ = w.emitEvent(ctx, jobID, "sabnzbd_cleaned_up", nil)
	}
}
