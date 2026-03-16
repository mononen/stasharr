-- name: GetAllConfig :many
SELECT * FROM config ORDER BY key;

-- name: GetConfigValue :one
SELECT value FROM config WHERE key = $1;

-- name: UpsertConfig :exec
INSERT INTO config (key, value, updated_at) VALUES ($1, $2, NOW())
ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW();
