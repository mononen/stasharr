DELETE FROM config WHERE key IN (
    'prowlarr.url', 'prowlarr.api_key', 'prowlarr.search_limit',
    'sabnzbd.url', 'sabnzbd.api_key', 'sabnzbd.category', 'sabnzbd.complete_dir',
    'stashdb.api_key',
    'matching.auto_threshold', 'matching.review_threshold',
    'pipeline.worker_resolver_pool', 'pipeline.worker_search_pool',
    'pipeline.worker_download_pool', 'pipeline.worker_move_pool',
    'pipeline.worker_scan_pool', 'pipeline.monitor_poll_interval',
    'pipeline.stashdb_rate_limit', 'pipeline.batch_auto_threshold',
    'pipeline.max_retries_resolver', 'pipeline.max_retries_search',
    'pipeline.max_retries_move', 'pipeline.max_retries_scan',
    'directory.template', 'directory.performer_max', 'directory.missing_field_value'
);
