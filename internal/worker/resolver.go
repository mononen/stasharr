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

const resolverBatchThreshold = 40

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

// splitBatch splits scenes at threshold, returning the first batch and the
// remaining slice. If len(scenes) <= threshold the remainder is nil.
func splitBatch(scenes []string, threshold int) (first, rest []string) {
	if len(scenes) <= threshold {
		return scenes, nil
	}
	return scenes[:threshold], scenes[threshold:]
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

func (w *ResolverWorker) resolveBatch(ctx context.Context, job *models.Job, entityID, entityType string) {
	var scenes []stashdb.Scene
	var err error

	switch entityType {
	case "performer":
		scenes, err = w.stashdb.FindPerformerScenes(ctx, entityID)
	case "studio":
		scenes, err = w.stashdb.FindStudioScenes(ctx, entityID)
	}
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	batchJob, err := queries.New(w.db).CreateBatchJob(ctx, queries.CreateBatchJobParams{
		JobID:           job.ID,
		Type:            entityType,
		StashdbEntityID: entityID,
		EntityName:      pgtype.Text{},
	})
	if err != nil {
		_ = w.updateJobStatus(ctx, job.ID, "resolve_failed", err.Error())
		_ = w.emitEvent(ctx, job.ID, "resolve_failed", map[string]string{"error": err.Error()})
		return
	}

	// Collect scene IDs and split into first batch and pending remainder.
	sceneIDs := make([]string, len(scenes))
	for i, s := range scenes {
		sceneIDs[i] = s.ID
	}
	firstBatch, pendingScenes := splitBatch(sceneIDs, resolverBatchThreshold)

	parentBatchID := pgtype.UUID{Bytes: batchJob.ID, Valid: true}
	for _, sid := range firstBatch {
		_, err := queries.New(w.db).CreateJob(ctx, queries.CreateJobParams{
			Type:          "scene",
			StashdbUrl:    "https://stashdb.org/scenes/" + sid,
			StashdbID:     pgtype.Text{},
			ParentBatchID: parentBatchID,
		})
		if err != nil {
			w.logger.Error().Err(err).Str("scene_id", sid).Msg("resolver: create child job")
		}
	}

	_, err = queries.New(w.db).UpdateBatchCounts(ctx, queries.UpdateBatchCountsParams{
		TotalSceneCount: pgtype.Int4{Int32: int32(len(scenes)), Valid: true},
		EnqueuedCount:   int32(len(firstBatch)),
		PendingCount:    int32(len(pendingScenes)),
		DuplicateCount:  0,
		ID:              batchJob.ID,
	})
	if err != nil {
		w.logger.Error().Err(err).Msg("resolver: update batch counts")
	}

	_ = w.updateJobStatus(ctx, job.ID, "resolved", "")
	_ = w.emitEvent(ctx, job.ID, "resolve_complete", map[string]any{
		"total":    len(scenes),
		"enqueued": len(firstBatch),
	})
}
