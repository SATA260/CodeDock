package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// SearchMessages 按 query 与 scope 检索已索引的 context message。本阶段为空实现。
func SearchMessages(_ context.Context, q *sqlite.Queries, search Search) ([]MessageHit, error) {
	_ = search
	if q != nil {
		_ = q.SearchContextMessages
	}
	return nil, nil
}
