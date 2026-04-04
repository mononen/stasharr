-- name: CreateSearchResult :one
INSERT INTO search_results (job_id, indexer_name, release_title, size_bytes, publish_date, download_url, nzb_id, confidence_score, score_breakdown, info_url)
VALUES (@job_id, @indexer_name, @release_title, sqlc.narg('size_bytes'), sqlc.narg('publish_date'), sqlc.narg('download_url'), sqlc.narg('nzb_id'), @confidence_score, @score_breakdown, sqlc.narg('info_url'))
RETURNING *;

-- name: ListSearchResultsByJobID :many
SELECT * FROM search_results
WHERE job_id = @job_id
ORDER BY confidence_score DESC;

-- name: SelectSearchResult :one
WITH target AS (
    SELECT sr.id, sr.job_id FROM search_results sr WHERE sr.id = @id
),
unselect AS (
    UPDATE search_results
    SET is_selected = FALSE,
        selected_by = NULL,
        selected_at = NULL
    WHERE job_id = (SELECT job_id FROM target)
)
UPDATE search_results
SET is_selected = TRUE,
    selected_by = @selected_by,
    selected_at = NOW()
WHERE id = (SELECT id FROM target)
RETURNING *;

-- name: GetSelectedResultByJobID :one
SELECT * FROM search_results
WHERE job_id = @job_id AND is_selected = TRUE;

-- name: GetSearchResultByID :one
SELECT * FROM search_results WHERE id = @id;

-- name: DeleteSearchResultsByJobID :exec
DELETE FROM search_results WHERE job_id = @job_id;
