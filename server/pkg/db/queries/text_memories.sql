-- name: GetTextMemory :one
SELECT * FROM text_memories
WHERE scope = ? AND scope_id = ? AND kind = ? AND name = ?;

-- name: ListTextMemories :many
SELECT * FROM text_memories
WHERE scope = ? AND scope_id = ?
ORDER BY kind ASC, name ASC;

-- name: UpsertTextMemory :one
INSERT INTO text_memories (
    id, scope, scope_id, kind, name, content, byte_len, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (scope, scope_id, kind, name) DO UPDATE SET
    content = excluded.content,
    byte_len = excluded.byte_len,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteTextMemory :exec
DELETE FROM text_memories
WHERE scope = ? AND scope_id = ? AND kind = ? AND name = ?;
