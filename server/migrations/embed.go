package migrations

import "embed"

// FS 包含按文件名排序应用的 SQL 迁移。
//
//go:embed *.sql
var FS embed.FS
