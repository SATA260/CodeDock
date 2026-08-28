package memory

import (
	"context"

	"codedock/pkg/db/sqlite"
)

// Create 新建指定 scope 的 Markdown 文档。本阶段为空实现。
func Create(_ context.Context, q *sqlite.Queries, memory TextMemory) (TextMemory, error) {
	_ = ByteLen(memory.Content)
	if q != nil {
		_ = q.InsertTextMemory
	}
	return TextMemory{}, nil
}

// Get 读取 Markdown 全文。本阶段为空实现。
func Get(_ context.Context, q *sqlite.Queries, key TextMemoryKey) (TextMemory, error) {
	_ = key
	if q != nil {
		_ = q.GetTextMemory
	}
	return TextMemory{}, nil
}

// Update 覆盖更新 Markdown 全文。本阶段为空实现。
func Update(_ context.Context, q *sqlite.Queries, memory TextMemory) (TextMemory, error) {
	_ = ByteLen(memory.Content)
	if q != nil {
		_ = q.UpdateTextMemory
	}
	return TextMemory{}, nil
}

// Delete 删除指定 scope 的 Markdown 文档。本阶段为空实现。
func Delete(_ context.Context, q *sqlite.Queries, key TextMemoryKey) error {
	_ = key
	if q != nil {
		_ = q.DeleteTextMemory
	}
	return nil
}
