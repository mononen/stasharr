package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
	"github.com/mononen/stasharr/internal/worker"
)

var (
	sceneURLRe     = regexp.MustCompile(`^https://stashdb\.org/scenes/([a-zA-Z0-9-]+)$`)
	performerURLRe = regexp.MustCompile(`^https://stashdb\.org/performers/([a-zA-Z0-9-]+)$`)
	studioURLRe    = regexp.MustCompile(`^https://stashdb\.org/studios/([a-zA-Z0-9-]+)$`)
)

var validJobTypes = map[string]bool{
	"scene":     true,
	"performer": true,
	"studio":    true,
}

// retryTargetStatus maps a failed or stuck in-progress status to the status
// we re-queue from. In-progress entries allow force-resetting a stuck job.
var retryTargetStatus = map[string]string{
	// Failed → retry from prior state
	"resolve_failed":  "submitted",
	"search_failed":   "resolved",
	"download_failed": "approved",
	"move_failed":     "download_complete",
	"scan_failed":     "moved",
	// In-progress → force-reset to prior state
	"resolving":      "submitted",
	"searching":      "resolved",
	"search_complete": "approved", // legacy status from old recoverStuckJobs
	"downloading":    "approved",
	"moving":         "download_complete",
	"scanning":       "moved",
}

// advanceTargetStatus maps stuck in-progress statuses to their next state,
// allowing a manual step-skip when the worker is unable to progress.
var advanceTargetStatus = map[string]string{
	// search_complete is a legacy status left by old recoverStuckJobs logic;
	// skip straight to download_complete with SABnzbd sync attempt.
	"search_complete": "download_complete",
	"downloading":     "download_complete",
	"moving":          "moved",
	"scanning":        "complete",
}

// extractEntityID parses the StashDB entity ID from a URL given its type.
// Returns the ID and whether the URL matched the expected pattern.
func extractEntityID(rawURL, jobType string) (string, bool) {
	switch jobType {
	case "scene":
		m := sceneURLRe.FindStringSubmatch(rawURL)
		if m == nil {
			return "", false
		}
		return m[1], true
	case "performer":
		m := performerURLRe.FindStringSubmatch(rawURL)
		if m == nil {
			return "", false
		}
		return m[1], true
	case "studio":
		m := studioURLRe.FindStringSubmatch(rawURL)
		if m == nil {
			return "", false
		}
		return m[1], true
	}
	return "", false
}

// --- List Jobs ---

func handleListJobs(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		status := c.Query("status")
		jobType := c.Query("type")
		batchID := c.Query("batch_id")
		limit := c.QueryInt("limit", 50)
		if limit < 1 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}

		// /review is a fixed filter for awaiting_review
		if strings.HasSuffix(c.Path(), "/review") {
			status = "awaiting_review"
		}

		var jobList []queries.Job
		var queryErr error

		if batchID != "" {
			batchUUID, parseErr := uuid.Parse(batchID)
			if parseErr != nil {
				return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid batch_id")
			}
			sql := `SELECT id, type, status, stashdb_url, stashdb_id, parent_batch_id,
			               error_message, retry_count, created_at, updated_at
			        FROM jobs WHERE parent_batch_id = $1`
			args := []interface{}{batchUUID}
			idx := 2
			if status != "" {
				sql += fmt.Sprintf(" AND status = $%d", idx)
				args = append(args, status)
				idx++
			}
			if jobType != "" {
				sql += fmt.Sprintf(" AND type = $%d", idx)
				args = append(args, jobType)
				idx++
			}
			sql += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
			args = append(args, int32(limit))

			rows, rowErr := app.DB.Query(ctx, sql, args...)
			if rowErr != nil {
				return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to list jobs")
			}
			defer rows.Close()
			for rows.Next() {
				var j queries.Job
				if scanErr := rows.Scan(
					&j.ID, &j.Type, &j.Status, &j.StashdbUrl, &j.StashdbID,
					&j.ParentBatchID, &j.ErrorMessage, &j.RetryCount, &j.CreatedAt, &j.UpdatedAt,
				); scanErr != nil {
					return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to scan jobs")
				}
				jobList = append(jobList, j)
			}
			if rows.Err() != nil {
				return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to iterate jobs")
			}
		} else {
			var pgStatus pgtype.Text
			if status != "" {
				pgStatus = pgtype.Text{String: status, Valid: true}
			}
			var pgType pgtype.Text
			if jobType != "" {
				pgType = pgtype.Text{String: jobType, Valid: true}
			}
			jobList, queryErr = q.ListJobs(ctx, queries.ListJobsParams{
				Status:     pgStatus,
				Type:       pgType,
				MaxResults: int32(limit),
			})
			if queryErr != nil {
				return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to list jobs")
			}
		}

		if jobList == nil {
			jobList = []queries.Job{}
		}

		type sceneSnippet struct {
			Title       string   `json:"title"`
			StudioName  *string  `json:"studio_name,omitempty"`
			ReleaseDate *string  `json:"release_date,omitempty"`
			Performers  []string `json:"performers,omitempty"`
		}
		type jobRow struct {
			ID         uuid.UUID     `json:"id"`
			Type       string        `json:"type"`
			Status     string        `json:"status"`
			StashdbURL string        `json:"stashdb_url"`
			Scene      *sceneSnippet `json:"scene,omitempty"`
			CreatedAt  interface{}   `json:"created_at"`
			UpdatedAt  interface{}   `json:"updated_at"`
		}

		rows := make([]jobRow, 0, len(jobList))
		for _, j := range jobList {
			row := jobRow{
				ID:         j.ID,
				Type:       j.Type,
				Status:     j.Status,
				StashdbURL: j.StashdbUrl,
				CreatedAt:  j.CreatedAt,
				UpdatedAt:  j.UpdatedAt,
			}
			scene, sceneErr := q.GetSceneByJobID(ctx, j.ID)
			if sceneErr == nil {
				sn := &sceneSnippet{Title: scene.Title}
				if scene.StudioName.Valid {
					sn.StudioName = &scene.StudioName.String
				}
				if scene.ReleaseDate.Valid {
					d := scene.ReleaseDate.Time.Format("2006-01-02")
					sn.ReleaseDate = &d
				}
				if len(scene.Performers) > 0 {
					var ps []struct {
						Name string `json:"name"`
					}
					if json.Unmarshal(scene.Performers, &ps) == nil {
						for _, p := range ps {
							sn.Performers = append(sn.Performers, p.Name)
						}
					}
				}
				row.Scene = sn
			}
			rows = append(rows, row)
		}

		var nextCursor interface{}
		if len(jobList) == limit {
			nextCursor = jobList[len(jobList)-1].ID
		}

		return c.JSON(fiber.Map{
			"jobs":        rows,
			"next_cursor": nextCursor,
			"total":       len(rows),
		})
	}
}

// --- Get Job ---

func handleGetJob(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid job id")
		}

		job, err := q.GetJob(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "job not found")
		}

		scene, _ := q.GetSceneByJobID(ctx, id)
		results, _ := q.ListSearchResultsByJobID(ctx, id)
		download, _ := q.GetDownloadByJobID(ctx, id)
		events, _ := q.ListJobEventsByJobID(ctx, id)

		// Scene
		var sceneResp interface{}
		if scene.ID != uuid.Nil {
			var performers interface{}
			if len(scene.Performers) > 0 {
				_ = json.Unmarshal(scene.Performers, &performers)
			}
			var tags interface{}
			if len(scene.Tags) > 0 {
				_ = json.Unmarshal(scene.Tags, &tags)
			}
			sr := fiber.Map{
				"stashdb_scene_id": scene.StashdbSceneID,
				"title":            scene.Title,
				"performers":       performers,
				"tags":             tags,
			}
			if scene.StudioName.Valid {
				sr["studio_name"] = scene.StudioName.String
			}
			if scene.StudioSlug.Valid {
				sr["studio_slug"] = scene.StudioSlug.String
			}
			if scene.ReleaseDate.Valid {
				sr["release_date"] = scene.ReleaseDate.Time.Format("2006-01-02")
			}
			if scene.DurationSeconds.Valid {
				sr["duration_seconds"] = scene.DurationSeconds.Int32
			}
			sceneResp = sr
		}

		// Search results
		type srResp struct {
			ID              uuid.UUID   `json:"id"`
			IndexerName     string      `json:"indexer_name"`
			ReleaseTitle    string      `json:"release_title"`
			SizeBytes       interface{} `json:"size_bytes"`
			PublishDate     interface{} `json:"publish_date"`
			InfoURL         interface{} `json:"info_url"`
			ConfidenceScore int32       `json:"confidence_score"`
			ScoreBreakdown  interface{} `json:"score_breakdown"`
			IsSelected      bool        `json:"is_selected"`
			SelectedBy      interface{} `json:"selected_by"`
		}
		srList := make([]srResp, 0, len(results))
		for _, sr := range results {
			var breakdown interface{}
			if len(sr.ScoreBreakdown) > 0 {
				_ = json.Unmarshal(sr.ScoreBreakdown, &breakdown)
			}
			r := srResp{
				ID:              sr.ID,
				IndexerName:     sr.IndexerName,
				ReleaseTitle:    sr.ReleaseTitle,
				ConfidenceScore: sr.ConfidenceScore,
				ScoreBreakdown:  breakdown,
				IsSelected:      sr.IsSelected,
			}
			if sr.SizeBytes.Valid {
				r.SizeBytes = sr.SizeBytes.Int64
			}
			if sr.PublishDate.Valid {
				r.PublishDate = sr.PublishDate.Time
			}
			if sr.InfoUrl.Valid {
				r.InfoURL = sr.InfoUrl.String
			}
			if sr.SelectedBy.Valid {
				r.SelectedBy = sr.SelectedBy.String
			}
			srList = append(srList, r)
		}

		// Events
		type evtResp struct {
			EventType string      `json:"event_type"`
			Payload   interface{} `json:"payload"`
			CreatedAt interface{} `json:"created_at"`
		}
		evtList := make([]evtResp, 0, len(events))
		for _, evt := range events {
			var payload interface{}
			if len(evt.Payload) > 0 {
				_ = json.Unmarshal(evt.Payload, &payload)
			}
			evtList = append(evtList, evtResp{
				EventType: evt.EventType,
				Payload:   payload,
				CreatedAt: evt.CreatedAt,
			})
		}

		// Download
		var dlResp interface{}
		if download.ID != uuid.Nil {
			d := fiber.Map{
				"id":             download.ID,
				"sabnzbd_nzo_id": download.SabnzbdNzoID,
				"status":         download.Status,
				"created_at":     download.CreatedAt,
				"updated_at":     download.UpdatedAt,
			}
			if download.Filename.Valid {
				d["filename"] = download.Filename.String
			}
			if download.SourcePath.Valid {
				d["source_path"] = download.SourcePath.String
			}
			if download.FinalPath.Valid {
				d["final_path"] = download.FinalPath.String
			}
			if download.SizeBytes.Valid {
				d["size_bytes"] = download.SizeBytes.Int64
			}
			if download.CompletedAt.Valid {
				d["completed_at"] = download.CompletedAt.Time
			}
			dlResp = d
		}

		resp := fiber.Map{
			"id":             job.ID,
			"type":           job.Type,
			"status":         job.Status,
			"stashdb_url":    job.StashdbUrl,
			"retry_count":    job.RetryCount,
			"error_message":  nil,
			"scene":          sceneResp,
			"search_results": srList,
			"download":       dlResp,
			"events":         evtList,
			"created_at":     job.CreatedAt,
			"updated_at":     job.UpdatedAt,
		}
		if job.ErrorMessage.Valid {
			resp["error_message"] = job.ErrorMessage.String
		}

		return c.JSON(resp)
	}
}

// --- Create Job ---

func handleCreateJob(app *models.App) fiber.Handler {
	return handleCreateJobWith(queries.New(app.DB), app)
}

// handleCreateJobWith is the testable implementation of handleCreateJob.
func handleCreateJobWith(q queries.Querier, app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		var body struct {
			URL  string `json:"url"`
			Type string `json:"type"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}
		if body.URL == "" {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "url is required")
		}
		if !validJobTypes[body.Type] {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "type must be one of: scene, performer, studio")
		}

		entityID, valid := extractEntityID(body.URL, body.Type)
		if !valid {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST",
				fmt.Sprintf("url does not match expected pattern for type %q", body.Type))
		}

		job, err := q.CreateJob(ctx, queries.CreateJobParams{
			Type:       body.Type,
			StashdbUrl: body.URL,
			StashdbID:  pgtype.Text{String: entityID, Valid: true},
		})
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to create job")
		}

		resp := fiber.Map{
			"job_id":       job.ID,
			"batch_job_id": nil,
			"type":         job.Type,
			"status":       job.Status,
		}

		if body.Type == "performer" || body.Type == "studio" {
			batch, batchErr := q.CreateBatchJob(ctx, queries.CreateBatchJobParams{
				JobID:           job.ID,
				Type:            body.Type,
				StashdbEntityID: entityID,
			})
			if batchErr == nil {
				resp["batch_job_id"] = batch.ID
			}
		}

		return c.Status(fiber.StatusAccepted).JSON(resp)
	}
}

// --- Approve Job ---

// handleApproveJob wraps handleApproveJobWith for production use.
func handleApproveJob(app *models.App) fiber.Handler {
	return handleApproveJobWith(queries.New(app.DB))
}

// handleApproveJobWith is the testable implementation accepting a Querier.
func handleApproveJobWith(q queries.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid job id")
		}

		job, err := q.GetJob(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "job not found")
		}

		if job.Status != "awaiting_review" && job.Status != "search_failed" {
			return apiError(c, fiber.StatusConflict, "INVALID_STATUS",
				fmt.Sprintf("job is in status %q, expected awaiting_review or search_failed", job.Status))
		}

		var body struct {
			ResultID string `json:"result_id"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}
		resultID, err := uuid.Parse(body.ResultID)
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid result_id")
		}

		_, err = q.SelectSearchResult(ctx, queries.SelectSearchResultParams{
			SelectedBy: pgtype.Text{String: "user", Valid: true},
			ID:         resultID,
		})
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "RESULT_NOT_FOUND", "search result not found")
		}

		updated, err := q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
			Status: "approved",
			ID:     id,
		})
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to update job status")
		}

		return c.JSON(fiber.Map{
			"job_id": updated.ID,
			"status": updated.Status,
		})
	}
}

// --- Retry Job ---

func handleRetryJob(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid job id")
		}

		job, err := q.GetJob(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "job not found")
		}

		_, ok := retryTargetStatus[job.Status]
		if !ok {
			return apiError(c, fiber.StatusConflict, "NOT_RETRYABLE",
				fmt.Sprintf("job in status %q is not retryable", job.Status))
		}

		targetStatus := retryTargetStatus[job.Status]
		if c.Query("from_start") == "true" {
			targetStatus = "submitted"
		}

		// Reset retry_count and clear error; no sqlc query for this, use pool directly.
		_, err = app.DB.Exec(ctx,
			`UPDATE jobs SET status = $1, retry_count = 0, error_message = NULL, updated_at = NOW() WHERE id = $2`,
			targetStatus, id,
		)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to retry job")
		}

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"job_id": job.ID,
			"status": targetStatus,
		})
	}
}

// --- Custom Search ---

func handleCustomSearch(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid job id")
		}

		job, err := queries.New(app.DB).GetJob(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "job not found")
		}

		if job.Status != "search_failed" && job.Status != "awaiting_review" {
			return apiError(c, fiber.StatusConflict, "INVALID_STATUS",
				fmt.Sprintf("job is in status %q; custom search requires search_failed or awaiting_review", job.Status))
		}

		var body struct {
			Query string `json:"query"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}
		body.Query = strings.TrimSpace(body.Query)
		if body.Query == "" {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "query is required")
		}

		if err := queries.New(app.DB).DeleteSearchResultsByJobID(ctx, id); err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to clear old results")
		}

		if err := worker.RunSearch(ctx, app.DB, app.Prowlarr, app.Config, id, body.Query); err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}

		return c.JSON(fiber.Map{"ok": true})
	}
}

// --- Advance Job ---

// handleAdvanceJob manually skips the current in-progress step and moves the
// job to the next status. For "downloading" it first attempts to sync the
// completed download from SABnzbd history (by NZO ID then by release title)
// so the move worker has a source path to work with.
func handleAdvanceJob(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid job id")
		}

		job, err := q.GetJob(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "job not found")
		}

		targetStatus, ok := advanceTargetStatus[job.Status]
		if !ok {
			return apiError(c, fiber.StatusConflict, "NOT_ADVANCEABLE",
				fmt.Sprintf("job in status %q cannot be advanced", job.Status))
		}

		// For downloading/search_complete → download_complete, try to populate
		// source_path from SABnzbd history so the move worker has something to work with.
		if job.Status == "downloading" || job.Status == "search_complete" {
			download, dlErr := q.GetDownloadByJobID(ctx, id)
			if dlErr == nil {
				historyItems, histErr := app.SABnzbd.GetHistory(ctx)
				if histErr == nil {
					histByNzo := make(map[string]sabnzbd.HistoryItem, len(historyItems))
					histByName := make(map[string]sabnzbd.HistoryItem, len(historyItems))
					for _, item := range historyItems {
						histByNzo[item.NzoID] = item
						histByName[item.Name] = item
					}

					var found sabnzbd.HistoryItem
					var matched bool
					if found, matched = histByNzo[download.SabnzbdNzoID]; !matched {
						result, resErr := q.GetSelectedResultByJobID(ctx, id)
						if resErr == nil {
							found, matched = histByName[result.ReleaseTitle]
						}
					}

					if matched {
						_, _ = q.UpdateDownloadComplete(ctx, queries.UpdateDownloadCompleteParams{
							Filename:   pgtype.Text{String: found.Name, Valid: found.Name != ""},
							SourcePath: pgtype.Text{String: found.StoragePath, Valid: found.StoragePath != ""},
							ID:         download.ID,
						})
						if found.NzoID != download.SabnzbdNzoID {
							_, _ = app.DB.Exec(ctx,
								`UPDATE downloads SET sabnzbd_nzo_id = $1, updated_at = NOW() WHERE id = $2`,
								found.NzoID, download.ID,
							)
						}
						_, _ = q.UpdateDownloadStatus(ctx, queries.UpdateDownloadStatusParams{
							Status: "complete",
							ID:     download.ID,
						})
					}
				}
			}
		}

		_, err = app.DB.Exec(ctx,
			`UPDATE jobs SET status = $1, error_message = NULL, updated_at = NOW() WHERE id = $2`,
			targetStatus, id,
		)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to advance job")
		}

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"job_id": job.ID,
			"status": targetStatus,
		})
	}
}

// --- Delete / Cancel Job ---

func handleDeleteJob(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid job id")
		}

		job, err := q.GetJob(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "job not found")
		}

		// If there's an active SABnzbd download, remove it.
		if job.Status == "downloading" || job.Status == "approved" {
			dl, dlErr := q.GetDownloadByJobID(ctx, id)
			if dlErr == nil && dl.SabnzbdNzoID != "" {
				_ = app.SABnzbd.DeleteJob(ctx, dl.SabnzbdNzoID)
			}
		}

		if err := q.CancelJob(ctx, id); err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel job")
		}

		return c.SendStatus(fiber.StatusNoContent)
	}
}
