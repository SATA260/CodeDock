package sqlite

import (
	"context"
	"database/sql"
)

const deleteContextMessageFTS = `DELETE FROM context_messages_fts WHERE id = ?`

const insertContextMessageFTS = `INSERT INTO context_messages_fts (id, content) VALUES (?, ?)`

const searchContextMessages = `
SELECT
    cm.id,
    cm.workspace_id,
    cm.session_id,
    cm.run_id,
    cm.role,
    cm.content,
    cm.created_at,
    bm25(context_messages_fts) AS hit_rank
FROM context_messages_fts
JOIN context_messages cm ON cm.id = context_messages_fts.id
WHERE context_messages_fts MATCH ?
  AND cm.workspace_id = ?
ORDER BY hit_rank
LIMIT ?
`

// DeleteContextMessageFTS 删除一条 FTS 索引。
func (q *Queries) DeleteContextMessageFTS(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, deleteContextMessageFTS, id)
	return err
}

type InsertContextMessageFTSParams struct {
	ID      string
	Content string
}

// InsertContextMessageFTS 写入一条 FTS 索引。
func (q *Queries) InsertContextMessageFTS(ctx context.Context, arg InsertContextMessageFTSParams) error {
	_, err := q.db.ExecContext(ctx, insertContextMessageFTS, arg.ID, arg.Content)
	return err
}

type SearchContextMessagesParams struct {
	Query       string
	WorkspaceID string
	Limit       int64
}

type SearchContextMessagesRow struct {
	ID          string
	WorkspaceID string
	SessionID   string
	RunID       string
	Role        string
	Content     string
	CreatedAt   string
	Rank        float64
}

// SearchContextMessages 按 FTS MATCH 与 workspace_id 检索消息。
func (q *Queries) SearchContextMessages(ctx context.Context, arg SearchContextMessagesParams) ([]SearchContextMessagesRow, error) {
	rows, err := q.db.QueryContext(ctx, searchContextMessages, arg.Query, arg.WorkspaceID, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SearchContextMessagesRow
	for rows.Next() {
		var i SearchContextMessagesRow
		var rank sql.NullFloat64
		if err := rows.Scan(
			&i.ID,
			&i.WorkspaceID,
			&i.SessionID,
			&i.RunID,
			&i.Role,
			&i.Content,
			&i.CreatedAt,
			&rank,
		); err != nil {
			return nil, err
		}
		if rank.Valid {
			i.Rank = rank.Float64
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
