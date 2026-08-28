package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// SearchMessages 按 query 与 scope 检索已索引的 context message。
// TODO: 按 query 与 scope 做 FTS5 检索并返回命中。
func SearchMessages(_ context.Context, q *sqlite.Queries, search Search) ([]MessageHit, error) {
	_ = search
	if q != nil {
		_ = q.SearchContextMessages
	}
	return nil, nil
}
