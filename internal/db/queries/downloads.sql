-- name: CreateDownload :one
INSERT INTO downloads (job_id, sabnzbd_nzo_id, size_bytes)
VALUES (@job_id, @sabnzbd_nzo_id, sqlc.narg('size_bytes'))
RETURNING *;

-- name: GetDownloadByJobID :one
SELECT * FROM downloads WHERE job_id = @job_id;

-- name: GetDownloadByNzoID :one
SELECT * FROM downloads WHERE sabnzbd_nzo_id = @sabnzbd_nzo_id;

-- name: UpdateDownloadStatus :one
UPDATE downloads
SET status     = @status,
    updated_at = NOW()
WHERE id = @id
RETURNING *;

-- name: UpdateDownloadComplete :one
UPDATE downloads
SET filename     = @filename,
    source_path  = @source_path,
    completed_at = NOW(),
    updated_at   = NOW()
WHERE id = @id
RETURNING *;

-- name: UpdateDownloadFinalPath :one
UPDATE downloads
SET final_path = @final_path,
    updated_at = NOW()
WHERE id = @id
RETURNING *;

-- name: CreateLocalDownload :one
INSERT INTO downloads (job_id, sabnzbd_nzo_id, source_path, status)
VALUES (@job_id, '', @source_path, 'downloading')
RETURNING *;

-- name: GetLocalFoundDownloads :many
SELECT d.job_id, d.source_path FROM downloads d
JOIN jobs j ON j.id = d.job_id
WHERE j.status = 'local_found';
