-- name: InsertContextMessage :one
INSERT INTO context_messages (
    id, workspace_id, session_id, run_id, role, content, created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(id) DO UPDATE SET
    workspace_id = excluded.workspace_id,
    session_id = excluded.session_id,
    run_id = excluded.run_id,
    role = excluded.role,
    content = excluded.content,
    created_at = excluded.created_at
RETURNING *;
