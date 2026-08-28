CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    status TEXT NOT NULL,
    active_run_id TEXT,
    last_event_seq INTEGER NOT NULL DEFAULT 0,
    compaction_seq INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    trigger_message_id TEXT NOT NULL,
    mode TEXT NOT NULL,
    config TEXT NOT NULL,
    status TEXT NOT NULL,
    current_turn_id TEXT,
    stop_reason TEXT,
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    started_at TEXT,
    finished_at TEXT
);

CREATE TABLE IF NOT EXISTS turns (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    number INTEGER NOT NULL,
    status TEXT NOT NULL,
    first_event_seq INTEGER NOT NULL DEFAULT 0,
    last_event_seq INTEGER NOT NULL DEFAULT 0,
    assistant_msg_id TEXT,
    usage_id TEXT,
    started_at TEXT,
    finished_at TEXT
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT,
    turn_id TEXT,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    attachments TEXT,
    tool_calls TEXT,
    event_seq INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_events (
    event_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    turn_id TEXT,
    seq INTEGER NOT NULL,
    type TEXT NOT NULL,
    version INTEGER NOT NULL,
    occurred_at TEXT NOT NULL,
    payload TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS approvals (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    tool_call_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    status TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS usage_records (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    usage_type TEXT NOT NULL,
    cache_creation_input_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    estimated INTEGER NOT NULL DEFAULT 0,
    raw_provider_usage TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS compaction_checkpoints (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    base_event_seq INTEGER NOT NULL,
    summary TEXT NOT NULL,
    created_by_run TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS session_leases (
    session_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    owner TEXT NOT NULL,
    fencing_token INTEGER NOT NULL,
    heartbeat_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
