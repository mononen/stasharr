package worker

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/config"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/matcher"
)

// RunSearch executes a Prowlarr search for the given query string, scores results
// against the scene, persists them to the DB, updates the job status, and emits
// events. It is used by both SearchWorker and the custom-search API handler.
func RunSearch(
	ctx context.Context,
	db *pgxpool.Pool,
	prowlarrClient *prowlarr.Client,
	cfg *config.Config,
	jobID uuid.UUID,
	query string,
) error {
	q := queries.New(db)

	scene, err := q.GetSceneByJobID(ctx, jobID)
	if err != nil {
		_ = emitJobEvent(ctx, db, jobID, "search_failed", map[string]string{"error": err.Error()})
		_, _ = db.Exec(ctx,
			`UPDATE jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
			"search_failed", err.Error(), jobID,
		)
		return err
	}

	_ = emitJobEvent(ctx, db, jobID, "search_started", map[string]string{"query": query})

	limit := 10
	if raw := cfg.Get("prowlarr.search_limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}

	results, err := prowlarrClient.Search(ctx, query, limit)
	if err != nil {
		_ = emitJobEvent(ctx, db, jobID, "search_failed", map[string]string{"error": err.Error()})
		_, _ = db.Exec(ctx,
			`UPDATE jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
			"search_failed", err.Error(), jobID,
		)
		return err
	}

	if len(results) == 0 {
		msg := "No results found across all indexers"
		_ = emitJobEvent(ctx, db, jobID, "search_failed", map[string]string{"error": msg})
		_, _ = db.Exec(ctx,
			`UPDATE jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
			"search_failed", msg, jobID,
		)
		return nil
	}

	_ = emitJobEvent(ctx, db, jobID, "results_found", map[string]any{"count": len(results)})

	// Load studio aliases.
	aliasRows, err := q.ListAliases(ctx)
	if err == nil {
		// non-fatal
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
			breakdownJSON = []byte("{}")
		}

		var publishDate pgtype.Timestamptz
		if r.Result.PublishDate != nil {
			publishDate = pgtype.Timestamptz{Time: *r.Result.PublishDate, Valid: true}
		}

		sr, err := q.CreateSearchResult(ctx, queries.CreateSearchResultParams{
			JobID:           jobID,
			IndexerName:     r.Result.IndexerName,
			ReleaseTitle:    r.Result.Title,
			SizeBytes:       pgtype.Int8{Int64: r.Result.SizeBytes, Valid: r.Result.SizeBytes > 0},
			PublishDate:     publishDate,
			DownloadUrl:     pgtype.Text{String: r.Result.DownloadURL, Valid: r.Result.DownloadURL != ""},
			NzbID:           pgtype.Text{String: r.Result.NzbID, Valid: r.Result.NzbID != ""},
			ConfidenceScore: int32(r.Score),
			ScoreBreakdown:  breakdownJSON,
		})
		if err == nil {
			persistedIDs[i] = sr
		}
	}

	// Read thresholds from config.
	autoThreshold := 85
	reviewThreshold := 50
	if raw := cfg.Get("matching.auto_threshold"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			autoThreshold = n
		}
	}
	if raw := cfg.Get("matching.review_threshold"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			reviewThreshold = n
		}
	}

	// Parse preferred resolutions.
	var preferredResolutions []string
	if raw := cfg.Get("matching.preferred_resolutions"); raw != "" {
		for _, r := range splitTrim(raw, ",") {
			if r != "" {
				preferredResolutions = append(preferredResolutions, r)
			}
		}
	}

	topScore := scored[0].Score
	_ = emitJobEvent(ctx, db, jobID, "scoring_complete", map[string]any{
		"result_count": len(scored),
		"top_score":    topScore,
		"top_result":   scored[0].Result.Title,
	})
	disposition := applyThreshold(topScore, autoThreshold, reviewThreshold)

	switch disposition {
	case "search_failed":
		msg := "Results found but no confident matches"
		_, _ = db.Exec(ctx,
			`UPDATE jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
			"search_failed", msg, jobID,
		)
		_ = emitJobEvent(ctx, db, jobID, "search_failed", map[string]string{"error": msg})

	case "auto_approved":
		bestIdx := selectAutoResult(scored, persistedIDs, preferredResolutions, autoThreshold)
		selected, err := q.SelectSearchResult(ctx, queries.SelectSearchResultParams{
			SelectedBy: pgtype.Text{String: "auto", Valid: true},
			ID:         persistedIDs[bestIdx].ID,
		})
		if err == nil {
			_ = emitJobEvent(ctx, db, jobID, "auto_approved", map[string]any{
				"result_id":  selected.ID,
				"score":      scored[bestIdx].Score,
				"resolution": matcher.ExtractResolution(scored[bestIdx].Result.Title),
			})
		}
		_, _ = db.Exec(ctx,
			`UPDATE jobs SET status = $1, error_message = NULL, updated_at = NOW() WHERE id = $2`,
			"approved", jobID,
		)

	case "awaiting_review":
		_, _ = db.Exec(ctx,
			`UPDATE jobs SET status = $1, error_message = NULL, updated_at = NOW() WHERE id = $2`,
			"awaiting_review", jobID,
		)
		_ = emitJobEvent(ctx, db, jobID, "sent_to_review", map[string]any{
			"result_count": len(scored),
			"top_score":    topScore,
		})
	}

	return nil
}
