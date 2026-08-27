package db

import (
	"context"
	"database/sql"
	"errors"
)

//go:generate sqlc generate -f ../../sqlc.yml

// Engine 标识 Client 使用的数据库引擎。
type Engine string

const (
	EngineSQLite   Engine = "sqlite"
	EnginePostgres Engine = "postgres"
)

// Config 描述打开统一 Client 所需的引擎和连接信息。
type Config struct {
	Engine Engine
	DSN    string
}

var (
	ErrEngineUnsupported = errors.New("db engine unsupported")
	ErrDSNRequired       = errors.New("db dsn required")
)

// Client 是跨引擎的统一数据库入口。
type Client interface {
	Engine() Engine
	DB() *sql.DB
	Close() error
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Open 按引擎创建 Client。本阶段仅实现 SQLite。
func Open(ctx context.Context, cfg Config) (Client, error) {
	switch cfg.Engine {
	case EngineSQLite:
		return openSQLite(ctx, cfg)
	default:
		return nil, ErrEngineUnsupported
	}
}
