-- name: CreateBatchJob :one
INSERT INTO batch_jobs (job_id, type, stashdb_entity_id, entity_name)
VALUES (@job_id, @type, @stashdb_entity_id, sqlc.narg('entity_name'))
RETURNING *;

-- name: GetBatchJob :one
SELECT * FROM batch_jobs WHERE id = @id;

-- name: ListBatchJobs :many
SELECT * FROM batch_jobs ORDER BY created_at DESC;

-- name: UpdateBatchCounts :one
UPDATE batch_jobs
SET total_scene_count = sqlc.narg('total_scene_count'),
    enqueued_count    = @enqueued_count,
    pending_count     = @pending_count,
    duplicate_count   = @duplicate_count,
    updated_at        = NOW()
WHERE id = @id
RETURNING *;

-- name: ConfirmBatch :one
UPDATE batch_jobs
SET confirmed    = TRUE,
    confirmed_at = NOW(),
    updated_at   = NOW()
WHERE id = @id
RETURNING *;
