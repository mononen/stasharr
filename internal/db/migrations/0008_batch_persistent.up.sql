ALTER TABLE batch_jobs
    ADD COLUMN last_checked_at TIMESTAMPTZ,
    DROP COLUMN tag_ids;
