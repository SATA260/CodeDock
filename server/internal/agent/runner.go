package agent

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	cderr "codedock/internal/errors"
	"codedock/internal/events"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

// Runtime 负责 Agent 运行时编排和数据库持久化。
type Runtime struct {
	db      db.Client
	queries *sqlite.Queries
	bus     *events.Bus
	worker  *Worker
	tools   tool.Registry
	log     *slog.Logger
}

// New 创建运行时及其 Worker。log 为 nil 时回退到 slog.Default。
func New(client db.Client, queries *sqlite.Queries, bus *events.Bus, tools tool.Registry, log *slog.Logger) *Runtime {
	if tools == nil {
		tools = tool.NewRegistry()
		_ = tools.Register(tool.Ping())
	}
	if log == nil {
		log = slog.Default()
	}
	runtime := &Runtime{db: client, queries: queries, bus: bus, tools: tools, log: log}
	runtime.worker = NewWorker(runtime)
	return runtime
}

// logger 返回运行时日志；Runtime 或字段为空时回退到 slog.Default。
func (r *Runtime) logger() *slog.Logger {
	if r == nil || r.log == nil {
		return slog.Default()
	}
	return r.log
}

// Worker 返回领取 Run 的 Worker。
func (r *Runtime) Worker() *Worker {
	if r == nil {
		return nil
	}
	return r.worker
}

// Tools 返回运行时工具注册中心。
func (r *Runtime) Tools() tool.Registry {
	if r == nil {
		return nil
	}
	return r.tools
}

// Start 启动 Worker 并恢复可继续的 Run。waiting_approval 不会自动恢复。
func (r *Runtime) Start(ctx context.Context) {
	if r.worker != nil {
		r.worker.Start(ctx)
	}
	if r.queries == nil {
		return
	}
	rows, err := r.q(ctx).ListRecoverableRuns(ctx)
	if err != nil {
		r.logger().Error("list recoverable runs failed", "error", err)
		return
	}
	r.logger().Info("recovering runs", "count", len(rows))
	for _, row := range rows {
		_ = r.worker.Submit(ctx, row.ID)
	}
}

// Execute 执行一个已被 Worker 领取的 Run，直到完成、等待审批或终止。
// 每轮先检查取消与上限；有工具 checkpoint 则跳过模型从 Dispatch 继续。
// 无 Tool Call 则 completed；需审批则写 checkpoint 后退出，否则进入下一 Turn。
func (r *Runtime) Execute(ctx context.Context, runID string) error {
	run, err := r.getRun(dbCtx(ctx), runID)
	if err != nil {
		return err
	}
	if pkgagent.IsTerminal(run.Status) {
		return nil
	}
	r.logger().Info("execute run", "session_id", run.SessionID, "run_id", run.ID, "status", run.Status)
	if err := r.acquireLease(dbCtx(ctx), run.SessionID, run.ID); err != nil {
		return err
	}
	defer r.releaseLease(dbCtx(ctx), run.SessionID)

	if run.CancelRequested || ctx.Err() != nil {
		return r.terminate(dbCtx(ctx), run, pkgagent.RunCancelled, pkgagent.StopCancelled, "cancelled")
	}
	var cancel context.CancelFunc
	if run.Config.Limits.MaxWallTime > 0 {
		deadline := run.Config.Limits.MaxWallTime
		if run.StartedAt != nil {
			remaining := time.Until(run.StartedAt.Add(run.Config.Limits.MaxWallTime))
			if remaining <= 0 {
				return r.terminate(ctx, run, pkgagent.RunFailed, pkgagent.StopTimeout, "timeout")
			}
			deadline = remaining
		}
		ctx, cancel = context.WithTimeout(ctx, deadline)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	go r.watchCancel(ctx, cancel, run.ID)

	checkpoint, hasCheckpoint, err := r.loadToolCheckpoint(ctx, run.ID)
	if err != nil {
		return r.fail(ctx, run, err, pkgagent.StopToolError)
	}

	for {
		run, err = r.getRun(ctx, run.ID)
		if err != nil {
			return r.stopFromErr(ctx, run, err)
		}
		if pkgagent.IsTerminal(run.Status) {
			return nil
		}
		if run.CancelRequested || ctx.Err() != nil {
			return r.terminate(ctx, run, pkgagent.RunCancelled, pkgagent.StopCancelled, "cancelled")
		}
		if exceeded, reason := wallExceeded(run); exceeded {
			return r.terminate(ctx, run, pkgagent.RunFailed, reason, string(reason))
		}

		turns, err := r.listTurns(ctx, run.ID)
		if err != nil {
			return r.fail(ctx, run, err, pkgagent.StopModelError)
		}
		resumeTools := hasCheckpoint && (run.Status == pkgagent.RunWaitingApproval || run.Status == pkgagent.RunExecutingTools || run.Status == pkgagent.RunQueued)
		if run.Status == pkgagent.RunWaitingApproval && !hasCheckpoint {
			return nil
		}

		turn, err := r.ensureTurn(ctx, run, turns, resumeTools)
		if err != nil {
			return r.fail(ctx, run, err, pkgagent.StopModelError)
		}
		if run, err = r.getRun(ctx, run.ID); err != nil {
			return r.stopFromErr(ctx, run, err)
		}
		if !resumeTools && run.Config.Limits.MaxTurns > 0 && turn.Number > run.Config.Limits.MaxTurns {
			return r.terminate(ctx, run, pkgagent.RunFailed, pkgagent.StopMaxTurns, "max_turns")
		}

		var calls []tool.Call
		if resumeTools {
			calls = checkpoint.Pending
			if err := r.transitionOrStop(ctx, run, pkgagent.RunExecutingTools, "resume tools"); err != nil {
				return err
			}
		} else {
			if err := r.runModelTurn(ctx, &run, &turn); err != nil {
				return r.stopFromErr(ctx, run, err)
			}
			run, _ = r.getRun(ctx, run.ID)
			turn, _ = r.getTurn(ctx, turn.ID)
			if turn.AssistantMsgID != nil {
				msg, err := r.getMessage(ctx, *turn.AssistantMsgID)
				if err != nil {
					return r.fail(ctx, run, err, pkgagent.StopModelError)
				}
				calls = msg.ToolCalls
			}
			if len(calls) == 0 {
				_ = r.completeTurn(ctx, run, turn)
				return r.terminate(ctx, run, pkgagent.RunCompleted, pkgagent.StopCompleted, "completed")
			}
			if err := r.PersistTransition(ctx, run.ID, pkgagent.RunExecutingTools, "dispatch tools"); err != nil {
				return r.fail(ctx, run, err, pkgagent.StopToolError)
			}
		}

		if limit := run.Config.Limits.MaxToolCalls; limit > 0 {
			used := countToolCalls(ctx, r, run.SessionID) + len(calls)
			if used > limit {
				return r.terminate(ctx, run, pkgagent.RunFailed, pkgagent.StopBudgetExceeded, "max_tool_calls")
			}
		}

		if resumeTools {
			for _, call := range calls {
				checkpoint.Completed = append(checkpoint.Completed, call.ID)
			}
		}
		paused, err := r.runTools(ctx, run, turn, calls, checkpoint)
		if err != nil {
			return r.stopFromErr(ctx, run, err)
		}
		if paused {
			return nil
		}
		hasCheckpoint = false
		checkpoint = toolCheckpoint{}
		r.deleteToolCheckpoint(ctx, run.ID)
		_ = r.completeTurn(ctx, run, turn)
	}
}

// runModelTurn 装载上下文、必要时压缩，再流式调用模型并落助手消息与用量。
func (r *Runtime) runModelTurn(ctx context.Context, run *pkgagent.Run, turn *pkgagent.Turn) error {
	if err := r.transitionOrStop(ctx, *run, pkgagent.RunLoadingContext, "load context"); err != nil {
		return err
	}
	snapshot, err := retryValue(ctx, run.Config.RetryPolicy.Context, func(int) (pkgagent.ContextSnapshot, error) {
		return r.loadSnapshot(ctx, *run, *turn)
	})
	if err != nil {
		return err
	}
	compacted, err := r.compactIfNeeded(ctx, *run, *turn, snapshot)
	if err != nil {
		return err
	}
	if compacted.Summary != nil && (snapshot.Summary == nil || snapshot.Summary.Content != compacted.Summary.Content || len(snapshot.Messages) != len(compacted.Messages)) {
		if err := r.persistCompaction(ctx, *run, compacted); err != nil {
			return err
		}
		snapshot, err = r.loadSnapshot(ctx, *run, *turn)
		if err != nil {
			return err
		}
	} else {
		snapshot = compacted
	}

	if err := r.transitionOrStop(ctx, *run, pkgagent.RunRunningLLM, "call model"); err != nil {
		return err
	}
	chat, err := pkgagent.Build(ctx, pkgagent.Prompt{Run: *run, Turn: *turn, Context: snapshot})
	if err != nil {
		return err
	}
	messageID := util.NewID()
	var result pkgagent.ModelStreamResult
	err = retryErr(ctx, run.Config.RetryPolicy.Model, func(attempt int) error {
		chat.Attempt = attempt
		stream, err := pkgagent.Stream(ctx, chat)
		if err != nil {
			return err
		}
		result, err = r.Transition(ctx, *run, *turn, messageID, stream)
		return err
	})
	if err != nil {
		return err
	}

	runID := run.ID
	turnID := turn.ID
	msg := result.Message
	msg.ID = messageID
	msg.SessionID = run.SessionID
	msg.RunID = &runID
	msg.TurnID = &turnID
	msg.Role = pkgagent.RoleAssistant
	msg.ToolCalls = result.ToolCalls
	completed, err := r.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		TurnID:    &turn.ID,
		Type:      pkgagent.EventAssistantCompleted,
		Payload: pkgagent.MarshalPayload(pkgagent.AssistantCompletedPayload{
			MessageID: messageID,
			Text:      pkgagent.DecodeText(msg.Content),
			ToolCalls: msg.ToolCalls,
		}),
	})
	if err != nil {
		return err
	}
	msg.EventSeq = completed.Seq
	saved, err := r.insertMessage(ctx, msg)
	if err != nil {
		return err
	}
	_ = saved

	usage := pkgagent.UsageRecord{
		SessionID:                run.SessionID,
		RunID:                    run.ID,
		TurnID:                   turn.ID,
		RequestID:                result.Usage.RequestID,
		Provider:                 result.Usage.Provider,
		Model:                    result.Usage.Model,
		UsageType:                "generation",
		CacheCreationInputTokens: result.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     result.Usage.CacheReadInputTokens,
		OutputTokens:             result.Usage.OutputTokens,
		ReasoningTokens:          result.Usage.ReasoningTokens,
		TotalTokens:              result.Usage.TotalTokens,
		Estimated:                result.Usage.Estimated,
		RawProviderUsage:         result.Usage.Raw,
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = pkgagent.CountTokens(pkgagent.DecodeText(msg.Content))
		usage.Estimated = true
	}
	if limit := run.Config.Limits.MaxOutputTokens; limit > 0 && usage.OutputTokens > limit {
		return errBudget
	}
	savedUsage, err := r.insertUsage(ctx, usage)
	if err != nil {
		return err
	}
	if _, err := r.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		TurnID:    &turn.ID,
		Type:      pkgagent.EventUsageRecorded,
		Payload: pkgagent.MarshalPayload(pkgagent.UsageRecordedPayload{
			UsageID:     savedUsage.ID,
			UsageType:   savedUsage.UsageType,
			TotalTokens: savedUsage.TotalTokens,
			Estimated:   savedUsage.Estimated,
		}),
	}); err != nil {
		return err
	}
	turn.AssistantMsgID = &saved.ID
	turn.UsageID = &savedUsage.ID
	now := util.Now()
	if turn.StartedAt == nil {
		turn.StartedAt = &now
	}
	return r.saveTurn(ctx, *turn)
}

// runTools 调度本轮工具调用。返回 true 表示已进入 waiting_approval，Execute 应退出。
func (r *Runtime) runTools(ctx context.Context, run pkgagent.Run, turn pkgagent.Turn, calls []tool.Call, previous toolCheckpoint) (bool, error) {
	approved := append([]string{}, previous.Completed...)
	inv := tool.Invocation{
		SessionID:        run.SessionID,
		RunID:            run.ID,
		TurnID:           turn.ID,
		Calls:            calls,
		Mode:             run.Config.ToolExecutionMode,
		FailurePolicy:    run.Config.ToolFailurePolicy,
		MaxParallel:      run.Config.Limits.MaxParallelTools,
		PermissionPolicy: run.Config.PermissionPolicy,
		ApprovalPolicy:   run.Config.ApprovalPolicy,
		AgentMode:        string(run.Mode),
		Registry:         r.tools,
		ApprovedCallIDs:  approved,
		OnEvent: func(kind string, call tool.Call, attempt int, result *tool.Result) {
			r.emitToolEvent(ctx, run, turn, kind, call, attempt, result, "")
		},
	}
	out, err := tool.Dispatch(ctx, inv)
	if err != nil && !out.WaitingApproval {
		return false, err
	}
	if out.WaitingApproval {
		for _, call := range out.PendingCalls {
			approval, aerr := r.insertApproval(ctx, pkgagent.Approval{
				SessionID:  run.SessionID,
				RunID:      run.ID,
				ToolCallID: call.ID,
				Scope:      pkgagent.ApprovalOnce,
				Status:     pkgagent.ApprovalPending,
				ExpiresAt:  util.Now().Add(run.Config.ApprovalPolicy.DefaultExpiry),
			})
			if aerr != nil {
				return false, aerr
			}
			r.emitToolEvent(ctx, run, turn, "approval_required", call, call.Attempt, nil, approval.ID)
		}
		completed := previous.Completed
		for _, result := range out.Results {
			if result.Success {
				completed = append(completed, result.CallID)
			}
		}
		if err := r.saveToolCheckpoint(ctx, run.ID, turn.ID, completed, out.PendingCalls, append(previous.Results, out.Results...)); err != nil {
			return false, err
		}
		if err := r.PersistTransition(ctx, run.ID, pkgagent.RunWaitingApproval, "approval required"); err != nil {
			return false, err
		}
		r.logger().Info("run waiting approval", "session_id", run.SessionID, "run_id", run.ID, "turn_id", turn.ID, "pending", len(out.PendingCalls))
		return true, nil
	}

	for _, result := range out.Results {
		content := pkgagent.EncodeToolResult(result.CallID, result.Output)
		if !result.Success {
			content = pkgagent.EncodeText(result.Error)
		}
		runID := run.ID
		turnID := turn.ID
		ev, err := r.AppendEvent(ctx, pkgagent.AgentEvent{
			SessionID: run.SessionID,
			RunID:     run.ID,
			TurnID:    &turn.ID,
			Type:      pkgagent.EventToolExecutionResult,
			Payload: pkgagent.MarshalPayload(pkgagent.ToolCallPayload{
				CallID:  result.CallID,
				Name:    result.Name,
				Success: boolPtr(result.Success),
				Error:   result.Error,
				Output:  result.Output,
			}),
		})
		if err != nil {
			return false, err
		}
		if _, err := r.insertMessage(ctx, pkgagent.Message{
			SessionID: run.SessionID,
			RunID:     &runID,
			TurnID:    &turnID,
			Role:      pkgagent.RoleTool,
			Content:   content,
			EventSeq:  ev.Seq,
		}); err != nil {
			return false, err
		}
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// emitToolEvent 把 Dispatch 过程事件落成 AgentEvent；审批与最终结果由调用方另写。
func (r *Runtime) emitToolEvent(ctx context.Context, run pkgagent.Run, turn pkgagent.Turn, kind string, call tool.Call, attempt int, result *tool.Result, approvalID string) {
	typ := pkgagent.EventToolCallStarted
	switch kind {
	case "approval_required":
		return
	case "execution_started":
		typ = pkgagent.EventToolExecutionStarted
	case "execution_retry":
		typ = pkgagent.EventToolExecutionRetry
	case "execution_result":
		return
	}
	payload := pkgagent.ToolCallPayload{
		CallID:     call.ID,
		Name:       call.Name,
		Arguments:  call.Arguments,
		Attempt:    attempt,
		ApprovalID: approvalID,
	}
	if result != nil {
		payload.Success = boolPtr(result.Success)
		payload.Error = result.Error
		payload.Output = result.Output
	}
	_, _ = r.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		TurnID:    &turn.ID,
		Type:      typ,
		Payload:   pkgagent.MarshalPayload(payload),
	})
}

// loadSnapshot 读取最新压缩 checkpoint 及其后的消息，再交给 pkg.Load。
func (r *Runtime) loadSnapshot(ctx context.Context, run pkgagent.Run, turn pkgagent.Turn) (pkgagent.ContextSnapshot, error) {
	var checkpoint *pkgagent.CompactionCheckpoint
	row, err := r.q(ctx).GetLatestCheckpoint(ctx, run.SessionID)
	if err == nil {
		mapped := mapCheckpoint(row)
		checkpoint = &mapped
	} else if !errors.Is(err, sql.ErrNoRows) {
		return pkgagent.ContextSnapshot{}, wrapDB(err)
	}
	after := int64(0)
	if checkpoint != nil {
		after = checkpoint.BaseEventSeq
	}
	rows, err := r.q(ctx).ListMessagesAfterSeq(ctx, sqlite.ListMessagesAfterSeqParams{
		SessionID: run.SessionID,
		EventSeq:  after,
	})
	if err != nil {
		return pkgagent.ContextSnapshot{}, wrapDB(err)
	}
	messages := make([]pkgagent.Message, 0, len(rows))
	for _, item := range rows {
		messages = append(messages, mapMessage(item))
	}
	return pkgagent.Load(ctx, pkgagent.History{
		Run:        run,
		Turn:       turn,
		Checkpoint: checkpoint,
		Messages:   messages,
		Tools:      tool.Definitions(r.tools),
		Prompt:     run.Config.Profile.Prompt.Inline,
	})
}

// compactIfNeeded 按 Context 重试策略调用 pkg.CompactIfNeeded。
func (r *Runtime) compactIfNeeded(ctx context.Context, run pkgagent.Run, turn pkgagent.Turn, snapshot pkgagent.ContextSnapshot) (pkgagent.ContextSnapshot, error) {
	var out pkgagent.ContextSnapshot
	err := retryErr(ctx, run.Config.RetryPolicy.Context, func(int) error {
		var err error
		out, err = pkgagent.CompactIfNeeded(ctx, pkgagent.Compaction{Run: run, Turn: turn, Snapshot: snapshot})
		return err
	})
	return out, err
}

// persistCompaction 写入压缩 checkpoint、会话 compaction_seq、用量和 context.compacted。
func (r *Runtime) persistCompaction(ctx context.Context, run pkgagent.Run, snapshot pkgagent.ContextSnapshot) error {
	if snapshot.Summary == nil {
		return nil
	}
	row, err := r.q(ctx).InsertCompactionCheckpoint(ctx, sqlite.InsertCompactionCheckpointParams{
		ID:           util.NewID(),
		SessionID:    run.SessionID,
		BaseEventSeq: snapshot.BaseEventSeq,
		Summary:      snapshot.Summary.Content,
		CreatedByRun: run.ID,
		CreatedAt:    util.FormatTime(util.Now()),
	})
	if err != nil {
		return wrapDB(err)
	}
	_ = r.q(ctx).UpdateCompactionSeq(ctx, sqlite.UpdateCompactionSeqParams{
		CompactionSeq: snapshot.BaseEventSeq,
		UpdatedAt:     util.FormatTime(util.Now()),
		ID:            run.SessionID,
	})
	usage, err := r.insertUsage(ctx, pkgagent.UsageRecord{
		SessionID:   run.SessionID,
		RunID:       run.ID,
		TurnID:      deref(run.CurrentTurnID),
		RequestID:   row.ID,
		Provider:    run.Config.Model.Provider,
		Model:       run.Config.Model.Model,
		UsageType:   "compaction",
		TotalTokens: pkgagent.CountTokens(snapshot.Summary.Content),
		Estimated:   true,
	})
	if err != nil {
		return err
	}
	_, err = r.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		TurnID:    run.CurrentTurnID,
		Type:      pkgagent.EventContextCompacted,
		Payload: pkgagent.MarshalPayload(pkgagent.ContextCompactedPayload{
			CheckpointID: row.ID,
			BaseEventSeq: row.BaseEventSeq,
		}),
	})
	_ = usage
	if err == nil {
		r.logger().Info("context compacted", "session_id", run.SessionID, "run_id", run.ID, "checkpoint_id", row.ID, "base_event_seq", row.BaseEventSeq)
	}
	return err
}

// ensureTurn 恢复进行中的 Turn，或新建下一 Turn 并写 turn.started。
func (r *Runtime) ensureTurn(ctx context.Context, run pkgagent.Run, turns []pkgagent.Turn, resume bool) (pkgagent.Turn, error) {
	if resume && run.CurrentTurnID != nil {
		return r.getTurn(ctx, *run.CurrentTurnID)
	}
	if n := len(turns); n > 0 {
		last := turns[n-1]
		if last.Status == pkgagent.TurnPending || last.Status == pkgagent.TurnRunning || last.Status == pkgagent.TurnWaitingApproval {
			return last, nil
		}
	}
	now := util.Now()
	number := int64(len(turns) + 1)
	row, err := r.q(ctx).InsertTurn(ctx, sqlite.InsertTurnParams{
		ID:            util.NewID(),
		RunID:         run.ID,
		Number:        number,
		Status:        string(pkgagent.TurnRunning),
		FirstEventSeq: 0,
		LastEventSeq:  0,
		StartedAt:     nullTime(&now),
	})
	if err != nil {
		return pkgagent.Turn{}, wrapDB(err)
	}
	turn := mapTurn(row)
	run.CurrentTurnID = &turn.ID
	if err := r.saveRun(ctx, run); err != nil {
		return pkgagent.Turn{}, err
	}
	if _, err := r.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		TurnID:    &turn.ID,
		Type:      pkgagent.EventTurnStarted,
		Payload:   pkgagent.MarshalPayload(pkgagent.TurnStartedPayload{Number: turn.Number}),
	}); err != nil {
		return pkgagent.Turn{}, err
	}
	return turn, nil
}

// completeTurn 把 Turn 标为完成并写 turn.completed。
func (r *Runtime) completeTurn(ctx context.Context, run pkgagent.Run, turn pkgagent.Turn) error {
	now := util.Now()
	turn.Status = pkgagent.TurnCompleted
	turn.FinishedAt = &now
	if err := r.saveTurn(ctx, turn); err != nil {
		return err
	}
	_, err := r.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		TurnID:    &turn.ID,
		Type:      pkgagent.EventTurnCompleted,
		Payload:   pkgagent.MarshalPayload(pkgagent.TurnCompletedPayload{Number: turn.Number, Status: turn.Status}),
	})
	return err
}

// Terminate 将 Run 置为终态。
func (r *Runtime) Terminate(ctx context.Context, runID string, status pkgagent.RunStatus, reason pkgagent.StopReason, message string) error {
	run, err := r.getRun(ctx, runID)
	if err != nil {
		return err
	}
	return r.terminate(ctx, run, status, reason, message)
}

// terminate 写入 StopReason 并迁移到终态；取消时先经过 cancelling。
func (r *Runtime) terminate(ctx context.Context, run pkgagent.Run, status pkgagent.RunStatus, reason pkgagent.StopReason, message string) error {
	current, err := r.getRun(ctx, run.ID)
	if err == nil {
		run = current
	}
	if pkgagent.IsTerminal(run.Status) {
		return nil
	}
	r.logger().Warn("run terminating", "session_id", run.SessionID, "run_id", run.ID, "status", status, "stop_reason", reason, "message", message)
	run.StopReason = &reason
	if run.CancelRequested && status == pkgagent.RunCancelled {
		run.CancelRequested = true
	}
	if err := r.saveRun(ctx, run); err != nil {
		return err
	}
	if run.Status != pkgagent.RunCancelling && status == pkgagent.RunCancelled {
		_ = r.PersistTransition(ctx, run.ID, pkgagent.RunCancelling, message)
		run.Status = pkgagent.RunCancelling
		_ = r.saveRun(ctx, run)
	}
	if err := r.PersistTransition(ctx, run.ID, status, message); err != nil && !isConflict(err) {
		return err
	}
	return nil
}

// watchCancel 轮询 cancel_requested，置位后取消 Execute 的 context。
func (r *Runtime) watchCancel(ctx context.Context, cancel context.CancelFunc, runID string) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, err := r.getRun(ctx, runID)
			if err != nil {
				continue
			}
			if run.CancelRequested {
				cancel()
				return
			}
		}
	}
}

// transitionOrStop 尝试状态迁移；冲突时若已取消或终态则按错误停跑。
func (r *Runtime) transitionOrStop(ctx context.Context, run pkgagent.Run, next pkgagent.RunStatus, reason string) error {
	if err := r.PersistTransition(ctx, run.ID, next, reason); err == nil {
		return nil
	} else if !isConflict(err) {
		return err
	}
	current, loadErr := r.getRun(dbCtx(ctx), run.ID)
	if loadErr != nil {
		return loadErr
	}
	if current.CancelRequested || current.Status == pkgagent.RunCancelling || pkgagent.IsTerminal(current.Status) {
		return r.stopFromErr(ctx, current, context.Canceled)
	}
	return nil
}

// dbCtx 在原 ctx 已取消时改用 Background，避免终态写库失败。
func dbCtx(ctx context.Context) context.Context {
	if ctx == nil || ctx.Err() != nil {
		return context.Background()
	}
	return ctx
}

// stopFromErr 按取消、超时或预算把 Run 落到对应终态，并返回原错误。
func (r *Runtime) stopFromErr(ctx context.Context, run pkgagent.Run, err error) error {
	ctx = dbCtx(ctx)
	current, loadErr := r.getRun(ctx, run.ID)
	if loadErr == nil {
		run = current
	}
	if run.CancelRequested || errors.Is(err, context.Canceled) {
		r.logger().Warn("run stopped", "session_id", run.SessionID, "run_id", run.ID, "stop_reason", pkgagent.StopCancelled, "error", err)
		return r.terminate(ctx, run, pkgagent.RunCancelled, pkgagent.StopCancelled, "cancelled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		r.logger().Warn("run stopped", "session_id", run.SessionID, "run_id", run.ID, "stop_reason", pkgagent.StopTimeout, "error", err)
		return r.terminate(ctx, run, pkgagent.RunFailed, pkgagent.StopTimeout, "timeout")
	}
	reason := pkgagent.StopModelError
	if errors.Is(err, errBudget) {
		reason = pkgagent.StopBudgetExceeded
	}
	return r.fail(ctx, run, err, reason)
}

// fail 把 Run 标为 failed 后原样返回错误。
func (r *Runtime) fail(ctx context.Context, run pkgagent.Run, err error, reason pkgagent.StopReason) error {
	r.logger().Error("run failed", "session_id", run.SessionID, "run_id", run.ID, "stop_reason", reason, "error", err)
	_ = r.terminate(ctx, run, pkgagent.RunFailed, reason, err.Error())
	return err
}

// GetRun 读取 Run。
func (r *Runtime) GetRun(ctx context.Context, id string) (pkgagent.Run, error) {
	return r.getRun(ctx, id)
}

// getRun 从数据库读取并映射 Run。
func (r *Runtime) getRun(ctx context.Context, id string) (pkgagent.Run, error) {
	row, err := r.q(ctx).GetRun(ctx, id)
	if err != nil {
		return pkgagent.Run{}, wrapDB(err)
	}
	return mapRun(row), nil
}

// getTurn 从数据库读取并映射 Turn。
func (r *Runtime) getTurn(ctx context.Context, id string) (pkgagent.Turn, error) {
	row, err := r.q(ctx).GetTurn(ctx, id)
	if err != nil {
		return pkgagent.Turn{}, wrapDB(err)
	}
	return mapTurn(row), nil
}

// getMessage 从数据库读取并映射消息。
func (r *Runtime) getMessage(ctx context.Context, id string) (pkgagent.Message, error) {
	row, err := r.q(ctx).GetMessage(ctx, id)
	if err != nil {
		return pkgagent.Message{}, wrapDB(err)
	}
	return mapMessage(row), nil
}

// listTurns 列出某 Run 的全部 Turn。
func (r *Runtime) listTurns(ctx context.Context, runID string) ([]pkgagent.Turn, error) {
	rows, err := r.q(ctx).ListRunTurns(ctx, runID)
	if err != nil {
		return nil, wrapDB(err)
	}
	out := make([]pkgagent.Turn, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapTurn(row))
	}
	return out, nil
}

// retryErr 按 RetryConfig 退避重试 fn，直到成功、不可重试或达上限。
func retryErr(ctx context.Context, cfg pkgagent.RetryConfig, fn func(attempt int) error) error {
	max := cfg.MaxAttempts
	if max <= 0 {
		max = 1
	}
	var err error
	for attempt := 1; attempt <= max; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn(attempt)
		if err == nil {
			return nil
		}
		if !pkgagent.ShouldRetry(cfg, attempt, err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pkgagent.Backoff(cfg, attempt)):
		}
	}
	return err
}

// retryValue 与 retryErr 相同，但带回成功时的返回值。
func retryValue[T any](ctx context.Context, cfg pkgagent.RetryConfig, fn func(attempt int) (T, error)) (T, error) {
	var zero T
	var out T
	err := retryErr(ctx, cfg, func(attempt int) error {
		var err error
		out, err = fn(attempt)
		return err
	})
	if err != nil {
		return zero, err
	}
	return out, nil
}

// wallExceeded 判断 Run 是否已超过墙钟上限。
func wallExceeded(run pkgagent.Run) (bool, pkgagent.StopReason) {
	if run.Config.Limits.MaxWallTime <= 0 || run.StartedAt == nil {
		return false, ""
	}
	if util.Now().After(run.StartedAt.Add(run.Config.Limits.MaxWallTime)) {
		return true, pkgagent.StopTimeout
	}
	return false, ""
}

// countToolCalls 统计会话中已落库的工具结果消息数。
func countToolCalls(ctx context.Context, r *Runtime, sessionID string) int {
	rows, err := r.q(ctx).ListSessionMessages(ctx, sessionID)
	if err != nil {
		return 0
	}
	n := 0
	for _, row := range rows {
		if row.Role == string(pkgagent.RoleTool) {
			n++
		}
	}
	return n
}

// isConflict 判断错误是否为状态冲突。
func isConflict(err error) bool {
	return cderr.IsConflict(err)
}

// boolPtr 返回布尔值的指针，供事件载荷使用。
func boolPtr(v bool) *bool { return &v }

var errBudget = errors.New("budget exceeded")
