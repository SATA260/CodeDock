CREATE TABLE IF NOT EXISTS text_memories (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    content TEXT NOT NULL,
    byte_len INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS text_memories_scope_scope_id
    ON text_memories (scope, scope_id);

CREATE TABLE IF NOT EXISTS context_messages (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS context_messages_fts USING fts5(
    content,
    id UNINDEXED
);
