package worker

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

// DownloadWorker claims approved jobs, fetches NZB files via Prowlarr,
// and submits them to SABnzbd.
type DownloadWorker struct {
	Base
	prowlarr *prowlarr.Client
	sabnzbd  *sabnzbd.Client
}

func NewDownloadWorker(app *models.App, logger zerolog.Logger) *DownloadWorker {
	return &DownloadWorker{
		Base:     Base{db: app.DB, config: app.Config, logger: logger},
		prowlarr: app.Prowlarr,
		sabnzbd:  app.SABnzbd,
	}
}

func (w *DownloadWorker) Name() string { return "download" }

func (w *DownloadWorker) Start(ctx context.Context) {
	for {
		job, err := w.claimJob(ctx, "approved", "downloading")
		if err != nil {
			w.logger.Error().Err(err).Msg("download: failed to claim job")
		} else if job != nil {
			if err := w.process(ctx, job); err != nil {
				w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("download: job processing failed")
			}
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (w *DownloadWorker) Stop() {}

func (w *DownloadWorker) process(ctx context.Context, job *models.Job) error {
	fail := func(msg string, err error) error {
		_ = w.updateJobStatus(ctx, job.ID, "download_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "download_failed", map[string]string{"error": err.Error()})
		return err
	}

	result, err := queries.New(w.db).GetSelectedResultByJobID(ctx, job.ID)
	if err != nil {
		return fail("get selected result", err)
	}

	if !result.DownloadUrl.Valid || result.DownloadUrl.String == "" {
		return fail("no download URL", errors.New("no download URL for selected result"))
	}

	nzbBytes, err := w.prowlarr.FetchNZB(ctx, result.DownloadUrl.String)
	if err != nil {
		return fail("fetch NZB", err)
	}

	nzoID, err := w.sabnzbd.SubmitNZB(ctx, nzbBytes, result.ReleaseTitle)
	if err != nil {
		return fail("submit NZB", err)
	}

	_, err = queries.New(w.db).CreateDownload(ctx, queries.CreateDownloadParams{
		JobID:        job.ID,
		SabnzbdNzoID: nzoID,
		SizeBytes:    pgtype.Int8{},
	})
	if err != nil {
		w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("download: failed to create download record")
	}

	if err := w.emitEvent(ctx, job.ID, "download_submitted", map[string]string{
		"nzo_id":        nzoID,
		"release_title": result.ReleaseTitle,
	}); err != nil {
		w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("download: failed to emit download_submitted event")
	}

	return nil
}
