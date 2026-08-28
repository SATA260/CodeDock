-- name: GetRun :one
SELECT * FROM runs
WHERE id = ?;

-- name: ListSessionRuns :many
SELECT * FROM runs
WHERE session_id = ?
ORDER BY id;

-- name: InsertRun :one
INSERT INTO runs (
    id, session_id, trigger_message_id, mode, config, status, current_turn_id, stop_reason, cancel_requested, started_at, finished_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateRun :one
UPDATE runs
SET status = ?, current_turn_id = ?, stop_reason = ?, cancel_requested = ?, started_at = ?, finished_at = ?
WHERE id = ?
RETURNING *;
