-- name: GetRunOverlay :one
SELECT * FROM run_overlays
WHERE run_id = ?;

-- name: UpsertRunOverlay :one
INSERT INTO run_overlays (
    run_id, system_prompt, hidden, updated_at
) VALUES (
    ?, ?, ?, ?
)
ON CONFLICT(run_id) DO UPDATE SET
    system_prompt = excluded.system_prompt,
    hidden = excluded.hidden,
    updated_at = excluded.updated_at
RETURNING *;
