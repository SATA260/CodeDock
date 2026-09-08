package agent

import (
	"context"

	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

// q 返回当前上下文可用的 Queries：若上下文存在事务则返回 WithTx 版本，否则返回主 Queries。
func (r *Runtime) q(ctx context.Context) *sqlite.Queries {
	if r.queries == nil {
		return nil
	}
	if tx, ok := db.TxFromContext(ctx); ok {
		return r.queries.WithTx(tx)
	}
	return r.queries
}
