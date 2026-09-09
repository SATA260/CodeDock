CREATE TABLE IF NOT EXISTS step_jobs (
    run_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    phase TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (run_id, step_index)
);

CREATE INDEX IF NOT EXISTS step_jobs_run_status_idx ON step_jobs (run_id, status);
