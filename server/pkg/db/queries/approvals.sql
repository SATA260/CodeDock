-- name: GetApproval :one
SELECT * FROM approvals
WHERE id = ?;

-- name: CountSessionApprovals :one
SELECT COUNT(*) FROM approvals
WHERE session_id = ?;

-- name: InsertApproval :one
INSERT INTO approvals (
    id, session_id, run_id, tool_call_id, tool_calls, scope, status, expires_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateApproval :one
UPDATE approvals
SET scope = ?, status = ?, tool_calls = ?
WHERE id = ?
RETURNING *;
