-- name: GetSessionLease :one
SELECT * FROM session_leases
WHERE session_id = ?;

-- name: UpsertSessionLease :one
INSERT INTO session_leases (
    session_id, run_id, owner, fencing_token, heartbeat_at, expires_at
) VALUES (
    ?, ?, ?, ?, ?, ?
)
ON CONFLICT(session_id) DO UPDATE SET
    run_id = excluded.run_id,
    owner = excluded.owner,
    fencing_token = excluded.fencing_token,
    heartbeat_at = excluded.heartbeat_at,
    expires_at = excluded.expires_at
RETURNING *;

-- name: DeleteSessionLease :exec
DELETE FROM session_leases
WHERE session_id = ?;
