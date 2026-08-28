-- name: GetTurn :one
SELECT * FROM turns
WHERE id = ?;

-- name: ListRunTurns :many
SELECT * FROM turns
WHERE run_id = ?
ORDER BY number;

-- name: InsertTurn :one
INSERT INTO turns (
    id, run_id, number, status, first_event_seq, last_event_seq, assistant_msg_id, usage_id, started_at, finished_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateTurn :one
UPDATE turns
SET status = ?, first_event_seq = ?, last_event_seq = ?, assistant_msg_id = ?, usage_id = ?, started_at = ?, finished_at = ?
WHERE id = ?
RETURNING *;
