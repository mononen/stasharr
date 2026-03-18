INSERT INTO config (key, value, description) VALUES
    ('stash.library_path', '', 'Stash library root directory for moved files'),
    ('sabnzbd.remote_path', '', 'Download path as reported by SABnzbd'),
    ('sabnzbd.local_path', '', 'Corresponding local path visible to stasharr')
ON CONFLICT (key) DO NOTHING;
