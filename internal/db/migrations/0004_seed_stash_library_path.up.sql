INSERT INTO config (key, value, description) VALUES
    ('stash.library_path', '', 'Stash library root directory for moved files')
ON CONFLICT (key) DO NOTHING;
