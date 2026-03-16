-- name: CreateAlias :one
INSERT INTO studio_aliases (canonical, alias)
VALUES (@canonical, @alias)
RETURNING *;

-- name: ListAliases :many
SELECT * FROM studio_aliases ORDER BY canonical, alias;

-- name: DeleteAlias :exec
DELETE FROM studio_aliases WHERE id = @id;

-- name: GetAliasByAlias :one
SELECT * FROM studio_aliases WHERE LOWER(alias) = LOWER(@alias);
