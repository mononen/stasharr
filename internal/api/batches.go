package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

func handleListBatches(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		batches, err := q.ListBatchJobs(ctx)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to list batches")
		}
		if batches == nil {
			batches = []queries.BatchJob{}
		}

		type batchRow struct {
			ID              uuid.UUID   `json:"id"`
			Type            string      `json:"type"`
			EntityName      interface{} `json:"entity_name"`
			StashdbEntityID string      `json:"stashdb_entity_id"`
			TotalSceneCount interface{} `json:"total_scene_count"`
			EnqueuedCount   int32       `json:"enqueued_count"`
			PendingCount    int32       `json:"pending_count"`
			DuplicateCount  int32       `json:"duplicate_count"`
			Confirmed       bool        `json:"confirmed"`
			TagNames        []string    `json:"tag_names,omitempty"`
			CreatedAt       interface{} `json:"created_at"`
		}

		rows := make([]batchRow, 0, len(batches))
		for _, b := range batches {
			row := batchRow{
				ID:              b.ID,
				Type:            b.Type,
				StashdbEntityID: b.StashdbEntityID,
				EnqueuedCount:   b.EnqueuedCount,
				PendingCount:    b.PendingCount,
				DuplicateCount:  b.DuplicateCount,
				Confirmed:       b.Confirmed,
				CreatedAt:       b.CreatedAt,
			}
			if b.EntityName.Valid {
				row.EntityName = b.EntityName.String
			}
			if b.TotalSceneCount.Valid {
				row.TotalSceneCount = b.TotalSceneCount.Int32
			}
			row.TagNames = resolveTagNames(ctx, app, b.TagIDs)
			rows = append(rows, row)
		}

		return c.JSON(fiber.Map{"batches": rows})
	}
}

func handleGetBatch(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid batch id")
		}

		batch, err := q.GetBatchJob(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "BATCH_NOT_FOUND", "batch not found")
		}

		// Summarise child jobs by status.
		rows, rowErr := app.DB.Query(ctx,
			`SELECT status, COUNT(*) FROM jobs WHERE parent_batch_id = $1 GROUP BY status`,
			batch.ID,
		)
		statusCounts := fiber.Map{}
		if rowErr == nil {
			defer rows.Close()
			for rows.Next() {
				var st string
				var cnt int64
				_ = rows.Scan(&st, &cnt)
				statusCounts[st] = cnt
			}
		}

		resp := fiber.Map{
			"id":                batch.ID,
			"type":              batch.Type,
			"stashdb_entity_id": batch.StashdbEntityID,
			"enqueued_count":    batch.EnqueuedCount,
			"pending_count":     batch.PendingCount,
			"duplicate_count":   batch.DuplicateCount,
			"confirmed":         batch.Confirmed,
			"created_at":        batch.CreatedAt,
			"updated_at":        batch.UpdatedAt,
			"job_status_counts": statusCounts,
		}
		if batch.EntityName.Valid {
			resp["entity_name"] = batch.EntityName.String
		}
		if batch.TotalSceneCount.Valid {
			resp["total_scene_count"] = batch.TotalSceneCount.Int32
		}
		if batch.ConfirmedAt.Valid {
			resp["confirmed_at"] = batch.ConfirmedAt.Time
		}
		if tagNames := resolveTagNames(ctx, app, batch.TagIDs); len(tagNames) > 0 {
			resp["tag_names"] = tagNames
		}

		return c.JSON(resp)
	}
}

// handleApproveBatch approves one or more pending_approval scene jobs in a batch,
// transitioning them to resolved so the search worker picks them up.
// Body: {"scene_ids": ["uuid",...]} or {"all": true}
func handleApproveBatch(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		batchID, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid batch id")
		}

		var body struct {
			SceneIDs []uuid.UUID `json:"scene_ids"`
			All      bool        `json:"all"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}

		jobIDs, err := pendingApprovalJobIDs(ctx, app, batchID, body.All, body.SceneIDs)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to query jobs")
		}

		q := queries.New(app.DB)
		approved := 0
		for _, jid := range jobIDs {
			if _, err := q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
				Status: "resolved",
				ID:     jid,
			}); err != nil {
				continue
			}
			approved++
		}

		return c.JSON(fiber.Map{"approved": approved})
	}
}

// handleDenyBatch cancels one or more pending_approval scene jobs in a batch.
// Body: {"scene_ids": ["uuid",...]} or {"all": true}
func handleDenyBatch(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		batchID, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid batch id")
		}

		var body struct {
			SceneIDs []uuid.UUID `json:"scene_ids"`
			All      bool        `json:"all"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}

		jobIDs, err := pendingApprovalJobIDs(ctx, app, batchID, body.All, body.SceneIDs)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to query jobs")
		}

		q := queries.New(app.DB)
		denied := 0
		for _, jid := range jobIDs {
			if _, err := q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
				Status: "cancelled",
				ID:     jid,
			}); err != nil {
				continue
			}
			denied++
		}

		return c.JSON(fiber.Map{"denied": denied})
	}
}

// handleNextBatch fetches the next page of scenes from StashDB and adds them to the
// batch as pending_approval jobs.
func handleNextBatch(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		batchID, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid batch id")
		}

		batch, err := q.GetBatchJob(ctx, batchID)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "BATCH_NOT_FOUND", "batch not found")
		}
		if batch.Confirmed {
			return apiError(c, fiber.StatusConflict, "EXHAUSTED", "no more scenes to load for this batch")
		}

		var tagIDs []string
		if len(batch.TagIDs) > 0 {
			_ = json.Unmarshal(batch.TagIDs, &tagIDs)
		}

		nextPage := int(batch.StashdbPage) + 1
		var scenes []stashdb.Scene
		var totalCount int

		switch batch.Type {
		case "performer":
			scenes, totalCount, err = app.StashDB.FindPerformerScenesPage(ctx, batch.StashdbEntityID, nextPage, tagIDs)
		case "studio":
			scenes, totalCount, err = app.StashDB.FindStudioScenesPage(ctx, batch.StashdbEntityID, nextPage, tagIDs)
		}
		if err != nil {
			return apiError(c, fiber.StatusBadGateway, "STASHDB_ERROR", "failed to fetch scenes from StashDB: "+err.Error())
		}
		if len(scenes) == 0 {
			// Mark as confirmed/exhausted even if StashDB returned an empty page.
			_, _ = q.AdvanceBatchPage(ctx, queries.AdvanceBatchPageParams{
				EnqueuedCount:  batch.EnqueuedCount,
				PendingCount:   0,
				DuplicateCount: batch.DuplicateCount,
				Confirmed:      true,
				ID:             batchID,
			})
			return c.JSON(fiber.Map{"added": 0, "remaining": 0, "exhausted": true})
		}

		// Try duplicate detection against default Stash instance.
		var stashClient *stashapp.Client
		if inst, err := q.GetDefaultStashInstance(ctx); err == nil {
			stashClient = stashapp.New(inst.Url, inst.ApiKey)
		}

		parentBatchID := pgtype.UUID{Bytes: batchID, Valid: true}
		added := 0
		newDuplicates := 0
		for _, scene := range scenes {
			// 1. Check against default Stash instance (if configured).
			if stashClient != nil {
				if existing, err := stashClient.FindSceneByStashDBID(ctx, scene.ID); err == nil && existing != nil {
					newDuplicates++
					continue
				}
			}

			// 2. Check against Stasharr's own database (prior queued/resolved jobs).
			if _, err := q.GetJobByStashDBID(ctx, pgtype.Text{String: scene.ID, Valid: true}); err == nil {
				newDuplicates++
				continue
			}

			childJob, err := q.CreatePendingApprovalJob(ctx, queries.CreatePendingApprovalJobParams{
				Type:          "scene",
				StashdbUrl:    "https://stashdb.org/scenes/" + scene.ID,
				StashdbID:     pgtype.Text{String: scene.ID, Valid: true},
				ParentBatchID: parentBatchID,
			})
			if err != nil {
				continue
			}

			performersJSON, _ := json.Marshal(scene.Performers)
			tagsJSON, _ := json.Marshal(scene.Tags)

			var releaseDate pgtype.Date
			if scene.Date != "" {
				if t, parseErr := time.Parse("2006-01-02", scene.Date); parseErr == nil {
					releaseDate = pgtype.Date{Time: t, Valid: true}
				}
			}
			var durationSeconds pgtype.Int4
			if scene.DurationSeconds > 0 {
				durationSeconds = pgtype.Int4{Int32: int32(scene.DurationSeconds), Valid: true}
			}

			if _, err := q.CreateScene(ctx, queries.CreateSceneParams{
				JobID:           childJob.ID,
				StashdbSceneID:  scene.ID,
				Title:           scene.Title,
				StudioName:      pgtype.Text{String: scene.StudioName, Valid: scene.StudioName != ""},
				StudioSlug:      pgtype.Text{String: scene.StudioSlug, Valid: scene.StudioSlug != ""},
				ReleaseDate:     releaseDate,
				DurationSeconds: durationSeconds,
				Performers:      performersJSON,
				Tags:            tagsJSON,
				RawResponse:     scene.RawResponse,
				ImageURL:        pgtype.Text{String: scene.ImageURL, Valid: scene.ImageURL != ""},
			}); err != nil {
				log.Error().Err(err).Str("stashdb_id", scene.ID).Msg("failed to create scene record")
				continue
			}
			added++
		}

		// Determine if this was the last page.
		exhausted := len(scenes) < stashdb.BatchPerPage
		newEnqueued := batch.EnqueuedCount + int32(added)
		newDuplicateCount := batch.DuplicateCount + int32(newDuplicates)
		remaining := totalCount - int(newEnqueued) - int(newDuplicateCount)
		if remaining < 0 {
			remaining = 0
		}

		_, _ = q.AdvanceBatchPage(ctx, queries.AdvanceBatchPageParams{
			EnqueuedCount:  newEnqueued,
			PendingCount:   int32(remaining),
			DuplicateCount: newDuplicateCount,
			Confirmed:      exhausted,
			ID:             batchID,
		})

		return c.JSON(fiber.Map{
			"added":     added,
			"remaining": remaining,
			"exhausted": exhausted,
		})
	}
}

// handleAutoStartBatch approves all pending_approval jobs in a batch immediately,
// sending them into the search/download pipeline without manual review.
func handleAutoStartBatch(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		batchID, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid batch id")
		}

		jobIDs, err := pendingApprovalJobIDs(ctx, app, batchID, true, nil)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to query jobs")
		}

		q := queries.New(app.DB)
		started := 0
		for _, jid := range jobIDs {
			if _, err := q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
				Status: "resolved",
				ID:     jid,
			}); err != nil {
				continue
			}
			started++
		}

		return c.JSON(fiber.Map{"started": started})
	}
}

// pendingApprovalJobIDs returns job IDs for pending_approval jobs in a batch.
// If all is true, returns all of them; otherwise filters to the provided IDs.
func pendingApprovalJobIDs(ctx context.Context, app *models.App, batchID uuid.UUID, all bool, filterIDs []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := app.DB.Query(ctx, `SELECT id FROM jobs WHERE parent_batch_id = $1 AND status = 'pending_approval'`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filterSet := make(map[uuid.UUID]bool, len(filterIDs))
	for _, id := range filterIDs {
		filterSet[id] = true
	}

	var result []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if all || filterSet[id] {
			result = append(result, id)
		}
	}
	return result, rows.Err()
}

// resolveTagNames converts a JSONB tag_ids array to human-readable tag names
// by querying StashDB. Returns nil if there are no tags or on error.
func resolveTagNames(ctx context.Context, app *models.App, tagIDsJSON []byte) []string {
	if len(tagIDsJSON) == 0 {
		return nil
	}
	var tagIDs []string
	if err := json.Unmarshal(tagIDsJSON, &tagIDs); err != nil || len(tagIDs) == 0 {
		return nil
	}
	names := make([]string, 0, len(tagIDs))
	for _, id := range tagIDs {
		name, err := app.StashDB.FindTagName(ctx, id)
		if err != nil || name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	return names
}
