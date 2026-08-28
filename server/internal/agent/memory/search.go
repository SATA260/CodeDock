package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// SearchMessages 按 query 与工作区检索已索引的 context message。
// TODO: 按 workspace_id 与 query 做 FTS5 MATCH 并返回命中。
func SearchMessages(_ context.Context, q *sqlite.Queries, search Search) ([]MessageHit, error) {
	_ = search
	if q != nil {
		_ = q.SearchContextMessages
	}
	return nil, nil
}
