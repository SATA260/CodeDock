-- name: GetAgentEvent :one
SELECT * FROM agent_events
WHERE event_id = ?;

-- name: ListSessionEventsAfter :many
SELECT * FROM agent_events
WHERE session_id = ? AND seq > ?
ORDER BY seq;

-- name: InsertAgentEvent :one
INSERT INTO agent_events (
    event_id, session_id, run_id, turn_id, seq, type, version, occurred_at, payload
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;
