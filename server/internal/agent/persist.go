package agent

import (
	"context"
	"database/sql"
	"errors"
	"time"

	cderr "codedock/internal/errors"
	"codedock/internal/events"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

// q 返回当前上下文可用的 Queries；事务内自动切到 WithTx。
func (r *Runtime) q(ctx context.Context) *sqlite.Queries {
	if r.queries == nil {
		return nil
	}
	if tx, ok := db.TxFromContext(ctx); ok {
		return r.queries.WithTx(tx)
	}
	return r.queries
}

// wrapDB 把 sql.ErrNoRows 转成 NotFound。
func wrapDB(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return cderr.NotFound("%s", err.Error())
	}
	return err
}

// AppendEvent 写入一条 Agent 事件；若不在事务中则自行开事务并在提交后发布。
func (r *Runtime) AppendEvent(ctx context.Context, ev pkgagent.AgentEvent) (pkgagent.AgentEvent, error) {
	ctx = dbCtx(ctx)
	if _, inTx := db.TxFromContext(ctx); inTx {
		return r.persistEventOnly(ctx, ev)
	}
	var out pkgagent.AgentEvent
	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = r.persistEventOnly(ctx, ev)
		return err
	})
	if err != nil {
		return pkgagent.AgentEvent{}, err
	}
	r.Publish(out)
	return out, nil
}

// persistEventOnly 递增 last_event_seq 并插入 AgentEvent，不发布到 Bus。
func (r *Runtime) persistEventOnly(ctx context.Context, ev pkgagent.AgentEvent) (pkgagent.AgentEvent, error) {
	if ev.EventID == "" {
		ev.EventID = util.NewID()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = util.Now()
	}
	if ev.Version == 0 {
		ev.Version = 1
	}
	if len(ev.Payload) == 0 {
		ev.Payload = []byte("{}")
	}
	seq, err := r.q(ctx).IncrementEventSeq(ctx, sqlite.IncrementEventSeqParams{
		UpdatedAt: util.FormatTime(util.Now()),
		ID:        ev.SessionID,
	})
	if err != nil {
		return pkgagent.AgentEvent{}, wrapDB(err)
	}
	ev.Seq = seq
	row, err := r.q(ctx).InsertAgentEvent(ctx, sqlite.InsertAgentEventParams{
		EventID:    ev.EventID,
		SessionID:  ev.SessionID,
		RunID:      ev.RunID,
		TurnID:     nullString(deref(ev.TurnID)),
		Seq:        ev.Seq,
		Type:       string(ev.Type),
		Version:    int64(ev.Version),
		OccurredAt: util.FormatTime(ev.OccurredAt),
		Payload:    string(ev.Payload),
	})
	if err != nil {
		return pkgagent.AgentEvent{}, wrapDB(err)
	}
	return mapEvent(row), nil
}

// Publish 把已落库的 AgentEvent 发到进程内总线。
func (r *Runtime) Publish(ev pkgagent.AgentEvent) {
	if r.bus == nil {
		return
	}
	r.bus.Publish(events.Event{
		Type:          string(ev.Type),
		ChatSessionID: ev.SessionID,
		Payload:       ev,
	})
}

// publishAll 按顺序把已落库事件发到 Bus。
func (r *Runtime) publishAll(events []pkgagent.AgentEvent) {
	for _, ev := range events {
		r.Publish(ev)
	}
}

// PersistTransition 校验状态机后更新 Run，并先落库再发布状态事件。
// 终态额外写 run.completed / failed / cancelled。
func (r *Runtime) PersistTransition(ctx context.Context, runID string, next pkgagent.RunStatus, reason string) error {
	ctx = dbCtx(ctx)
	var published []pkgagent.AgentEvent
	var from pkgagent.RunStatus
	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		row, err := r.q(ctx).GetRun(ctx, runID)
		if err != nil {
			return wrapDB(err)
		}
		run := mapRun(row)
		if err := pkgagent.CanTransition(run.Status, next); err != nil {
			return cderr.Conflict("%s", err.Error())
		}
		from = run.Status
		now := util.Now()
		run.Status = next
		if next == pkgagent.RunLoadingContext && run.StartedAt == nil {
			run.StartedAt = &now
		}
		if pkgagent.IsTerminal(next) {
			run.FinishedAt = &now
		}
		if _, err := r.q(ctx).UpdateRun(ctx, runUpdateParams(run)); err != nil {
			return wrapDB(err)
		}
		state, err := r.persistEventOnly(ctx, pkgagent.AgentEvent{
			SessionID: run.SessionID,
			RunID:     run.ID,
			TurnID:    run.CurrentTurnID,
			Type:      pkgagent.EventRunStateChanged,
			Payload: pkgagent.MarshalPayload(pkgagent.RunStateChangedPayload{
				From:   from,
				To:     next,
				Reason: reason,
			}),
		})
		if err != nil {
			return err
		}
		published = append(published, state)
		if pkgagent.IsTerminal(next) {
			terminal, err := r.persistEventOnly(ctx, pkgagent.AgentEvent{
				SessionID: run.SessionID,
				RunID:     run.ID,
				TurnID:    run.CurrentTurnID,
				Type:      pkgagent.TerminalEvent(next),
				Payload: pkgagent.MarshalPayload(pkgagent.RunTerminalPayload{
					Status:     next,
					StopReason: run.StopReason,
				}),
			})
			if err != nil {
				return err
			}
			published = append(published, terminal)
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.publishAll(published)
	r.logger().Info("run state changed", "run_id", runID, "from", from, "to", next, "reason", reason)
	if pkgagent.IsTerminal(next) {
		r.logger().Info("run reached terminal", "run_id", runID, "status", next, "reason", reason)
	}
	return nil
}

// saveRun 把领域 Run 写回数据库。
func (r *Runtime) saveRun(ctx context.Context, run pkgagent.Run) error {
	_, err := r.q(ctx).UpdateRun(ctx, runUpdateParams(run))
	return wrapDB(err)
}

// saveTurn 把领域 Turn 写回数据库。
func (r *Runtime) saveTurn(ctx context.Context, turn pkgagent.Turn) error {
	_, err := r.q(ctx).UpdateTurn(ctx, turnUpdateParams(turn))
	return wrapDB(err)
}

// insertMessage 插入一条消息并回填生成字段。
func (r *Runtime) insertMessage(ctx context.Context, msg pkgagent.Message) (pkgagent.Message, error) {
	if msg.ID == "" {
		msg.ID = util.NewID()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = util.Now()
	}
	if len(msg.Content) == 0 {
		msg.Content = []byte("{}")
	}
	row, err := r.q(ctx).InsertMessage(ctx, sqlite.InsertMessageParams{
		ID:          msg.ID,
		SessionID:   msg.SessionID,
		RunID:       nullString(deref(msg.RunID)),
		TurnID:      nullString(deref(msg.TurnID)),
		Role:        string(msg.Role),
		Content:     string(msg.Content),
		Attachments: nullString(marshalJSON(msg.Attachments)),
		ToolCalls:   nullString(marshalJSON(msg.ToolCalls)),
		EventSeq:    msg.EventSeq,
		CreatedAt:   util.FormatTime(msg.CreatedAt),
	})
	if err != nil {
		return pkgagent.Message{}, wrapDB(err)
	}
	saved := mapMessage(row)
	if session, serr := r.getSession(ctx, saved.SessionID); serr == nil {
		r.indexPersistedMessage(ctx, session, saved)
	}
	return saved, nil
}

// insertUsage 插入一条用量记录。
func (r *Runtime) insertUsage(ctx context.Context, rec pkgagent.UsageRecord) (pkgagent.UsageRecord, error) {
	if rec.ID == "" {
		rec.ID = util.NewID()
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = util.Now()
	}
	estimated := int64(0)
	if rec.Estimated {
		estimated = 1
	}
	row, err := r.q(ctx).InsertUsageRecord(ctx, sqlite.InsertUsageRecordParams{
		ID:                       rec.ID,
		SessionID:                rec.SessionID,
		RunID:                    rec.RunID,
		TurnID:                   rec.TurnID,
		RequestID:                rec.RequestID,
		Provider:                 rec.Provider,
		Model:                    rec.Model,
		UsageType:                rec.UsageType,
		CacheCreationInputTokens: rec.CacheCreationInputTokens,
		CacheReadInputTokens:     rec.CacheReadInputTokens,
		OutputTokens:             rec.OutputTokens,
		ReasoningTokens:          rec.ReasoningTokens,
		TotalTokens:              rec.TotalTokens,
		Estimated:                estimated,
		RawProviderUsage:         nullString(string(rec.RawProviderUsage)),
		CreatedAt:                util.FormatTime(rec.CreatedAt),
	})
	if err != nil {
		return pkgagent.UsageRecord{}, wrapDB(err)
	}
	return mapUsage(row), nil
}

// insertApproval 插入一条待处理审批。
func (r *Runtime) insertApproval(ctx context.Context, item pkgagent.Approval) (pkgagent.Approval, error) {
	if item.ID == "" {
		item.ID = util.NewID()
	}
	firstID := item.ToolCallID
	if firstID == "" && len(item.ToolCalls) > 0 {
		firstID = item.ToolCalls[0].ID
	}
	row, err := r.q(ctx).InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID:         item.ID,
		SessionID:  item.SessionID,
		RunID:      item.RunID,
		ToolCallID: firstID,
		ToolCalls:  marshalJSON(item.ToolCalls),
		Scope:      string(item.Scope),
		Status:     string(item.Status),
		ExpiresAt:  util.FormatTime(item.ExpiresAt),
	})
	if err != nil {
		return pkgagent.Approval{}, wrapDB(err)
	}
	return mapApproval(row), nil
}

// acquireLease 为 Session 抢占执行租约。
func (r *Runtime) acquireLease(ctx context.Context, sessionID, runID string) error {
	now := util.Now()
	_, err := r.q(ctx).UpsertSessionLease(ctx, sqlite.UpsertSessionLeaseParams{
		SessionID:    sessionID,
		RunID:        runID,
		Owner:        util.NewID(),
		FencingToken: now.UnixNano(),
		HeartbeatAt:  util.FormatTime(now),
		ExpiresAt:    util.FormatTime(now.Add(time.Hour)),
	})
	return wrapDB(err)
}

// releaseLease 释放 Session 上的执行租约。
func (r *Runtime) releaseLease(ctx context.Context, sessionID string) {
	_ = r.q(ctx).DeleteSessionLease(ctx, sessionID)
}

// clearActive 在 active_run_id 仍指向该 Run 时清空它。
func (r *Runtime) clearActive(ctx context.Context, sessionID, runID string) error {
	return wrapDB(r.q(ctx).ClearActiveRun(ctx, sqlite.ClearActiveRunParams{
		UpdatedAt:   util.FormatTime(util.Now()),
		ID:          sessionID,
		ActiveRunID: nullString(runID),
	}))
}

// saveToolCheckpoint 保存本 Turn 已完成与待执行的工具调用，供审批恢复。
func (r *Runtime) saveToolCheckpoint(ctx context.Context, cp toolCheckpoint) error {
	_, err := r.q(ctx).UpsertRunToolCheckpoint(ctx, sqlite.UpsertRunToolCheckpointParams{
		RunID:          cp.RunID,
		TurnID:         cp.TurnID,
		CompletedCalls: marshalJSON(cp.Completed),
		PendingCalls:   marshalJSON(cp.Pending),
		Results:        marshalJSON(cp.Results),
		ApprovedCalls:  marshalJSON(cp.Approved),
		DeniedCalls:    marshalJSON(cp.Denied),
		UpdatedAt:      util.FormatTime(util.Now()),
	})
	return wrapDB(err)
}

type toolCheckpoint struct {
	RunID     string
	TurnID    string
	Completed []string
	Approved  []string
	Denied    []string
	Pending   []tool.Call
	Results   []tool.Result
}

// HasRecordedToolDecisions 表示 checkpoint 已写入批准或拒绝，审完待领取。
func (r *Runtime) HasRecordedToolDecisions(ctx context.Context, runID string) (bool, error) {
	cp, ok, err := r.loadToolCheckpoint(ctx, runID)
	if err != nil || !ok {
		return false, err
	}
	return len(cp.Approved) > 0 || len(cp.Denied) > 0, nil
}

// RecordToolDecisions 把一批审批裁决写入 checkpoint，恢复时不再猜测。
func (r *Runtime) RecordToolDecisions(ctx context.Context, runID string, approved, denied []string) error {
	cp, ok, err := r.loadToolCheckpoint(ctx, runID)
	if err != nil {
		return err
	}
	if !ok {
		return cderr.NotFound("tool checkpoint not found")
	}
	cp.Approved = approved
	cp.Denied = denied
	return r.saveToolCheckpoint(ctx, cp)
}

// loadToolCheckpoint 读取审批恢复用的工具 checkpoint；不存在时 ok=false。
func (r *Runtime) loadToolCheckpoint(ctx context.Context, runID string) (toolCheckpoint, bool, error) {
	row, err := r.q(ctx).GetRunToolCheckpoint(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return toolCheckpoint{}, false, nil
		}
		return toolCheckpoint{}, false, wrapDB(err)
	}
	var out toolCheckpoint
	out.RunID = row.RunID
	out.TurnID = row.TurnID
	unmarshalJSON(row.CompletedCalls, &out.Completed)
	unmarshalJSON(row.PendingCalls, &out.Pending)
	unmarshalJSON(row.Results, &out.Results)
	unmarshalJSON(row.ApprovedCalls, &out.Approved)
	unmarshalJSON(row.DeniedCalls, &out.Denied)
	return out, true, nil
}

// deleteToolCheckpoint 删除该 Run 的工具 checkpoint。
func (r *Runtime) deleteToolCheckpoint(ctx context.Context, runID string) {
	_ = r.q(ctx).DeleteRunToolCheckpoint(ctx, runID)
}
