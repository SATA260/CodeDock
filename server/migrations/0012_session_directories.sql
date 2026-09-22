CREATE TABLE IF NOT EXISTS session_directories (
    engine TEXT NOT NULL,
    session_id TEXT NOT NULL,
    path TEXT NOT NULL,
    PRIMARY KEY (engine, session_id)
);
