package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"

	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

const (
	sseBackfillLimit = 50
	ssePingInterval  = 10 * time.Second
	ssePollInterval  = 3 * time.Second
)

// writeSSEEvent writes a single SSE event to the stream and flushes.
// Returns an error if the write or flush fails (i.e. client disconnected).
func writeSSEEvent(w *bufio.Writer, event, data string) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	return w.Flush()
}

// jobEventJSON serialises a JobEvent for SSE emission.
func jobEventJSON(evt queries.JobEvent) string {
	type payload struct {
		JobID     uuid.UUID   `json:"job_id"`
		EventType string      `json:"event_type"`
		Payload   interface{} `json:"payload"`
		CreatedAt interface{} `json:"created_at"`
	}
	var p interface{}
	if len(evt.Payload) > 0 {
		_ = json.Unmarshal(evt.Payload, &p)
	}
	b, _ := json.Marshal(payload{
		JobID:     evt.JobID,
		EventType: evt.EventType,
		Payload:   p,
		CreatedAt: evt.CreatedAt,
	})
	return string(b)
}

// --- Global SSE ---

func handleGlobalEvents(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		// Snapshot backfill and last seen ID before entering stream writer.
		ctx := c.UserContext()
		q := queries.New(app.DB)

		backfill, _ := q.ListRecentGlobalEvents(ctx, sseBackfillLimit)

		// Reverse so oldest-first.
		for i, j := 0, len(backfill)-1; i < j; i, j = i+1, j-1 {
			backfill[i], backfill[j] = backfill[j], backfill[i]
		}

		var lastID int64
		if len(backfill) > 0 {
			lastID = backfill[len(backfill)-1].ID
		}

		// Try to set up LISTEN/NOTIFY. Fall back to polling if unavailable.
		notifCh := make(chan string, 16)
		listenCtx, listenCancel := context.WithCancel(ctx)

		pgConn, listenErr := app.DB.Acquire(listenCtx)
		if listenErr == nil {
			_, listenErr = pgConn.Exec(listenCtx, "LISTEN stasharr_events")
		}

		if listenErr == nil {
			// LISTEN goroutine: forwards notifications onto notifCh.
			go func() {
				defer pgConn.Release()
				for {
					notif, err := pgConn.Conn().WaitForNotification(listenCtx)
					if err != nil {
						return
					}
					select {
					case notifCh <- notif.Payload:
					case <-listenCtx.Done():
						return
					}
				}
			}()
		} else {
			// Polling fallback: every ssePollInterval emit new events by ID.
			go func() {
				ticker := time.NewTicker(ssePollInterval)
				defer ticker.Stop()
				for {
					select {
					case <-listenCtx.Done():
						return
					case <-ticker.C:
						evts, err := q.ListRecentGlobalEvents(ctx, sseBackfillLimit)
						if err != nil {
							continue
						}
						for i := len(evts) - 1; i >= 0; i-- {
							if evts[i].ID > lastID {
								select {
								case notifCh <- jobEventJSON(evts[i]):
									lastID = evts[i].ID
								case <-listenCtx.Done():
									return
								}
							}
						}
					}
				}
			}()
		}

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			defer listenCancel()

			// Flush headers and start the stream.
			if _, err := fmt.Fprint(w, ": ok\n\n"); err != nil {
				return
			}
			if w.Flush() != nil {
				return
			}

			// Send backfill.
			for _, evt := range backfill {
				if writeSSEEvent(w, "job_event", jobEventJSON(evt)) != nil {
					return
				}
			}

			pingTicker := time.NewTicker(ssePingInterval)
			defer pingTicker.Stop()

			for {
				select {
				case <-pingTicker.C:
					if writeSSEEvent(w, "ping", "{}") != nil {
						return
					}
				case payload, ok := <-notifCh:
					if !ok {
						return
					}
					if writeSSEEvent(w, "job_event", payload) != nil {
						return
					}
				}
			}
		}))

		return nil
	}
}

// --- Per-job SSE ---

func handleJobEvents(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		jobID, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid job id")
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		ctx := c.UserContext()
		q := queries.New(app.DB)

		backfill, _ := q.ListJobEventsByJobID(ctx, jobID)

		notifCh := make(chan string, 16)
		listenCtx, listenCancel := context.WithCancel(ctx)

		pgConn, listenErr := app.DB.Acquire(listenCtx)
		if listenErr == nil {
			_, listenErr = pgConn.Exec(listenCtx, "LISTEN stasharr_events")
		}

		if listenErr == nil {
			go func() {
				defer pgConn.Release()
				for {
					notif, err := pgConn.Conn().WaitForNotification(listenCtx)
					if err != nil {
						return
					}
					// Filter to this job by parsing the payload.
					if isForJob(notif.Payload, jobID) {
						select {
						case notifCh <- notif.Payload:
						case <-listenCtx.Done():
							return
						}
					}
				}
			}()
		} else {
			// Polling fallback: track last seen event ID for this job.
			var lastID int64
			if len(backfill) > 0 {
				lastID = backfill[len(backfill)-1].ID
			}
			go func() {
				ticker := time.NewTicker(ssePollInterval)
				defer ticker.Stop()
				for {
					select {
					case <-listenCtx.Done():
						return
					case <-ticker.C:
						evts, err := q.ListJobEventsByJobID(ctx, jobID)
						if err != nil {
							continue
						}
						for _, evt := range evts {
							if evt.ID > lastID {
								select {
								case notifCh <- jobEventJSON(evt):
									lastID = evt.ID
								case <-listenCtx.Done():
									return
								}
							}
						}
					}
				}
			}()
		}

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			defer listenCancel()

			// Flush headers and start the stream.
			if _, err := fmt.Fprint(w, ": ok\n\n"); err != nil {
				return
			}
			if w.Flush() != nil {
				return
			}

			for _, evt := range backfill {
				if writeSSEEvent(w, "job_event", jobEventJSON(evt)) != nil {
					return
				}
			}

			pingTicker := time.NewTicker(ssePingInterval)
			defer pingTicker.Stop()

			for {
				select {
				case <-pingTicker.C:
					if writeSSEEvent(w, "ping", "{}") != nil {
						return
					}
				case payload, ok := <-notifCh:
					if !ok {
						return
					}
					if writeSSEEvent(w, "job_event", payload) != nil {
						return
					}
				}
			}
		}))

		return nil
	}
}

// isForJob checks if a raw NOTIFY payload JSON contains the given job_id.
func isForJob(payload string, jobID uuid.UUID) bool {
	var msg struct {
		JobID string `json:"job_id"`
	}
	if json.Unmarshal([]byte(payload), &msg) != nil {
		return false
	}
	id, err := uuid.Parse(msg.JobID)
	if err != nil {
		return false
	}
	return id == jobID
}
