-- name: CreateStashInstance :one
INSERT INTO stash_instances (name, url, api_key, is_default)
VALUES (@name, @url, @api_key, @is_default)
RETURNING *;

-- name: ListStashInstances :many
SELECT * FROM stash_instances ORDER BY name;

-- name: GetStashInstance :one
SELECT * FROM stash_instances WHERE id = @id;

-- name: UpdateStashInstance :one
UPDATE stash_instances
SET name       = @name,
    url        = @url,
    api_key    = @api_key,
    is_default = @is_default,
    updated_at = NOW()
WHERE id = @id
RETURNING *;

-- name: DeleteStashInstance :exec
DELETE FROM stash_instances WHERE id = @id;

-- name: GetDefaultStashInstance :one
SELECT * FROM stash_instances WHERE is_default = TRUE LIMIT 1;

-- name: SetDefaultStashInstance :exec
UPDATE stash_instances
SET is_default = (id = @id),
    updated_at = NOW();
