-- name: GetMessage :one
SELECT * FROM messages
WHERE id = ?;

-- name: ListSessionMessages :many
SELECT * FROM messages
WHERE session_id = ?
ORDER BY event_seq, created_at;

-- name: InsertMessage :one
INSERT INTO messages (
    id, session_id, run_id, turn_id, role, content, attachments, tool_calls, event_seq, created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: DeleteMessage :exec
DELETE FROM messages
WHERE id = ?;

-- name: ListMessagesAfterSeq :many
SELECT * FROM messages
WHERE session_id = ? AND event_seq > ?
ORDER BY event_seq, created_at;
