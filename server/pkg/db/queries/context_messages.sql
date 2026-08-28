-- name: InsertContextMessage :one
INSERT INTO context_messages (
    id, workspace_id, session_id, run_id, role, content, created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: SearchContextMessages :many
SELECT
    id,
    workspace_id,
    session_id,
    run_id,
    role,
    content,
    created_at
FROM context_messages
WHERE workspace_id = ?
LIMIT ?;
-- TODO: 按 workspace_id 与 query 做 FTS5 MATCH。
