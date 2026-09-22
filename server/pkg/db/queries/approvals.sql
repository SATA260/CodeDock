-- name: GetApproval :one
SELECT * FROM approvals
WHERE id = ?;

-- name: CountSessionApprovals :one
SELECT COUNT(*) FROM approvals
WHERE session_id = ?;

-- name: InsertApproval :one
INSERT INTO approvals (
    id, session_id, run_id, tool_call_id, tool_calls, scope, status, expires_at, kind
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateApproval :one
UPDATE approvals
SET scope = ?, status = ?, tool_calls = ?
WHERE id = ?
RETURNING *;

-- name: ListPendingApprovals :many
SELECT * FROM approvals
WHERE status = 'pending'
ORDER BY id ASC;

-- name: ListPendingApprovalsBySession :many
SELECT * FROM approvals
WHERE session_id = ? AND status = 'pending'
ORDER BY id ASC;

-- name: CountPendingApprovalsBySession :one
SELECT COUNT(*) FROM approvals
WHERE session_id = ? AND status = 'pending';

-- name: CountPendingApprovals :one
SELECT COUNT(*) FROM approvals
WHERE session_id = ? AND status = 'pending';
