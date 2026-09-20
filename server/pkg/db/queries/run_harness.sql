-- name: GetRunHarness :one
SELECT snapshot_oid, harness FROM runs
WHERE id = ?;

-- name: UpdateRunHarness :exec
UPDATE runs
SET snapshot_oid = ?, harness = ?
WHERE id = ?;
