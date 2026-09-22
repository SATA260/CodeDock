-- name: GetWork :one
SELECT * FROM works
WHERE id = ?;

-- name: ListWorksByUser :many
SELECT * FROM works
WHERE tenant_id = ? AND user_id = ?
ORDER BY updated_at DESC;

-- name: InsertWork :one
INSERT INTO works (
    id, tenant_id, user_id, title, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateWork :one
UPDATE works
SET title = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: TouchWork :exec
UPDATE works
SET updated_at = ?
WHERE id = ?;

-- name: DeleteWork :exec
DELETE FROM works
WHERE id = ?;

-- name: GetWorkInfo :one
SELECT * FROM work_infos
WHERE work_id = ? AND checkout = ?;

-- name: ListWorkInfos :many
SELECT * FROM work_infos
WHERE work_id = ?;

-- name: UpsertWorkInfo :one
INSERT INTO work_infos (
    work_id, checkout, body, updated_at
) VALUES (
    ?, ?, ?, ?
)
ON CONFLICT (work_id, checkout) DO UPDATE SET
    body = excluded.body,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteWorkInfos :exec
DELETE FROM work_infos
WHERE work_id = ?;

-- name: DeleteWorkInfo :exec
DELETE FROM work_infos
WHERE work_id = ? AND checkout = ?;

-- name: GetWorkCheckout :one
SELECT * FROM work_checkouts
WHERE work_id = ? AND path = ?;

-- name: ListWorkCheckouts :many
SELECT * FROM work_checkouts
WHERE work_id = ?
ORDER BY path ASC;

-- name: InsertWorkCheckout :one
INSERT INTO work_checkouts (
    work_id, path, kind
) VALUES (
    ?, ?, ?
)
RETURNING *;

-- name: DeleteWorkCheckout :exec
DELETE FROM work_checkouts
WHERE work_id = ? AND path = ?;

-- name: DeleteWorkCheckouts :exec
DELETE FROM work_checkouts
WHERE work_id = ?;

-- name: GetSessionPlacement :one
SELECT * FROM session_placements
WHERE engine = ? AND session_id = ?;

-- name: ListPlacementsByWork :many
SELECT * FROM session_placements
WHERE work_id = ?
ORDER BY session_id ASC;

-- name: ListPlacementsByUser :many
SELECT p.engine, p.session_id, p.work_id, p.checkout
FROM session_placements p
INNER JOIN works w ON w.id = p.work_id
WHERE w.tenant_id = ? AND w.user_id = ?
ORDER BY w.updated_at DESC, p.session_id ASC;

-- name: InsertSessionPlacement :one
INSERT INTO session_placements (
    engine, session_id, work_id, checkout
) VALUES (
    ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateSessionPlacement :one
UPDATE session_placements
SET work_id = ?, checkout = ?
WHERE engine = ? AND session_id = ?
RETURNING *;

-- name: ClearPlacementCheckout :exec
UPDATE session_placements
SET checkout = ''
WHERE work_id = ? AND checkout = ?;

-- name: DeleteSessionPlacement :exec
DELETE FROM session_placements
WHERE engine = ? AND session_id = ?;

-- name: DeletePlacementsByWork :exec
DELETE FROM session_placements
WHERE work_id = ?;

-- name: ListUngroupedNativeSessions :many
SELECT s.*
FROM sessions s
LEFT JOIN session_placements p
    ON p.engine = 'native' AND p.session_id = s.id
WHERE s.tenant_id = ? AND s.user_id = ?
  AND s.status != 'archived'
  AND p.session_id IS NULL
ORDER BY s.updated_at DESC;

-- name: GetSessionIssue :one
SELECT * FROM session_issues
WHERE engine = ? AND session_id = ?;

-- name: UpsertSessionIssue :one
INSERT INTO session_issues (
    engine, session_id, repo, number, title, body, url, state, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (engine, session_id) DO UPDATE SET
    repo = excluded.repo,
    number = excluded.number,
    title = excluded.title,
    body = excluded.body,
    url = excluded.url,
    state = excluded.state,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteSessionIssue :exec
DELETE FROM session_issues
WHERE engine = ? AND session_id = ?;

-- name: ListSessionPulls :many
SELECT * FROM session_pulls
WHERE engine = ? AND session_id = ?
ORDER BY number ASC;

-- name: UpsertSessionPull :one
INSERT INTO session_pulls (
    engine, session_id, repo, number, title, body, url, state, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (engine, session_id, repo, number) DO UPDATE SET
    title = excluded.title,
    body = excluded.body,
    url = excluded.url,
    state = excluded.state,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteSessionPull :exec
DELETE FROM session_pulls
WHERE engine = ? AND session_id = ? AND repo = ? AND number = ?;

-- name: DeleteSessionPulls :exec
DELETE FROM session_pulls
WHERE engine = ? AND session_id = ?;
