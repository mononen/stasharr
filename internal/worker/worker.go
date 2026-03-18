package worker

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/config"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

// Worker is the interface implemented by all pipeline workers.
type Worker interface {
	Start(ctx context.Context)
	Stop()
	Name() string
}

// Base provides shared database and config access for all workers.
type Base struct {
	db     *pgxpool.Pool
	config *config.Config
	logger zerolog.Logger
}

// claimJob atomically claims a single job by transitioning it from
// inputStatus to claimedStatus. Returns nil, nil when no jobs are
// available.
func (b *Base) claimJob(ctx context.Context, inputStatus, claimedStatus string) (*models.Job, error) {
	tx, err := b.db.Begin(ctx)
	if err != nil {
		return nil, err
	}

	jobs, err := queries.New(tx).GetJobsForWorker(ctx, queries.GetJobsForWorkerParams{
		Status:     inputStatus,
		MaxResults: 1,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}

	if len(jobs) == 0 {
		_ = tx.Rollback(ctx)
		return nil, nil
	}

	job := jobs[0]

	updated, err := queries.New(tx).UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
		Status:       claimedStatus,
		ErrorMessage: pgtype.Text{},
		ID:           job.ID,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &updated, nil
}

// updateJobStatus sets the status (and optional error message) of a job.
func (b *Base) updateJobStatus(ctx context.Context, jobID uuid.UUID, status, errorMsg string) error {
	errText := pgtype.Text{}
	if errorMsg != "" {
		errText = pgtype.Text{String: errorMsg, Valid: true}
	}

	_, err := queries.New(b.db).UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
		Status:       status,
		ErrorMessage: errText,
		ID:           jobID,
	})
	return err
}

// emitJobEvent records a job event with a JSON payload and notifies SSE listeners.
// If payload is nil, an empty JSON object is stored.
func emitJobEvent(ctx context.Context, db *pgxpool.Pool, jobID uuid.UUID, eventType string, payload any) error {
	var raw []byte
	var err error

	if payload == nil {
		raw = []byte("{}")
	} else {
		raw, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	evt, err := queries.New(db).CreateJobEvent(ctx, queries.CreateJobEventParams{
		JobID:     jobID,
		EventType: eventType,
		Payload:   raw,
	})
	if err != nil {
		return err
	}

	// Notify SSE listeners. Use the same JSON shape the frontend expects.
	var p interface{}
	if len(evt.Payload) > 0 {
		_ = json.Unmarshal(evt.Payload, &p)
	}
	type notifyMsg struct {
		JobID     uuid.UUID   `json:"job_id"`
		EventType string      `json:"event_type"`
		Payload   interface{} `json:"payload"`
		CreatedAt interface{} `json:"created_at"`
	}
	if notifyJSON, merr := json.Marshal(notifyMsg{
		JobID:     evt.JobID,
		EventType: evt.EventType,
		Payload:   p,
		CreatedAt: evt.CreatedAt,
	}); merr == nil {
		_, _ = db.Exec(ctx, "SELECT pg_notify('stasharr_events', $1)", string(notifyJSON))
	}

	return nil
}

// emitEvent is the receiver-based convenience wrapper for workers.
func (b *Base) emitEvent(ctx context.Context, jobID uuid.UUID, eventType string, payload any) error {
	return emitJobEvent(ctx, b.db, jobID, eventType, payload)
}
