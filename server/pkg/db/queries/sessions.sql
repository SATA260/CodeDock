-- name: GetSession :one
SELECT * FROM sessions
WHERE id = ?;

-- name: ListSessions :many
SELECT * FROM sessions
ORDER BY updated_at DESC;

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
