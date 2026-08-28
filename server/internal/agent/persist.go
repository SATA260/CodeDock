package agent

import (
	"context"

	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
)

// PersistTransition 写入 Run 的下一状态和对应事件。
// TODO: 更新 Run 状态并写入对应事件。
func (r *Runtime) PersistTransition(ctx context.Context, _ string, next pkgagent.RunStatus, reason string) error {
	_ = next
	_ = reason
	_ = r.persistRun(ctx)
	_ = r.persistEvent(ctx)
	r.publish(ctx, pkgagent.AgentEvent{})
	return nil
}

// persistRun 写入 Run。
// TODO: 读取或写入 Run。
func (r *Runtime) persistRun(_ context.Context) error {
	if r.queries == nil {
		return nil
	}
	_ = r.queries.GetRun
	_ = r.queries.InsertRun
	_ = r.queries.UpdateRun
	return nil
}

// persistTurn 写入 Turn。
// TODO: 读取或写入 Turn。
func (r *Runtime) persistTurn(_ context.Context) error {
	if r.queries == nil {
		return nil
	}
	_ = r.queries.GetTurn
	_ = r.queries.InsertTurn
	_ = r.queries.UpdateTurn
	return nil
}

// persistMessage 写入助手或工具消息。
// TODO: 写入助手或工具消息，并在后期调用 memory.IndexMessage。
func (r *Runtime) persistMessage(_ context.Context) error {
	if r.queries == nil {
		return nil
	}
	_ = r.queries.InsertMessage
	return nil
}

// persistUsage 写入用量。
// TODO: 写入用量记录。
func (r *Runtime) persistUsage(_ context.Context) error {
	if r.queries == nil {
		return nil
	}
	_ = r.queries.InsertUsageRecord
	return nil
}

// persistEvent 写入 Agent 事件。
// TODO: 写入 Agent 事件。
func (r *Runtime) persistEvent(_ context.Context) error {
	if r.queries == nil {
		return nil
	}
	_ = r.queries.InsertAgentEvent
	return nil
}

// persistCheckpoint 写入压缩 checkpoint。
// TODO: 读取最近 checkpoint 或写入新 checkpoint。
func (r *Runtime) persistCheckpoint(_ context.Context) error {
	if r.queries == nil {
		return nil
	}
	_ = r.queries.GetLatestCheckpoint
	_ = r.queries.InsertCompactionCheckpoint
	return nil
}

// persistLease 写入会话级 lease。
// TODO: 占用或刷新会话级 lease。
func (r *Runtime) persistLease(_ context.Context) error {
	if r.queries == nil {
		return nil
	}
	_ = r.queries.GetSessionLease
	_ = r.queries.UpsertSessionLease
	return nil
}

// publish 在持久化后发布进程内事件。
// TODO: 把已落库的 AgentEvent 发布到 Bus。
func (r *Runtime) publish(_ context.Context, _ pkgagent.AgentEvent) {
	if r.bus == nil {
		return
	}
	r.bus.Publish(events.Event{})
}
