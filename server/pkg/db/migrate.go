package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"codedock/migrations"
	"codedock/pkg/db/sqlite"
)

// Migrate 按文件名顺序应用尚未执行的 SQL 迁移。
// 先确保 schema_migrations 存在，再跳过已记录版本后逐条 Exec 并写入版本号。
func Migrate(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return fmt.Errorf("migrate: database is nil")
	}
	if _, err := database.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);
`); err != nil {
		return fmt.Errorf("migrate: ensure schema_migrations: %w", err)
	}

	queries := sqlite.New(database)
	applied, err := queries.ListSchemaMigrations(ctx)
	if err != nil {
		return fmt.Errorf("migrate: list applied: %w", err)
	}
	seen := make(map[string]struct{}, len(applied))
	for _, row := range applied {
		seen[row.Version] = struct{}{}
	}

	entries, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return fmt.Errorf("migrate: list files: %w", err)
	}
	sort.Strings(entries)

	for _, name := range entries {
		version := strings.TrimSuffix(path.Base(name), ".sql")
		if _, ok := seen[version]; ok {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}
		if _, err := database.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", name, err)
		}
		if _, err := database.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)
`, version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("migrate: record %s: %w", name, err)
		}
	}
	return nil
}
