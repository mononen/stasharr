package worker

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

const defaultMonitorPollInterval = 30

// MonitorWorker polls SABnzbd for download progress and updates job/download
// statuses accordingly.
type MonitorWorker struct {
	Base
	sabnzbd *sabnzbd.Client
}

func NewMonitorWorker(app *models.App, logger zerolog.Logger) *MonitorWorker {
	return &MonitorWorker{
		Base:    Base{db: app.DB, config: app.Config, logger: logger},
		sabnzbd: app.SABnzbd,
	}
}

func (w *MonitorWorker) Name() string { return "monitor" }

func (w *MonitorWorker) Start(ctx context.Context) {
	intervalSecs := defaultMonitorPollInterval
	if raw := w.config.Get("pipeline.monitor_poll_interval"); raw != "" {
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

func (w *MonitorWorker) Stop() {}

func (w *MonitorWorker) tick(ctx context.Context) {
	jobs, err := queries.New(w.db).ListJobs(ctx, queries.ListJobsParams{
		Status:     pgtype.Text{String: "downloading", Valid: true},
		Type:       pgtype.Text{},
		MaxResults: 1000,
	})
	if err != nil {
		w.logger.Error().Err(err).Msg("monitor: failed to list downloading jobs")
		return
	}
	if len(jobs) == 0 {
		return
	}

	queueItems, err := w.sabnzbd.GetQueue(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("monitor: failed to get SABnzbd queue")
		return
	}

	historyItems, err := w.sabnzbd.GetHistory(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("monitor: failed to get SABnzbd history")
		return
	}

	queueByNzo := make(map[string]sabnzbd.QueueItem, len(queueItems))
	for _, item := range queueItems {
		queueByNzo[item.NzoID] = item
	}

	historyByNzo := make(map[string]sabnzbd.HistoryItem, len(historyItems))
	for _, item := range historyItems {
		historyByNzo[item.NzoID] = item
	}

	for _, job := range jobs {
		if err := w.processJob(ctx, job, queueByNzo, historyByNzo); err != nil {
			w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: error processing job")
		}
	}
}

func (w *MonitorWorker) processJob(
	ctx context.Context,
	job models.Job,
	queueByNzo map[string]sabnzbd.QueueItem,
	historyByNzo map[string]sabnzbd.HistoryItem,
) error {
	download, err := queries.New(w.db).GetDownloadByJobID(ctx, job.ID)
	if err != nil {
		return err
	}

	nzoID := download.SabnzbdNzoID

	if qItem, inQueue := queueByNzo[nzoID]; inQueue {
		status := mapSABnzbdQueueStatus(qItem.Status)
		if status != download.Status {
			if _, err := queries.New(w.db).UpdateDownloadStatus(ctx, queries.UpdateDownloadStatusParams{
				Status: status,
				ID:     download.ID,
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to update download status from queue")
			}
			switch status {
			case "queued":
				_ = w.emitEvent(ctx, job.ID, "download_queued", nil)
			case "verifying":
				_ = w.emitEvent(ctx, job.ID, "download_verifying", nil)
			case "repairing":
				_ = w.emitEvent(ctx, job.ID, "download_repairing", nil)
			case "unpacking":
				_ = w.emitEvent(ctx, job.ID, "download_unpacking", nil)
			}
		}

		if status == "downloading" {
			if err := w.emitEvent(ctx, job.ID, "download_progress", map[string]string{
				"percentage": qItem.Percentage,
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to emit download_progress event")
			}
		}
		return nil
	}

	if hItem, inHistory := historyByNzo[nzoID]; inHistory {
		switch hItem.Status {
		case "Completed":
			if _, err := queries.New(w.db).UpdateDownloadComplete(ctx, queries.UpdateDownloadCompleteParams{
				Filename:   pgtype.Text{String: hItem.Name, Valid: hItem.Name != ""},
				SourcePath: pgtype.Text{String: hItem.StoragePath, Valid: hItem.StoragePath != ""},
				ID:         download.ID,
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to update download complete")
			}

			if _, err := queries.New(w.db).UpdateDownloadStatus(ctx, queries.UpdateDownloadStatusParams{
				Status: "complete",
				ID:     download.ID,
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to set download status to complete")
			}

			if err := w.updateJobStatus(ctx, job.ID, "download_complete", ""); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to update job status to download_complete")
			}

			if err := w.emitEvent(ctx, job.ID, "download_complete", map[string]string{
				"filename":    hItem.Name,
				"source_path": hItem.StoragePath,
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to emit download_complete event")
			}

		case "Failed":
			if _, err := queries.New(w.db).UpdateDownloadStatus(ctx, queries.UpdateDownloadStatusParams{
				Status: "failed",
				ID:     download.ID,
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to set download status to failed")
			}

			if err := w.updateJobStatus(ctx, job.ID, "download_failed", "SABnzbd reported failure"); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to update job status to download_failed")
			}

			if err := w.emitEvent(ctx, job.ID, "download_failed", map[string]string{
				"error": "SABnzbd reported failure",
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to emit download_failed event")
			}

		default:
			status := mapSABnzbdQueueStatus(hItem.Status)
			if _, err := queries.New(w.db).UpdateDownloadStatus(ctx, queries.UpdateDownloadStatusParams{
				Status: status,
				ID:     download.ID,
			}); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to update download status from history")
			}
		}

		return nil
	}

	// NZO ID not found in queue or history.
	const notFoundMsg = "NZO ID not found in SABnzbd queue or history"

	if _, err := queries.New(w.db).UpdateDownloadStatus(ctx, queries.UpdateDownloadStatusParams{
		Status: "failed",
		ID:     download.ID,
	}); err != nil {
		w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to set download status to failed (not found)")
	}

	if err := w.updateJobStatus(ctx, job.ID, "download_failed", notFoundMsg); err != nil {
		w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to update job status (not found)")
	}

	if err := w.emitEvent(ctx, job.ID, "download_failed", map[string]string{
		"error": notFoundMsg,
	}); err != nil {
		w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("monitor: failed to emit download_failed event (not found)")
	}

	return nil
}

// mapSABnzbdQueueStatus maps SABnzbd status strings to internal download statuses.
func mapSABnzbdQueueStatus(status string) string {
	switch status {
	case "Queued":
		return "queued"
	case "Downloading":
		return "downloading"
	case "Verifying":
		return "verifying"
	case "Repairing":
		return "repairing"
	case "Extracting":
		return "unpacking"
	default:
		return "queued"
	}
}
