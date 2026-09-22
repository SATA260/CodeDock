CREATE TABLE IF NOT EXISTS works (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_works_user_updated
    ON works (tenant_id, user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS work_infos (
    work_id TEXT NOT NULL,
    checkout TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (work_id, checkout)
);

CREATE TABLE IF NOT EXISTS work_checkouts (
    work_id TEXT NOT NULL,
    path TEXT NOT NULL,
    kind TEXT NOT NULL,
    PRIMARY KEY (work_id, path)
);

CREATE TABLE IF NOT EXISTS session_placements (
    engine TEXT NOT NULL,
    session_id TEXT NOT NULL,
    work_id TEXT NOT NULL,
    checkout TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (engine, session_id)
);

CREATE INDEX IF NOT EXISTS idx_session_placements_work
    ON session_placements (work_id);

CREATE TABLE IF NOT EXISTS session_issues (
    engine TEXT NOT NULL,
    session_id TEXT NOT NULL,
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (engine, session_id)
);

CREATE TABLE IF NOT EXISTS session_pulls (
    engine TEXT NOT NULL,
    session_id TEXT NOT NULL,
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (engine, session_id, repo, number)
);
