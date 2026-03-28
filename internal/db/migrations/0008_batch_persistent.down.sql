ALTER TABLE batch_jobs
    DROP COLUMN last_checked_at,
    ADD COLUMN tag_ids JSONB NOT NULL DEFAULT '[]';
