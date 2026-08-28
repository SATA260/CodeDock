package agent

import (
	"context"

	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
)

// Runtime 负责 Agent 运行时编排和数据库持久化。
// Handler 负责对前端的 Start / Continue / Cancel / 审批；领取之后的主循环在这里。
type Runtime struct {
	queries *sqlite.Queries
	bus     *events.Bus
	worker  *Worker
}

// New 创建运行时及其 Worker。本阶段不绑定具体业务实现。
func New(queries *sqlite.Queries, bus *events.Bus) *Runtime {
	runtime := &Runtime{queries: queries, bus: bus}
	runtime.worker = NewWorker(runtime)
	return runtime
}

// Worker 返回领取 Run 的 Worker。
func (r *Runtime) Worker() *Worker {
	if r == nil {
		return nil
	}
	return r.worker
}

// Execute 执行一个已被 Worker 领取的 Run，直到完成、等待审批或终止。
//
// 入口不在这里。Handler 先写用户消息和 Run，再 Worker.Submit；Worker 用 goroutine 调 Execute。
//
// 一个 Session 同时只有一个 active Run。进入后先抢会话 lease，避免旧执行者继续写库。
//
// 每个 Turn 按同一条链路走一遍：
//
//  1. 读 Run / 消息 / 最近压缩 checkpoint
//  2. 状态 -> loading_context
//  3. pkg.Load 装摘要和 checkpoint 之后的消息
//  4. 超预算则 pkg.CompactIfNeeded，写新 checkpoint
//  5. pkg.Build 组装系统提示词、历史和工具定义
//  6. 状态 -> running_llm，pkg.Stream 拉模型流
//  7. Transition(bus, stream) 把 LLM 增量发到事件总线，再写助手消息和用量
//  8. 无 Tool Call：状态 -> completed，结束 Run
//  9. 有 Tool Call：状态 -> executing_tools，pkg.Dispatch
//     - 需要审批：状态 -> waiting_approval，保存恢复点后退出
//     - 工具跑完：结果进入下一 Turn，从步骤 1 再来
//  10. 取消、超时、超限或错误：进入 cancelled / failed，保留已落库内容
//
// 本阶段为空实现，下面只保留调用顺序。
func (r *Runtime) Execute(ctx context.Context, runID string) error {
	// 1. 领取：读 Run，占用 session lease，创建本轮 Turn。
	_ = runID
	_ = r.persistRun(ctx)
	_ = r.persistTurn(ctx)
	_ = r.persistLease(ctx)

	// 2. 装上下文。只用 Run 上已冻结的 Agent 配置。
	_ = r.PersistTransition(ctx, runID, pkgagent.RunLoadingContext, "")
	_, _ = pkgagent.Load(ctx, pkgagent.History{})

	// 3. 超预算则压缩。pkg 内部自己创建 Eino ToolCallingChatModel。
	_, _ = pkgagent.CompactIfNeeded(ctx, pkgagent.Compaction{})
	_ = r.persistCheckpoint(ctx)

	// 4. 组装模型调用，进入 running_llm。
	_ = r.PersistTransition(ctx, runID, pkgagent.RunRunningLLM, "")
	chat, _ := pkgagent.Build(ctx, pkgagent.Prompt{})

	// 5. 流式调用模型；pkg 内部创建 Eino ToolCallingChatModel。Transition 把增量发到事件总线。
	stream, _ := pkgagent.Stream(ctx, chat)
	_ = Transition(ctx, r.bus, stream)
	_ = r.persistMessage(ctx)
	_ = r.persistUsage(ctx)

	// 6. 有 Tool Call 则调度；需要审批时暂停，否则把工具结果留给下一 Turn。
	_ = r.PersistTransition(ctx, runID, pkgagent.RunExecutingTools, "")
	_, _ = tool.Dispatch(ctx, tool.Invocation{})

	// 7. 无下一 Turn 时结束。真实实现会在这里循环或因审批/取消退出。
	_ = r.PersistTransition(ctx, runID, pkgagent.RunCompleted, "")
	_ = r.persistEvent(ctx)
	r.publish(ctx, pkgagent.AgentEvent{})
	return nil
}
