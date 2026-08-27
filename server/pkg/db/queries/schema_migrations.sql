-- name: ListSchemaMigrations :many
SELECT version, applied_at
FROM schema_migrations
ORDER BY version;
