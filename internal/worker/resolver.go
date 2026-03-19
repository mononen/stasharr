package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

// stashDBURLRe matches StashDB entity URLs of the form
// https://stashdb.org/<entityType>/<uuid>.
var stashDBURLRe = regexp.MustCompile(
	`^https://stashdb\.org/(scenes|performers|studios)/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`,
)

// parseStashDBURL extracts the entity type and ID from a StashDB URL.
// Returns an error if the URL does not match the expected format.
func parseStashDBURL(url string) (entityType, entityID string, err error) {
	m := stashDBURLRe.FindStringSubmatch(url)
	if m == nil {
		return "", "", fmt.Errorf("parseStashDBURL: no match for %q", url)
	}
	return m[1], m[2], nil
}

// ResolverWorker resolves StashDB URLs to structured metadata.
type ResolverWorker struct {
	Base
	stashdb *stashdb.Client
}

func NewResolverWorker(app *models.App, logger zerolog.Logger) *ResolverWorker {
	return &ResolverWorker{
		Base:    Base{db: app.DB, config: app.Config, logger: logger},
		stashdb: app.StashDB,
	}
}

func (w *ResolverWorker) Name() string { return "resolver" }

func (w *ResolverWorker) Start(ctx context.Context) {
	for {
		job, err := w.claimJob(ctx, "submitted", "resolving")
		if err != nil {
			w.logger.Error().Err(err).Msg("resolver: claim job error")
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

func (w *ResolverWorker) Stop() {}

func (w *ResolverWorker) process(ctx context.Context, job *models.Job) {
	if err := w.emitEvent(ctx, job.ID, "resolve_started", nil); err != nil {
		w.logger.Error().Err(err).Msg("resolver: emit resolve_started")
	}

	entityType, entityID, err := parseStashDBURL(job.StashdbUrl)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	switch entityType {
	case "scenes":
		w.resolveScene(ctx, job, entityID)
	case "performers":
		w.resolveBatch(ctx, job, entityID, "performer")
	case "studios":
		w.resolveBatch(ctx, job, entityID, "studio")
	}
}

func (w *ResolverWorker) resolveScene(ctx context.Context, job *models.Job, sceneID string) {
	// Check if the scene is already in the local Stash instance before hitting StashDB.
	_ = w.emitEvent(ctx, job.ID, "stash_check_started", nil)
	if stashInstance, err := queries.New(w.db).GetDefaultStashInstance(ctx); err != nil {
		w.logger.Warn().Err(err).Msg("resolver: no default stash instance, skipping stash check")
	} else {
		client := stashapp.New(stashInstance.Url, stashInstance.ApiKey)
		if stashScene, err := client.FindSceneByStashDBID(ctx, sceneID); err != nil {
			w.logger.Warn().Err(err).Str("stashdb_id", sceneID).Msg("resolver: stash instance check failed, continuing")
		} else if stashScene != nil {
			_, _ = queries.New(w.db).CreateScene(ctx, queries.CreateSceneParams{
				JobID:          job.ID,
				StashdbSceneID: sceneID,
				Title:          stashScene.Title,
				StudioName:     pgtype.Text{String: stashScene.StudioName, Valid: stashScene.StudioName != ""},
				Performers:     []byte("[]"),
				Tags:           []byte("[]"),
			})
			_ = w.updateJobStatus(ctx, job.ID, "already_stashed", "")
			_ = w.emitEvent(ctx, job.ID, "already_stashed", map[string]string{"stashdb_id": sceneID, "title": stashScene.Title})
			return
		}
	}

	_ = w.emitEvent(ctx, job.ID, "stashdb_fetch_started", nil)
	scene, err := w.stashdb.FindScene(ctx, sceneID)
	if err != nil {
		var statusErr *stashdb.StatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == 404 {
			msg := "scene not found on StashDB"
			_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", msg)
			_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": msg})
			return
		}
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	performersJSON, err := json.Marshal(scene.Performers)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	tagsJSON, err := json.Marshal(scene.Tags)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	var releaseDate pgtype.Date
	if scene.Date != "" {
		t, err := time.Parse("2006-01-02", scene.Date)
		if err == nil {
			releaseDate = pgtype.Date{Time: t, Valid: true}
		}
	}

	var durationSeconds pgtype.Int4
	if scene.DurationSeconds > 0 {
		durationSeconds = pgtype.Int4{Int32: int32(scene.DurationSeconds), Valid: true}
	}

	_, err = queries.New(w.db).CreateScene(ctx, queries.CreateSceneParams{
		JobID:           job.ID,
		StashdbSceneID:  scene.ID,
		Title:           scene.Title,
		StudioName:      pgtype.Text{String: scene.StudioName, Valid: scene.StudioName != ""},
		StudioSlug:      pgtype.Text{String: scene.StudioSlug, Valid: scene.StudioSlug != ""},
		ReleaseDate:     releaseDate,
		DurationSeconds: durationSeconds,
		Performers:      performersJSON,
		Tags:            tagsJSON,
		RawResponse:     scene.RawResponse,
	})
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	_ = w.updateJobStatus(ctx, job.ID, "resolved", "")
	_ = w.emitEvent(ctx, job.ID, "resolve_complete", map[string]string{
		"stashdb_id": scene.ID,
		"title":      scene.Title,
	})
}

// createBatchSceneJobs creates pending_approval jobs and their scene records for a slice
// of StashDB scenes associated with a batch. It performs duplicate detection against the
// default Stash instance (skipping scenes already present there) and returns the number
// of jobs created and the number of duplicates found.
func (w *ResolverWorker) createBatchSceneJobs(
	ctx context.Context,
	scenes []stashdb.Scene,
	batchID [16]byte,
) (created int, duplicates int) {
	q := queries.New(w.db)
	parentBatchID := pgtype.UUID{Bytes: batchID, Valid: true}

	// Try duplicate detection against the default Stash instance.
	var stashClient *stashapp.Client
	if inst, err := q.GetDefaultStashInstance(ctx); err == nil {
		stashClient = stashapp.New(inst.Url, inst.ApiKey)
	}

	for _, scene := range scenes {
		// Skip scenes that already exist in the local Stash instance.
		if stashClient != nil {
			if existing, err := stashClient.FindSceneByStashDBID(ctx, scene.ID); err != nil {
				w.logger.Warn().Err(err).Str("scene_id", scene.ID).Msg("resolver: stash duplicate check failed, skipping check")
			} else if existing != nil {
				duplicates++
				continue
			}
		}

		childJob, err := q.CreatePendingApprovalJob(ctx, queries.CreatePendingApprovalJobParams{
			Type:          "scene",
			StashdbUrl:    "https://stashdb.org/scenes/" + scene.ID,
			ParentBatchID: parentBatchID,
		})
		if err != nil {
			w.logger.Error().Err(err).Str("scene_id", scene.ID).Msg("resolver: create pending approval job")
			continue
		}

		performersJSON, _ := json.Marshal(scene.Performers)
		tagsJSON, _ := json.Marshal(scene.Tags)

		var releaseDate pgtype.Date
		if scene.Date != "" {
			if t, err := time.Parse("2006-01-02", scene.Date); err == nil {
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
		}); err != nil {
			w.logger.Error().Err(err).Str("scene_id", scene.ID).Msg("resolver: create scene record")
		}

		created++
	}
	return created, duplicates
}

func (w *ResolverWorker) resolveBatch(ctx context.Context, job *models.Job, entityID, entityType string) {
	// Fetch entity name (non-fatal if unavailable).
	var entityName string
	switch entityType {
	case "performer":
		if name, err := w.stashdb.FindPerformerName(ctx, entityID); err != nil {
			w.logger.Warn().Err(err).Msg("resolver: fetch performer name")
		} else {
			entityName = name
		}
	case "studio":
		if name, err := w.stashdb.FindStudioName(ctx, entityID); err != nil {
			w.logger.Warn().Err(err).Msg("resolver: fetch studio name")
		} else {
			entityName = name
		}
	}

	// The API handler already created the batch_job row. Look it up by job ID.
	q := queries.New(w.db)
	batchJob, err := q.GetBatchJobByJobID(ctx, job.ID)
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	// Persist the entity name now that we have it.
	if entityName != "" {
		if updated, updateErr := q.UpdateBatchEntityName(ctx, queries.UpdateBatchEntityNameParams{
			EntityName: pgtype.Text{String: entityName, Valid: true},
			ID:         batchJob.ID,
		}); updateErr == nil {
			batchJob = updated
		}
	}

	// Unmarshal tag IDs stored on the batch row.
	var tagIDs []string
	if len(batchJob.TagIDs) > 0 {
		_ = json.Unmarshal(batchJob.TagIDs, &tagIDs)
	}

	// Fetch only page 1 from StashDB — subsequent pages are fetched on demand.
	var (
		scenes     []stashdb.Scene
		totalCount int
	)
	switch entityType {
	case "performer":
		scenes, totalCount, err = w.stashdb.FindPerformerScenesPage(ctx, entityID, 1, tagIDs)
	case "studio":
		scenes, totalCount, err = w.stashdb.FindStudioScenesPage(ctx, entityID, 1, tagIDs)
	}
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	created, duplicates := w.createBatchSceneJobs(ctx, scenes, batchJob.ID)

	// Approximate remaining count: total from StashDB minus what we've loaded this page.
	// Future pages will be fetched on demand; actual duplicate count may grow.
	pendingCount := totalCount - len(scenes)
	if pendingCount < 0 {
		pendingCount = 0
	}

	_, err = queries.New(w.db).UpdateBatchCounts(ctx, queries.UpdateBatchCountsParams{
		TotalSceneCount: pgtype.Int4{Int32: int32(totalCount), Valid: true},
		EnqueuedCount:   int32(created),
		PendingCount:    int32(pendingCount),
		DuplicateCount:  int32(duplicates),
		StashdbPage:     1,
		ID:              batchJob.ID,
	})
	if err != nil {
		w.logger.Error().Err(err).Msg("resolver: update batch counts")
	}

	_ = w.updateJobStatus(ctx, job.ID, "batch_created", "")
	_ = w.emitEvent(ctx, job.ID, "batch_created", map[string]any{
		"entity_name": entityName,
		"total":       totalCount,
		"enqueued":    created,
		"pending":     pendingCount,
	})
}
