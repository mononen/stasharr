-- name: GetJob :one
SELECT * FROM jobs WHERE id = @id;

-- name: ListJobs :many
SELECT * FROM jobs
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('max_results');

-- name: CreateJob :one
INSERT INTO jobs (type, stashdb_url, stashdb_id, parent_batch_id)
VALUES (@type, @stashdb_url, sqlc.narg('stashdb_id'), sqlc.narg('parent_batch_id'))
RETURNING *;

-- name: UpdateJobStatus :one
UPDATE jobs
SET status        = @status,
    error_message = sqlc.narg('error_message'),
    updated_at    = NOW()
WHERE id = @id
RETURNING *;

-- name: CancelJob :exec
UPDATE jobs
SET status     = 'cancelled',
    updated_at = NOW()
WHERE id = @id;

-- name: GetJobsForWorker :many
SELECT * FROM jobs
WHERE status = @status
ORDER BY created_at ASC
LIMIT @max_results
FOR UPDATE SKIP LOCKED;
