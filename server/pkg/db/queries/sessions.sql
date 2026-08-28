-- name: GetSession :one
SELECT * FROM sessions
WHERE id = ?;

-- name: CountSessions :one
SELECT COUNT(*) FROM sessions;

-- name: InsertSession :one
INSERT INTO sessions (
    id, tenant_id, user_id, agent_id, status, active_run_id, last_event_seq, compaction_seq, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateSession :one
UPDATE sessions
SET agent_id = ?, status = ?, active_run_id = ?, last_event_seq = ?, compaction_seq = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: ArchiveSession :exec
UPDATE sessions
SET status = 'archived', updated_at = ?
WHERE id = ?;

-- name: ClaimActiveRun :one
UPDATE sessions
SET active_run_id = ?, updated_at = ?
WHERE id = ? AND active_run_id IS NULL
RETURNING *;

-- name: ClearActiveRun :exec
UPDATE sessions
SET active_run_id = NULL, updated_at = ?
WHERE id = ? AND active_run_id = ?;

-- name: IncrementEventSeq :one
UPDATE sessions
SET last_event_seq = last_event_seq + 1, updated_at = ?
WHERE id = ?
RETURNING last_event_seq;

-- name: UpdateCompactionSeq :exec
UPDATE sessions
SET compaction_seq = ?, updated_at = ?
WHERE id = ?;
