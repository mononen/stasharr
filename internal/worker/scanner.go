package worker

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

// ScanWorker triggers StashApp to scan imported files.
type ScanWorker struct {
	Base
	stashapp *stashapp.Client
}

func NewScanWorker(app *models.App, logger zerolog.Logger) *ScanWorker {
	return &ScanWorker{
		Base:     Base{db: app.DB, config: app.Config, logger: logger},
		stashapp: app.StashApp,
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

	stashInstance, err := q.GetDefaultStashInstance(ctx)
	if err != nil {
		msg := "no default Stash instance configured"
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", msg)
		return errors.New(msg)
	}

	// Build a per-request client using the stored instance credentials.
	client := stashapp.New(stashInstance.Url, stashInstance.ApiKey)

	exists, err := client.FindSceneByPath(ctx, finalPath)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "scan_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "scan_failed", map[string]string{"error": err.Error()})
		return err
	}
	if exists {
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

	_ = w.updateJobStatus(ctx, job.ID, "complete", "")
	_ = w.emitEvent(ctx, job.ID, "scan_complete", nil)
	_ = w.emitEvent(ctx, job.ID, "job_complete", map[string]string{"final_path": finalPath})

	return nil
}
