package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// IndexMessage 将一条 context message 写入存储并更新 FTS5 索引。
// 供 Loop 后期在写 message 时调用，不暴露给 Handler 或 Agent 工具。
// TODO: 写入 context_messages，并同步 FTS5 索引（含写入/删除触发器）。
func IndexMessage(_ context.Context, q *sqlite.Queries, msg ContextMessage) error {
	_ = msg
	if q != nil {
		_ = q.InsertContextMessage
	}
	return nil
}
