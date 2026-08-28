package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// Get 读取一篇目录或专题。
// TODO: 按 scope、scope_id、kind、name 读取 Markdown。
func Get(_ context.Context, q *sqlite.Queries, key TextMemoryKey) (TextMemory, error) {
	_ = key
	if q != nil {
		_ = q.GetTextMemory
	}
	return TextMemory{}, nil
}

// Upsert 覆盖写入一篇目录或专题。
// TODO: 按唯一键写入全文，并用 ByteLen 填充字节长度；目录超限时仍写入。
func Upsert(_ context.Context, q *sqlite.Queries, memory TextMemory) (TextMemory, error) {
	_ = ByteLen(memory.Content)
	_ = IndexOverBudget(memory.Content)
	if q != nil {
		_ = q.UpsertTextMemory
	}
	return TextMemory{}, nil
}

// Delete 删除一篇目录或专题。
// TODO: 按 scope、scope_id、kind、name 删除。
func Delete(_ context.Context, q *sqlite.Queries, key TextMemoryKey) error {
	_ = key
	if q != nil {
		_ = q.DeleteTextMemory
	}
	return nil
}

// List 列出某一 scope 下的目录与专题。
// TODO: 按 scope 与 scope_id 列出记忆。
func List(_ context.Context, q *sqlite.Queries, scope TextMemoryScope, scopeID string) ([]TextMemory, error) {
	_ = scope
	_ = scopeID
	if q != nil {
		_ = q.ListTextMemories
	}
	return nil, nil
}
