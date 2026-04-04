-- name: CreateScene :one
INSERT INTO scenes (job_id, stashdb_scene_id, stash_scene_id, title, studio_name, studio_slug, release_date, duration_seconds, performers, tags, raw_response, image_url)
VALUES (@job_id, @stashdb_scene_id, sqlc.narg('stash_scene_id'), @title, sqlc.narg('studio_name'), sqlc.narg('studio_slug'), sqlc.narg('release_date'), sqlc.narg('duration_seconds'), @performers, @tags, sqlc.narg('raw_response'), sqlc.narg('image_url'))
RETURNING *;

-- name: GetSceneByJobID :one
SELECT * FROM scenes WHERE job_id = @job_id;

-- name: GetSceneByStashDBID :one
SELECT * FROM scenes WHERE stashdb_scene_id = @stashdb_scene_id LIMIT 1;

-- name: GetSearchFailedScenes :many
SELECT s.* FROM scenes s
JOIN jobs j ON j.id = s.job_id
WHERE j.status = 'search_failed'
ORDER BY j.created_at ASC;
