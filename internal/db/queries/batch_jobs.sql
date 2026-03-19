-- name: CreateBatchJob :one
INSERT INTO batch_jobs (job_id, type, stashdb_entity_id, entity_name, tag_ids)
VALUES (@job_id, @type, @stashdb_entity_id, sqlc.narg('entity_name'), @tag_ids)
RETURNING *;

-- name: GetBatchJob :one
SELECT * FROM batch_jobs WHERE id = @id;

-- name: GetBatchJobByJobID :one
SELECT * FROM batch_jobs WHERE job_id = @job_id;

-- name: UpdateBatchEntityName :one
UPDATE batch_jobs SET entity_name = @entity_name, updated_at = NOW() WHERE id = @id RETURNING *;

-- name: ListBatchJobs :many
SELECT * FROM batch_jobs ORDER BY created_at DESC;

-- name: UpdateBatchCounts :one
UPDATE batch_jobs
SET total_scene_count = sqlc.narg('total_scene_count'),
    enqueued_count    = @enqueued_count,
    pending_count     = @pending_count,
    duplicate_count   = @duplicate_count,
    stashdb_page      = @stashdb_page,
    updated_at        = NOW()
WHERE id = @id
RETURNING *;

-- name: AdvanceBatchPage :one
UPDATE batch_jobs
SET stashdb_page    = stashdb_page + 1,
    enqueued_count  = @enqueued_count,
    pending_count   = @pending_count,
    duplicate_count = @duplicate_count,
    confirmed       = @confirmed,
    updated_at      = NOW()
WHERE id = @id
RETURNING *;

-- name: ConfirmBatch :one
UPDATE batch_jobs
SET confirmed    = TRUE,
    confirmed_at = NOW(),
    updated_at   = NOW()
WHERE id = @id
RETURNING *;
