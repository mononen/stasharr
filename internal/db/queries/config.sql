-- name: GetAllConfig :many
SELECT * FROM config ORDER BY key;

-- name: GetConfigValue :one
SELECT value FROM config WHERE key = @key;

-- name: SetConfigValue :exec
INSERT INTO config (key, value, updated_at)
VALUES (@key, @value, NOW())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();

-- name: SetConfigValues :exec
INSERT INTO config (key, value, updated_at)
SELECT UNNEST(@keys::text[]), UNNEST(@values::text[]), NOW()
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
