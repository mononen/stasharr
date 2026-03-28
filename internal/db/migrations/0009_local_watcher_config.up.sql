INSERT INTO config (key, value, description) VALUES
  ('localwatcher.enabled',                 'false', 'Enable local file watcher worker'),
  ('localwatcher.watch_dir',               '',      'Directory to scan for JDownloader downloads'),
  ('localwatcher.stable_seconds',          '120',   'Seconds file size must be stable before querying JD'),
  ('localwatcher.stable_fallback_seconds', '600',   'Seconds of stability to accept if JD package not found in MyJDownloader'),
  ('localwatcher.poll_interval',           '60',    'Seconds between directory scans'),
  ('localwatcher.match_threshold',         '60',    'Minimum percentage of title tokens that must match filename'),
  ('myjdownloader.email',                  '',      'MyJDownloader account email'),
  ('myjdownloader.password',               '',      'MyJDownloader account password'),
  ('myjdownloader.device_name',            '',      'JDownloader device name as shown in MyJDownloader')
ON CONFLICT (key) DO NOTHING;
