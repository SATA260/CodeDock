CREATE TABLE IF NOT EXISTS run_tool_checkpoints (
    run_id TEXT PRIMARY KEY,
    turn_id TEXT NOT NULL,
    completed_calls TEXT NOT NULL,
    pending_calls TEXT NOT NULL,
    results TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
