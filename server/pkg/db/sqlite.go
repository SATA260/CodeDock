package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	dbsqlite "codedock/pkg/db/sqlite"

	_ "modernc.org/sqlite"
)

type sqliteClient struct {
	db      *sql.DB
	queries *dbsqlite.Queries
}

// openSQLite 打开 SQLite，限制单连接并设置 busy_timeout。
func openSQLite(ctx context.Context, cfg Config) (Client, error) {
	if cfg.DSN == "" {
		return nil, ErrDSNRequired
	}
	if path, ok := sqliteFilePath(cfg.DSN); ok {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}

	database, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if _, err := database.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		_ = database.Close()
		return nil, err
	}
	if !strings.Contains(cfg.DSN, "mode=memory") {
		if _, err := database.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
			_ = database.Close()
			return nil, err
		}
	}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}

	return &sqliteClient{
		db:      database,
		queries: dbsqlite.New(database),
	}, nil
}

// Engine 返回 SQLite 引擎标识。
func (c *sqliteClient) Engine() Engine {
	return EngineSQLite
}

// DB 返回底层 *sql.DB。
func (c *sqliteClient) DB() *sql.DB {
	return c.db
}

// Queries 返回 SQLite 专属的 sqlc 查询对象，供 Handler 与运行时直接使用。
func (c *sqliteClient) Queries() *dbsqlite.Queries {
	return c.queries
}

// SQLiteQueries 在引擎为 SQLite 时取出 sqlc Queries。
func SQLiteQueries(client Client) *dbsqlite.Queries {
	sqlite, ok := client.(*sqliteClient)
	if !ok {
		return nil
	}
	return sqlite.Queries()
}

// Close 关闭数据库连接。
func (c *sqliteClient) Close() error {
	return c.db.Close()
}

func sqliteFilePath(dsn string) (string, bool) {
	if strings.Contains(dsn, "mode=memory") {
		return "", false
	}
	path, ok := strings.CutPrefix(dsn, "file:")
	if !ok || path == "" {
		return "", false
	}
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	return path, path != ""
}

type txContextKey struct{}

// TxFromContext 取出 WithTx 放入的数据库事务。
func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx, ok
}

// WithTx 开启事务，把 *sql.Tx 放入 context 后执行 fn。
func (c *sqliteClient) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(context.WithValue(ctx, txContextKey{}, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
