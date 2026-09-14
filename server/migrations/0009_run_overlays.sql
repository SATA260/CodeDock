CREATE TABLE IF NOT EXISTS run_overlays (
    run_id TEXT PRIMARY KEY,
    system_prompt TEXT NOT NULL DEFAULT '',
    hidden TEXT NOT NULL DEFAULT '[]',
    updated_at TEXT NOT NULL
);
