ALTER TABLE text_memories ADD COLUMN kind TEXT NOT NULL DEFAULT 'index';
ALTER TABLE text_memories ADD COLUMN name TEXT NOT NULL DEFAULT 'index';

DROP INDEX IF EXISTS text_memories_scope_scope_id;
CREATE UNIQUE INDEX IF NOT EXISTS text_memories_scope_key
    ON text_memories (scope, scope_id, kind, name);

ALTER TABLE sessions ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'default';

DROP TABLE IF EXISTS context_messages_fts;
DROP TABLE IF EXISTS context_messages;

CREATE TABLE context_messages (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE context_messages_fts USING fts5(
    content,
    id UNINDEXED
);
