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

// loadMemoryIndexes 读取用户与工作区冻结目录，供本 Session 装上下文。
func (r *Runtime) loadMemoryIndexes(ctx context.Context, userID, workspaceID string) []string {
	if r == nil {
		return nil
	}
	q := r.q(ctx)
	var out []string
	if userID != "" {
		if item, err := memory.Get(ctx, q, memory.TextMemoryKey{
			Scope:   memory.ScopeUser,
			ScopeID: userID,
			Kind:    memory.KindIndex,
			Name:    memory.NameIndex,
		}); err == nil && item.Content != "" {
			out = append(out, item.Content)
		}
	}
	if workspaceID != "" {
		if item, err := memory.Get(ctx, q, memory.TextMemoryKey{
			Scope:   memory.ScopeWorkspace,
			ScopeID: workspaceID,
			Kind:    memory.KindIndex,
			Name:    memory.NameIndex,
		}); err == nil && item.Content != "" {
			out = append(out, item.Content)
		}
	}
	return out
}

// indexPersisted 把已落库消息写入冷层 FTS。
func (r *Runtime) indexPersisted(ctx context.Context, workspaceID string, msg pkgagent.Message) {
	if r == nil || workspaceID == "" || msg.ID == "" {
		return
	}
	content := pkgagent.DecodeText(msg.Content)
	if content == "" {
		return
	}
	runID := deref(msg.RunID)
	if err := memory.IndexMessage(ctx, r.q(ctx), memory.ContextMessage{
		ID:          msg.ID,
		WorkspaceID: workspaceID,
		SessionID:   msg.SessionID,
		RunID:       runID,
		Role:        string(msg.Role),
		Content:     content,
		CreatedAt:   msg.CreatedAt,
	}); err != nil {
		r.logger().Error("index message failed", "message_id", msg.ID, "error", err)
	}
}
