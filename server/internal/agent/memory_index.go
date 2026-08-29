package agent

import (
	"context"
	"encoding/json"
	"time"

	"codedock/internal/agent/memory"
	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
)

type frozenIndexes struct {
	compactionSeq int64
	user          string
	workspace     string
}

func compactKey(key memory.TextMemoryKey) string {
	return string(key.Scope) + "/" + key.ScopeID + "/" + string(key.Kind) + "/" + key.Name
}

// EnqueueIndexCompact 后台压缩超限目录，不阻塞调用方。
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

// WaitIndexCompact 等待已入队的目录压缩结束，供测试使用。
func (r *Runtime) WaitIndexCompact() {
	if r == nil {
		return
	}
	r.compactWG.Wait()
}

// FrozenMemoryIndexes 返回已冻结的目录正文（不含标题），供测试使用。
func (r *Runtime) FrozenMemoryIndexes(sessionID string) (user, workspace string, ok bool) {
	if r == nil {
		return "", "", false
	}
	value, loaded := r.freeze.Load(sessionID)
	if !loaded {
		return "", "", false
	}
	frozen, ok := value.(frozenIndexes)
	if !ok {
		return "", "", false
	}
	return frozen.user, frozen.workspace, true
}

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

func (r *Runtime) loadMemoryIndexes(ctx context.Context, session pkgagent.Session) []string {
	if cached, ok := r.freeze.Load(session.ID); ok {
		if frozen, ok := cached.(frozenIndexes); ok && frozen.compactionSeq == session.CompactionSeq {
			return formatMemoryIndexes(frozen.user, frozen.workspace)
		}
	}
	user := r.readIndex(ctx, memory.ScopeUser, session.UserID)
	workspaceID := session.WorkspaceID
	if workspaceID == "" {
		workspaceID = "default"
	}
	workspace := r.readIndex(ctx, memory.ScopeWorkspace, workspaceID)
	r.freeze.Store(session.ID, frozenIndexes{
		compactionSeq: session.CompactionSeq,
		user:          user,
		workspace:     workspace,
	})
	return formatMemoryIndexes(user, workspace)
}

func (r *Runtime) readIndex(ctx context.Context, scope memory.TextMemoryScope, scopeID string) string {
	if scopeID == "" {
		return ""
	}
	item, err := memory.Get(ctx, r.q(ctx), memory.TextMemoryKey{
		Scope:   scope,
		ScopeID: scopeID,
		Kind:    memory.KindIndex,
		Name:    memory.NameIndex,
	})
	if err != nil {
		if !cderr.IsNotFound(err) {
			r.logger().Error("load memory index failed", "error", err, "scope", scope, "scope_id", scopeID)
		}
		return ""
	}
	if item.OverBudget {
		r.EnqueueIndexCompact(memory.TextMemoryKey{Scope: item.Scope, ScopeID: item.ScopeID, Kind: item.Kind, Name: item.Name})
	}
	return memory.ClipIndex(item.Content)
}

func formatMemoryIndexes(user, workspace string) []string {
	var out []string
	if user != "" {
		out = append(out, "## User memory index\n"+user)
	}
	if workspace != "" {
		out = append(out, "## Workspace memory index\n"+workspace)
	}
	return out
}

func (r *Runtime) getSession(ctx context.Context, id string) (pkgagent.Session, error) {
	row, err := r.q(ctx).GetSession(ctx, id)
	if err != nil {
		return pkgagent.Session{}, wrapDB(err)
	}
	return mapSession(row), nil
}

func (r *Runtime) indexPersistedMessage(ctx context.Context, session pkgagent.Session, msg pkgagent.Message) {
	if msg.ID == "" {
		return
	}
	content := pkgagent.DecodeText(msg.Content)
	if msg.Role == pkgagent.RoleTool {
		var result pkgagent.ToolResultContent
		if err := json.Unmarshal(msg.Content, &result); err == nil && len(result.Output) > 0 {
			content = string(result.Output)
		}
	}
	workspaceID := session.WorkspaceID
	if workspaceID == "" {
		workspaceID = "default"
	}
	if err := memory.IndexMessage(ctx, r.q(ctx), memory.ContextMessage{
		ID:          msg.ID,
		WorkspaceID: workspaceID,
		SessionID:   msg.SessionID,
		RunID:       deref(msg.RunID),
		Role:        string(msg.Role),
		Content:     content,
		CreatedAt:   msg.CreatedAt,
	}); err != nil {
		r.logger().Error("index message failed", "error", err, "message_id", msg.ID)
	}
}

func (r *Runtime) indexLoadedMessages(ctx context.Context, session pkgagent.Session, messages []pkgagent.Message) {
	for _, msg := range messages {
		r.indexPersistedMessage(ctx, session, msg)
	}
}
