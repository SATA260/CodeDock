-- name: GetRunToolCheckpoint :one
SELECT * FROM run_tool_checkpoints
WHERE run_id = ?;

-- name: UpsertRunToolCheckpoint :one
INSERT INTO run_tool_checkpoints (
    run_id, turn_id, completed_calls, pending_calls, results, approved_calls, denied_calls, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(run_id) DO UPDATE SET
    turn_id = excluded.turn_id,
    completed_calls = excluded.completed_calls,
    pending_calls = excluded.pending_calls,
    results = excluded.results,
    approved_calls = excluded.approved_calls,
    denied_calls = excluded.denied_calls,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteRunToolCheckpoint :exec
DELETE FROM run_tool_checkpoints
WHERE run_id = ?;
