-- name: GetCompactionCheckpoint :one
SELECT * FROM compaction_checkpoints
WHERE id = ?;

-- name: GetLatestCheckpoint :one
SELECT * FROM compaction_checkpoints
WHERE session_id = ?
ORDER BY base_event_seq DESC
LIMIT 1;

-- name: InsertCompactionCheckpoint :one
INSERT INTO compaction_checkpoints (
    id, session_id, base_event_seq, summary, created_by_run, created_at
) VALUES (
    ?, ?, ?, ?, ?, ?
)
RETURNING *;
