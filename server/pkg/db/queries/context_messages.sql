-- name: InsertContextMessage :one
INSERT INTO context_messages (
    id, project_id, group_id, session_id, run_id, role, content, created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: SearchContextMessages :many
SELECT
    id,
    project_id,
    group_id,
    session_id,
    run_id,
    role,
    content,
    created_at
FROM context_messages
WHERE project_id = ?
LIMIT ?;
