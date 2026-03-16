-- name: CreateScene :one
INSERT INTO scenes (job_id, stashdb_scene_id, title, studio_name, studio_slug, release_date, duration_seconds, performers, tags, raw_response)
VALUES (@job_id, @stashdb_scene_id, @title, sqlc.narg('studio_name'), sqlc.narg('studio_slug'), sqlc.narg('release_date'), sqlc.narg('duration_seconds'), @performers, @tags, sqlc.narg('raw_response'))
RETURNING *;

-- name: GetSceneByJobID :one
SELECT * FROM scenes WHERE job_id = @job_id;

-- name: GetSceneByStashDBID :one
SELECT * FROM scenes WHERE stashdb_scene_id = @stashdb_scene_id LIMIT 1;
