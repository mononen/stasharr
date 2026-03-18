package worker

import (
	"context"
	"errors"
	"net/url"
	"time"

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

// isRedirectURL returns true if downloadURL does not share the same host as the
// configured Prowlarr base URL. When Prowlarr's "redirect" toggle is enabled for
// an indexer, the download URL points directly to the indexer rather than through
// Prowlarr's proxy — fetching it via Prowlarr would fail for strict indexers.
func (w *DownloadWorker) isRedirectURL(downloadURL string) bool {
	prowlarrBase := w.config.Get("prowlarr.url")
	pb, err := url.Parse(prowlarrBase)
	if err != nil || pb.Host == "" {
		return false
	}
	du, err := url.Parse(downloadURL)
	if err != nil {
		return false
	}
	return du.Host != pb.Host
}

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

	var nzoID string
	if w.isRedirectURL(result.DownloadUrl.String) {
		w.logger.Debug().Str("download_url", result.DownloadUrl.String).Msg("download: redirect URL detected, submitting directly to SABnzbd")
		_ = w.emitEvent(ctx, job.ID, "nzb_submitting", map[string]string{"method": "direct_url"})
		nzoID, err = w.sabnzbd.SubmitNZBURL(ctx, result.DownloadUrl.String, result.ReleaseTitle)
		if err != nil {
			return fail("submit NZB URL", err)
		}
	} else {
		var nzbBytes []byte
		_ = w.emitEvent(ctx, job.ID, "nzb_fetching", nil)
		nzbBytes, err = w.prowlarr.FetchNZB(ctx, result.DownloadUrl.String)
		if err != nil {
			return fail("fetch NZB", err)
		}
		_ = w.emitEvent(ctx, job.ID, "nzb_fetched", map[string]any{"size_bytes": len(nzbBytes)})
		_ = w.emitEvent(ctx, job.ID, "nzb_submitting", map[string]string{"method": "prowlarr_proxy"})
		nzoID, err = w.sabnzbd.SubmitNZB(ctx, nzbBytes, result.ReleaseTitle)
		if err != nil {
			return fail("submit NZB", err)
		}
	}

	// Upsert: if a download record already exists for this job (e.g. a retry
	// after a restart), update the NZO ID rather than failing on the unique constraint.
	if _, err = w.db.Exec(ctx,
		`INSERT INTO downloads (job_id, sabnzbd_nzo_id, status)
		 VALUES ($1, $2, 'queued')
		 ON CONFLICT (job_id) DO UPDATE SET sabnzbd_nzo_id = EXCLUDED.sabnzbd_nzo_id,
		                                    status = 'queued',
		                                    filename = NULL,
		                                    source_path = NULL,
		                                    final_path = NULL,
		                                    updated_at = NOW()`,
		job.ID, nzoID,
	); err != nil {
		w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("download: failed to upsert download record")
	}

	if err := w.emitEvent(ctx, job.ID, "download_submitted", map[string]string{
		"nzo_id":        nzoID,
		"release_title": result.ReleaseTitle,
	}); err != nil {
		w.logger.Error().Err(err).Str("job_id", job.ID.String()).Msg("download: failed to emit download_submitted event")
	}

	return nil
}
