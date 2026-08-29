package memory

import (
	"context"
	"strings"
	"unicode"

	cderr "codedock/internal/errors"
	"codedock/pkg/db/sqlite"
)

// SearchMessages 按 query 与工作区检索已索引的 context message。
func SearchMessages(ctx context.Context, q *sqlite.Queries, search Search) ([]MessageHit, error) {
	if q == nil {
		return nil, cderr.Invalid("queries required")
	}
	if search.WorkspaceID == "" {
		return nil, cderr.Invalid("workspace_id is required")
	}
	query := strings.TrimSpace(search.Query)
	if query == "" {
		return nil, cderr.Invalid("query is required")
	}
	limit := int64(search.Limit)
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	rows, err := q.SearchContextMessages(ctx, sqlite.SearchContextMessagesParams{
		Query:       ftsQuery(query),
		WorkspaceID: search.WorkspaceID,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	hits := make([]MessageHit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, MessageHit{
			Message: ContextMessage{
				ID:          row.ID,
				WorkspaceID: row.WorkspaceID,
				SessionID:   row.SessionID,
				RunID:       row.RunID,
				Role:        row.Role,
				Content:     row.Content,
				CreatedAt:   parseTime(row.CreatedAt),
			},
			Rank: row.Rank,
		})
	}
	return hits, nil
}

func ftsQuery(query string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range query {
		if r == '"' {
			b.WriteByte(' ')
			continue
		}
		if unicode.IsControl(r) {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
