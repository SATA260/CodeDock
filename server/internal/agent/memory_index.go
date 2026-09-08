package agent

import (
	"context"
	"time"

	"codedock/internal/agent/memory"
	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
)

// compactKey 把记忆目录键编码为去重字符串，防止重复压缩。
func compactKey(key memory.TextMemoryKey) string {
	return string(key.Scope) + "/" + key.ScopeID + "/" + string(key.Kind) + "/" + key.Name
}

// EnqueueIndexCompact 在后台异步压缩超限目录，不会阻塞调用方。
func (r *Runtime) EnqueueIndexCompact(key memory.TextMemoryKey) {
	if r == nil || key.Kind != memory.KindIndex {
		return
	}
	id := compactKey(key)
	if _, loaded := r.compact.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	r.compactWG.Add(1)
	go func() {
		defer r.compactWG.Done()
		defer r.compact.Delete(id)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		r.compactIndex(ctx, key)
	}()
}

// WaitIndexCompact 等待所有已入队的目录压缩任务结束。仅用于测试。
func (r *Runtime) WaitIndexCompact() {
	if r == nil {
		return
	}
	r.compactWG.Wait()
}

// compactIndex 对单个超限目录执行压缩：读取、摘要、截断、回写。
func (r *Runtime) compactIndex(ctx context.Context, key memory.TextMemoryKey) {
	item, err := memory.Get(ctx, r.q(ctx), key)
	if err != nil {
		if !cderr.IsNotFound(err) {
			r.logger().Error("index compact get failed", "error", err, "scope", key.Scope, "scope_id", key.ScopeID)
		}
		return
	}
	if item.Kind != memory.KindIndex || !item.OverBudget {
		return
	}
	summary, err := pkgagent.CompactIndex(ctx, r.model, item.Content)
	if err != nil {
		r.logger().Error("index compact model failed", "error", err, "scope", key.Scope, "scope_id", key.ScopeID)
		return
	}
	if summary == "" || memory.IndexOverBudget(summary) {
		summary = memory.ClipIndex(item.Content)
	}
	if memory.IndexOverBudget(summary) {
		summary = memory.ClipIndex(summary)
	}
	if _, err := memory.Upsert(ctx, r.q(ctx), memory.TextMemory{
		ID:      item.ID,
		Scope:   item.Scope,
		ScopeID: item.ScopeID,
		Kind:    item.Kind,
		Name:    item.Name,
		Content: summary,
	}); err != nil {
		r.logger().Error("index compact upsert failed", "error", err, "scope", key.Scope, "scope_id", key.ScopeID)
	}
}
