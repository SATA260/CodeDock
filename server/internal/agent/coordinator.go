package agent

import (
	"context"

	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
)

// CreateAgentState 创建一次 Agent 执行的初始状态（只生成 ID，不写入数据库）。
// TODO：后续写入 queued 状态 Run 并发布 run.created 事件。
func (r *Runtime) CreateAgentState(_ context.Context, sessionID, triggerMessageID string, mode pkgagent.AgentMode, config pkgagent.RunConfigSnapshot) (string, error) {
	_ = sessionID
	_ = triggerMessageID
	_ = mode
	_ = config
	return util.NewID(), nil
}

// ClaimSession 将当前 Run 标记为会话的 active Run。
// TODO：写入 sessions.active_run_id。
func (r *Runtime) ClaimSession(_ context.Context, sessionID, runID string) error {
	_ = sessionID
	_ = runID
	return nil
}

// Enqueue 把 StepJob 投递给 Worker。
func (r *Runtime) Enqueue(ctx context.Context, job pkgagent.StepJob) error {
	if r == nil || r.worker == nil {
		return nil
	}
	return r.worker.Submit(ctx, job)
}

// TryClaimStep 互斥领取指定 Run 的指定步骤，防止多个 Worker 重复执行。
// TODO：实现步骤级锁，当前恒返回 true 以便骨架跑通。
func (r *Runtime) TryClaimStep(_ context.Context, runID string, stepIndex int) (bool, error) {
	_ = runID
	_ = stepIndex
	return true, nil
}

// LoadAgentState 从数据库加载 Run 与 checkpoint，拼出当前 AgentState。
// TODO：从库读取 Run 与 checkpoint。
func (r *Runtime) LoadAgentState(_ context.Context, runID string) (pkgagent.AgentState, error) {
	return pkgagent.AgentState{RunID: runID}, nil
}

// AppendFact 在步骤内写入一条事实并发布事件。
// TODO：递增 sessions.last_event_seq，写入 AgentEvent，提交后发布到事件总线。
func (r *Runtime) AppendFact(_ context.Context, runID string, fact pkgagent.Fact) (pkgagent.AgentEvent, error) {
	_ = runID
	_ = fact
	return pkgagent.AgentEvent{}, nil
}

// CommitStep 提交一步结果：校验状态与步骤序号，持久化 Run / Turn / Message / checkpoint，
// 并在非终态时把下一步作业重新入队。
func (r *Runtime) CommitStep(ctx context.Context, runID string, result pkgagent.StepResult) error {
	_ = runID
	if result.Next != nil {
		return r.Enqueue(ctx, *result.Next)
	}
	return nil
}

// RequestCancel 标记用户已请求取消本次 Run。
// TODO：写入 runs.cancel_requested。
func (r *Runtime) RequestCancel(_ context.Context, runID string) error {
	_ = runID
	return nil
}

// RecoverActive 启动时恢复非终态且已裁决的 StepJob，重新入队。
// TODO：扫描非终态 Run 并补投 StepJob。
func (r *Runtime) RecoverActive(_ context.Context) error {
	return nil
}

// DequeueNext 当前 Run 结束后，唤醒该会话下一个排队的 Run。
// TODO：清 active_run_id 并 Enqueue 下一条 queued Run。
func (r *Runtime) DequeueNext(_ context.Context, sessionID, finishedRunID string) error {
	_ = sessionID
	_ = finishedRunID
	return nil
}
