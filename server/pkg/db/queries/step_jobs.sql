-- name: UpsertStepJob :one
INSERT INTO step_jobs (
    run_id, step_index, phase, payload, status, attempt, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(run_id, step_index) DO UPDATE SET
    phase = excluded.phase,
    payload = excluded.payload,
    status = excluded.status,
    attempt = excluded.attempt,
    updated_at = excluded.updated_at
RETURNING *;

-- name: UpdateStepJobStatus :exec
UPDATE step_jobs
SET status = ?, updated_at = ?
WHERE run_id = ? AND step_index = ?;

-- name: GetStepJob :one
SELECT * FROM step_jobs
WHERE run_id = ? AND step_index = ?;

-- name: GetLatestOpenStepJob :one
SELECT * FROM step_jobs
WHERE run_id = ? AND status IN ('queued', 'running')
ORDER BY step_index DESC
LIMIT 1;

-- name: CancelOpenStepJobs :exec
UPDATE step_jobs
SET status = 'cancelled', updated_at = ?
WHERE run_id = ? AND status IN ('queued', 'running');
