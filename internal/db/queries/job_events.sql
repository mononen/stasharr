-- name: CreateJobEvent :one
INSERT INTO job_events (job_id, event_type, payload)
VALUES (@job_id, @event_type, @payload)
RETURNING *;

-- name: ListJobEventsByJobID :many
SELECT * FROM job_events
WHERE job_id = @job_id
ORDER BY id ASC;

-- name: ListRecentGlobalEvents :many
SELECT * FROM job_events
ORDER BY id DESC
LIMIT @max_results;
