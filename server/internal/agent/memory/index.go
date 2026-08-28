package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// IndexMessage 将一条 context message 写入存储并更新 FTS5 索引。
// 供 Loop 后期在写 message 时调用，不暴露给 Handler 或 Agent 工具。本阶段为空实现。
func IndexMessage(_ context.Context, q *sqlite.Queries, msg ContextMessage) error {
	_ = msg
	if q != nil {
		_ = q.InsertContextMessage
	}
	return nil
}
