-- name: GetUsageRecord :one
SELECT * FROM usage_records
WHERE id = ?;

-- name: CountUsageByRun :one
SELECT COUNT(*) FROM usage_records
WHERE run_id = ?;

-- name: CountUsageBySession :one
SELECT COUNT(*) FROM usage_records
WHERE session_id = ?;

-- name: InsertUsageRecord :one
INSERT INTO usage_records (
    id, session_id, run_id, turn_id, request_id, provider, model, usage_type,
    cache_creation_input_tokens, cache_read_input_tokens, output_tokens, reasoning_tokens, total_tokens,
    estimated, raw_provider_usage, created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;
