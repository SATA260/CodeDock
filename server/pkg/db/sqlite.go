package db

import (
	"context"
	"database/sql"

	dbsqlite "codedock/pkg/db/sqlite"

	_ "modernc.org/sqlite"
)

type sqliteClient struct {
	db      *sql.DB
	queries *dbsqlite.Queries
}

func openSQLite(ctx context.Context, cfg Config) (Client, error) {
	if cfg.DSN == "" {
		return nil, ErrDSNRequired
	}

	database, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, err
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

func (c *sqliteClient) Engine() Engine {
	return EngineSQLite
}

func (c *sqliteClient) DB() *sql.DB {
	return c.db
}

// Queries 返回 SQLite 专属的 sqlc 查询对象，供后续 Store 使用。
func (c *sqliteClient) Queries() *dbsqlite.Queries {
	return c.queries
}

func (c *sqliteClient) Close() error {
	return c.db.Close()
}

type txContextKey struct{}

// TxFromContext 取出 WithTx 放入的数据库事务。
func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx, ok
}

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
