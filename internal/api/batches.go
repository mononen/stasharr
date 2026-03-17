package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

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

		return c.JSON(resp)
	}
}

func handleConfirmBatch(app *models.App) fiber.Handler {
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

		if batch.Confirmed {
			return apiError(c, fiber.StatusConflict, "ALREADY_CONFIRMED", "batch has already been confirmed")
		}
		if batch.PendingCount == 0 {
			return apiError(c, fiber.StatusConflict, "NOTHING_PENDING", "batch has no pending scenes to confirm")
		}

		pendingCount := batch.PendingCount

		_, err = q.ConfirmBatch(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to confirm batch")
		}

		// The resolver worker will pick up confirmed=true and create the remaining scene jobs.
		return c.JSON(fiber.Map{
			"batch_id":       batch.ID,
			"newly_enqueued": pendingCount,
		})
	}
}
