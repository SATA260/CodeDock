-- name: GetTextMemory :one
SELECT * FROM text_memories
WHERE scope = ? AND scope_id = ?;

-- name: ListTextMemories :many
SELECT * FROM text_memories
ORDER BY updated_at DESC;

-- name: InsertTextMemory :one
INSERT INTO text_memories (
    id, scope, scope_id, content, byte_len, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateTextMemory :one
UPDATE text_memories
SET content = ?, byte_len = ?, updated_at = ?
WHERE scope = ? AND scope_id = ?
RETURNING *;

-- name: DeleteTextMemory :exec
DELETE FROM text_memories
WHERE scope = ? AND scope_id = ?;
