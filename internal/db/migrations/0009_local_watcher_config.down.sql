DELETE FROM config WHERE key IN (
  'localwatcher.enabled',
  'localwatcher.watch_dir',
  'localwatcher.stable_seconds',
  'localwatcher.stable_fallback_seconds',
  'localwatcher.poll_interval',
  'localwatcher.match_threshold',
  'myjdownloader.email',
  'myjdownloader.password',
  'myjdownloader.device_name'
);
