package memory

import (
	"context"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	"codedock/pkg/db/sqlite"
)

// IndexMessage 将一条 context message 写入存储并更新 FTS5 索引。
// 供 Loop 在写 message 时调用，不暴露给 Handler 或 Agent 工具。
func IndexMessage(ctx context.Context, q *sqlite.Queries, msg ContextMessage) error {
	if q == nil {
		return cderr.Invalid("queries required")
	}
	if msg.ID == "" || msg.WorkspaceID == "" {
		return cderr.Invalid("context message id and workspace_id are required")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = util.Now()
	}
	if _, err := q.InsertContextMessage(ctx, sqlite.InsertContextMessageParams{
		ID:          msg.ID,
		WorkspaceID: msg.WorkspaceID,
		SessionID:   msg.SessionID,
		RunID:       msg.RunID,
		Role:        msg.Role,
		Content:     msg.Content,
		CreatedAt:   util.FormatTime(msg.CreatedAt),
	}); err != nil {
		return err
	}
	if err := q.DeleteContextMessageFTS(ctx, msg.ID); err != nil {
		return err
	}
	return q.InsertContextMessageFTS(ctx, sqlite.InsertContextMessageFTSParams{
		ID:      msg.ID,
		Content: msg.Content,
	})
}
