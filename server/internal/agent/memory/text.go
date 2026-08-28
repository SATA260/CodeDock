package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// Create 新建指定 scope 的 Markdown 文档。
// TODO: 调用 sqlc InsertTextMemory 写入全文，并用 ByteLen 填充字节长度。
func Create(_ context.Context, q *sqlite.Queries, memory TextMemory) (TextMemory, error) {
	_ = ByteLen(memory.Content)
	if q != nil {
		_ = q.InsertTextMemory
	}
	return TextMemory{}, nil
}

// Get 读取 Markdown 全文。
// TODO: 按 scope 与 scope_id 读取 Markdown 全文。
func Get(_ context.Context, q *sqlite.Queries, key TextMemoryKey) (TextMemory, error) {
	_ = key
	if q != nil {
		_ = q.GetTextMemory
	}
	return TextMemory{}, nil
}

// Update 覆盖更新 Markdown 全文。
// TODO: 覆盖更新 Markdown 全文与 ByteLen。
func Update(_ context.Context, q *sqlite.Queries, memory TextMemory) (TextMemory, error) {
	_ = ByteLen(memory.Content)
	if q != nil {
		_ = q.UpdateTextMemory
	}
	return TextMemory{}, nil
}

// Delete 删除指定 scope 的 Markdown 文档。
// TODO: 按 scope 与 scope_id 删除 Markdown 文档。
func Delete(_ context.Context, q *sqlite.Queries, key TextMemoryKey) error {
	_ = key
	if q != nil {
		_ = q.DeleteTextMemory
	}
	return nil
}
