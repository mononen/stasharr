package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/matcher"
	"github.com/mononen/stasharr/internal/models"
)

// applyThreshold maps a score to a disposition string based on the configured
// auto-approval and review thresholds.
//
//   - score >= autoThreshold  → "auto_approved"
//   - score >= reviewThreshold → "awaiting_review"
//   - score < reviewThreshold  → "search_failed"
func applyThreshold(topScore, autoThreshold, reviewThreshold int) string {
	switch {
	case topScore >= autoThreshold:
		return "auto_approved"
	case topScore >= reviewThreshold:
		return "awaiting_review"
	default:
		return "search_failed"
	}
}

// SearchWorker queries Prowlarr for NZB results.
type SearchWorker struct {
	Base
	prowlarr *prowlarr.Client
}

func NewSearchWorker(app *models.App, logger zerolog.Logger) *SearchWorker {
	return &SearchWorker{
		Base:     Base{db: app.DB, config: app.Config, logger: logger},
		prowlarr: app.Prowlarr,
	}
}

func (w *SearchWorker) Name() string { return "search" }

func (w *SearchWorker) Start(ctx context.Context) {
	for {
		job, err := w.claimJob(ctx, "resolved", "searching")
		if err != nil {
			w.logger.Error().Err(err).Msg("search: claim job error")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
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

		w.process(ctx, job)
	}
}

func (w *SearchWorker) Stop() {}

func (w *SearchWorker) process(ctx context.Context, job *models.Job) {
	// Emit a preliminary search_started before we know the query.
	if err := w.emitEvent(ctx, job.ID, "search_started", nil); err != nil {
		w.logger.Error().Err(err).Msg("search: emit search_started")
	}

	scene, err := queries.New(w.db).GetSceneByJobID(ctx, job.ID)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "search_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "search_failed", map[string]string{"error": err.Error()})
		return
	}

	// Build search query.
	query := scene.Title
	if scene.StudioName.Valid && scene.StudioName.String != "" {
		query = fmt.Sprintf("%s %s", scene.Title, scene.StudioName.String)
	}

	// Re-emit with the actual query string.
	if err := w.emitEvent(ctx, job.ID, "search_started", map[string]string{"query": query}); err != nil {
		w.logger.Error().Err(err).Msg("search: emit search_started with query")
	}

	// Read search limit from config; default 10.
	limit := 10
	if raw := w.config.Get("prowlarr.search_limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}

	results, err := w.prowlarr.Search(ctx, query, limit)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "search_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "search_failed", map[string]string{"error": err.Error()})
		return
	}

	if len(results) == 0 {
		msg := "No results found across all indexers"
		_ = w.updateJobStatus(ctx, job.ID, "search_failed", msg)
		_ = w.emitEvent(ctx, job.ID, "search_failed", map[string]string{"error": msg})
		return
	}

	// Load studio aliases and build normalised map.
	aliasRows, err := queries.New(w.db).ListAliases(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("search: list aliases")
	}
	aliasMap := make(map[string]string, len(aliasRows))
	for _, a := range aliasRows {
		aliasMap[matcher.NormalizeString(a.Alias)] = matcher.NormalizeString(a.Canonical)
	}

	// Convert prowlarr.Result → matcher.ProwlarrResult.
	matcherResults := make([]matcher.ProwlarrResult, len(results))
	for i, r := range results {
		matcherResults[i] = matcher.ProwlarrResult{
			Title:       r.Title,
			SizeBytes:   r.SizeBytes,
			PublishDate: r.PublishDate,
			IndexerName: r.IndexerName,
			DownloadURL: r.DownloadURL,
			NzbID:       r.NzbID,
			InfoURL:     r.InfoURL,
		}
	}

	scored := matcher.ScoreResults(scene, matcherResults, aliasMap)

	// Persist all scored results.
	persistedIDs := make([]queries.SearchResult, len(scored))
	for i, r := range scored {
		breakdownJSON, err := json.Marshal(r.Breakdown)
		if err != nil {
			w.logger.Error().Err(err).Msg("search: marshal score breakdown")
			breakdownJSON = []byte("{}")
		}

		var publishDate pgtype.Timestamptz
		if r.Result.PublishDate != nil {
			publishDate = pgtype.Timestamptz{Time: *r.Result.PublishDate, Valid: true}
		}

		sr, err := queries.New(w.db).CreateSearchResult(ctx, queries.CreateSearchResultParams{
			JobID:           job.ID,
			IndexerName:     r.Result.IndexerName,
			ReleaseTitle:    r.Result.Title,
			SizeBytes:       pgtype.Int8{Int64: r.Result.SizeBytes, Valid: r.Result.SizeBytes > 0},
			PublishDate:     publishDate,
			DownloadUrl:     pgtype.Text{String: r.Result.DownloadURL, Valid: r.Result.DownloadURL != ""},
			NzbID:           pgtype.Text{String: r.Result.NzbID, Valid: r.Result.NzbID != ""},
			InfoUrl:         pgtype.Text{String: r.Result.InfoURL, Valid: r.Result.InfoURL != ""},
			ConfidenceScore: int32(r.Score),
			ScoreBreakdown:  breakdownJSON,
		})
		if err != nil {
			w.logger.Error().Err(err).Msg("search: create search result")
		}
		persistedIDs[i] = sr
	}

	// Read scoring thresholds from config; defaults: auto=85, review=50.
	autoThreshold := 85
	reviewThreshold := 50
	if raw := w.config.Get("matching.auto_threshold"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			autoThreshold = n
		}
	}
	if raw := w.config.Get("matching.review_threshold"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			reviewThreshold = n
		}
	}

	topScore := scored[0].Score
	disposition := applyThreshold(topScore, autoThreshold, reviewThreshold)

	switch disposition {
	case "search_failed":
		msg := "Results found but no confident matches"
		_ = w.updateJobStatus(ctx, job.ID, "search_failed", msg)
		_ = w.emitEvent(ctx, job.ID, "search_failed", map[string]string{"error": msg})

	case "auto_approved":
		selected, err := queries.New(w.db).SelectSearchResult(ctx, queries.SelectSearchResultParams{
			SelectedBy: pgtype.Text{String: "auto", Valid: true},
			ID:         persistedIDs[0].ID,
		})
		if err != nil {
			w.logger.Error().Err(err).Msg("search: select search result")
		}
		_ = w.updateJobStatus(ctx, job.ID, "approved", "")
		_ = w.emitEvent(ctx, job.ID, "auto_approved", map[string]any{
			"result_id": selected.ID,
			"score":     topScore,
		})

	case "awaiting_review":
		_ = w.updateJobStatus(ctx, job.ID, "awaiting_review", "")
		_ = w.emitEvent(ctx, job.ID, "sent_to_review", map[string]any{
			"result_count": len(scored),
			"top_score":    topScore,
		})
	}
}
