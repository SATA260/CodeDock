package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	cderr "codedock/internal/errors"
	"codedock/internal/events"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

const sessionSummaryMaxRunes = 200

// CreateAgentState 写入用户消息与 queued Run，并发布 run.created。
// content 是用户正文，不是已有 message id。
// 逻辑：事务内写 message/run/首条事件，可选填 Session 摘要 → 提交后 publish 并索引触发消息。
func (r *Runtime) CreateAgentState(ctx context.Context, sessionID, content string, mode pkgagent.AgentMode, config pkgagent.RunConfigSnapshot) (string, error) {
	if r == nil || r.db == nil {
		return "", cderr.Invalid("runtime is not initialized")
	}
	if sessionID == "" {
		return "", cderr.Invalid("session_id is required")
	}
	if strings.TrimSpace(content) == "" {
		return "", cderr.Invalid("content is required")
	}
	if mode == "" {
		mode = config.Mode
	}
	if mode == "" {
		mode = pkgagent.ModeAskForApproval
	}
	config.Mode = mode
	if config.Profile.Mode == "" {
		config.Profile.Mode = string(mode)
	}

	runID := util.NewID()
	msgID := util.NewID()
	now := util.Now()
	nowStr := util.FormatTime(now)
	var created pkgagent.AgentEvent

	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		q := r.q(ctx)
		sess, err := q.GetSession(ctx, sessionID)
		if err != nil {
			return wrapDB(err)
		}
		seq, err := q.IncrementEventSeq(ctx, sqlite.IncrementEventSeqParams{UpdatedAt: nowStr, ID: sessionID})
		if err != nil {
			return err
		}
		if _, err := q.InsertMessage(ctx, sqlite.InsertMessageParams{
			ID:        msgID,
			SessionID: sessionID,
			RunID:     nullString(runID),
			Role:      string(pkgagent.RoleUser),
			Content:   string(pkgagent.EncodeText(content)),
			EventSeq:  seq,
			CreatedAt: nowStr,
		}); err != nil {
			return err
		}
		cfg, err := json.Marshal(config)
		if err != nil {
			return err
		}
		if _, err := q.InsertRun(ctx, sqlite.InsertRunParams{
			ID:               runID,
			SessionID:        sessionID,
			TriggerMessageID: msgID,
			Mode:             string(mode),
			Config:           string(cfg),
			Status:           string(pkgagent.RunQueued),
			CancelRequested:  0,
		}); err != nil {
			return err
		}
		if sess.Summary == "" {
			if summary := clipSessionSummary(content); summary != "" {
				_ = q.SetSessionSummary(ctx, sqlite.SetSessionSummaryParams{
					Summary:   summary,
					UpdatedAt: nowStr,
					ID:        sessionID,
				})
			}
		}
		created, err = r.insertEventTx(ctx, sess.ID, runID, "", pkgagent.Fact{
			Type: pkgagent.EventRunCreated,
			Payload: pkgagent.MarshalPayload(pkgagent.RunCreatedPayload{
				TriggerMessageID: msgID,
				Mode:             mode,
				Status:           pkgagent.RunQueued,
				Text:             content,
				Config:           config,
			}),
		})
		return err
	})
	if err != nil {
		return "", err
	}
	r.publish(created)
	r.indexPersistedMessage(ctx, runID)
	return runID, nil
}

// ClaimSession 将当前 Run 标为会话的 active Run。已有其他 active 时不抢，返回 claimed=false。
func (r *Runtime) ClaimSession(ctx context.Context, sessionID, runID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, cderr.Invalid("runtime is not initialized")
	}
	if sessionID == "" || runID == "" {
		return false, cderr.Invalid("session_id and run_id are required")
	}
	sess, err := r.q(ctx).GetSession(ctx, sessionID)
	if err != nil {
		return false, wrapDB(err)
	}
	if sess.ActiveRunID.Valid && sess.ActiveRunID.String != "" {
		return sess.ActiveRunID.String == runID, nil
	}
	_, err = r.q(ctx).ClaimActiveRun(ctx, sqlite.ClaimActiveRunParams{
		ActiveRunID: nullString(runID),
		UpdatedAt:   util.FormatTime(util.Now()),
		ID:          sessionID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Enqueue 先把 StepJob 落成 queued，再投递给 Worker。StepIndex 为 0 时按已提交步骤 + 1 补齐。
func (r *Runtime) Enqueue(ctx context.Context, job pkgagent.StepJob) error {
	if r == nil {
		return nil
	}
	if job.StepIndex <= 0 && job.RunID != "" {
		state, _, err := r.LoadAgentState(ctx, job.RunID)
		if err != nil {
			job.StepIndex = 1
		} else {
			job.StepIndex = state.StepIndex + 1
		}
	}
	if err := r.persistStepJob(ctx, job, stepJobQueued); err != nil {
		return err
	}
	if r.worker == nil {
		return nil
	}
	return r.worker.Submit(ctx, job)
}

// TryClaimStep 互斥领取指定 Run 的指定步骤，防止多个 Worker 重复执行。
func (r *Runtime) TryClaimStep(_ context.Context, runID string, stepIndex int) (bool, error) {
	if r == nil {
		return false, nil
	}
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	if r.claimedSteps == nil {
		r.claimedSteps = map[string]struct{}{}
	}
	key := stepClaimKey(runID, stepIndex)
	if _, ok := r.claimedSteps[key]; ok {
		return false, nil
	}
	r.claimedSteps[key] = struct{}{}
	return true, nil
}

// releaseStep 释放 TryClaimStep 占用的步骤锁。
func (r *Runtime) releaseStep(runID string, stepIndex int) {
	if r == nil {
		return
	}
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	delete(r.claimedSteps, stepClaimKey(runID, stepIndex))
}

// stepClaimKey 返回步骤互斥锁的键：run_id + step_index。
func stepClaimKey(runID string, stepIndex int) string {
	return fmt.Sprintf("%s/%d", runID, stepIndex)
}

// LoadAgentState 从数据库加载 Run、checkpoint、消息、可见工具和冻结目录。
// 逻辑：读 Run/Session → 装 checkpoint 与审批裁决 → 推算 StepIndex 与下一 Turn → 过滤压缩后的消息 → 拼 History。
func (r *Runtime) LoadAgentState(ctx context.Context, runID string) (pkgagent.AgentState, pkgagent.History, error) {
	if r == nil || r.queries == nil {
		return pkgagent.AgentState{}, pkgagent.History{}, cderr.Invalid("runtime is not initialized")
	}
	if runID == "" {
		return pkgagent.AgentState{}, pkgagent.History{}, cderr.Invalid("run id is required")
	}
	q := r.q(ctx)
	row, err := q.GetRun(ctx, runID)
	if err != nil {
		return pkgagent.AgentState{}, pkgagent.History{}, wrapDB(err)
	}
	run := mapRun(row)
	sessRow, err := q.GetSession(ctx, run.SessionID)
	if err != nil {
		return pkgagent.AgentState{}, pkgagent.History{}, wrapDB(err)
	}
	sess := mapSession(sessRow)

	state := pkgagent.AgentState{
		SessionID:       run.SessionID,
		RunID:           run.ID,
		TurnID:          run.CurrentTurnID,
		Status:          run.Status,
		Config:          run.Config,
		CancelRequested: run.CancelRequested,
		StopReason:      run.StopReason,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
	}

	if cpRow, err := q.GetRunToolCheckpoint(ctx, runID); err == nil {
		state.Checkpoint = mapToolCheckpoint(cpRow)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return pkgagent.AgentState{}, pkgagent.History{}, err
	}

	turns, err := q.ListRunTurns(ctx, runID)
	if err != nil {
		return pkgagent.AgentState{}, pkgagent.History{}, err
	}
	nextTurn := pkgagent.Turn{Number: len(turns) + 1}
	if run.CurrentTurnID != nil && *run.CurrentTurnID != "" {
		if turnRow, err := q.GetTurn(ctx, *run.CurrentTurnID); err == nil {
			mapped := mapTurn(turnRow)
			nextTurn.ID = mapped.ID
			if mapped.Number > 0 {
				nextTurn.Number = mapped.Number + 1
			}
		}
	}
	if maxTurns := run.Config.Limits.MaxTurns; maxTurns > 0 && len(turns) >= maxTurns && len(state.Checkpoint.Pending) == 0 {
		state.ForceFinish = true
	}

	state.StepIndex = r.inferStepIndex(ctx, run.SessionID, run.ID)
	r.applyApprovalDecisions(ctx, run.SessionID, run.ID, &state)

	msgRows, err := q.ListSessionMessages(ctx, run.SessionID)
	if err != nil {
		return pkgagent.AgentState{}, pkgagent.History{}, err
	}
	messages := make([]pkgagent.Message, 0, len(msgRows))
	for _, item := range msgRows {
		messages = append(messages, mapMessage(item))
	}

	var compact *pkgagent.CompactionCheckpoint
	if cp, err := q.GetLatestCheckpoint(ctx, run.SessionID); err == nil {
		mapped := mapCompaction(cp)
		compact = &mapped
		filtered := messages[:0]
		for _, msg := range messages {
			if msg.EventSeq > mapped.BaseEventSeq {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	} else if !errors.Is(err, sql.ErrNoRows) {
		return pkgagent.AgentState{}, pkgagent.History{}, err
	}

	names := run.Config.Profile.Tools.Names
	tools := tool.VisibleDefinitions(tool.Definitions(r.tools), names, tool.ModeCapabilities(string(run.Mode)))
	prompt := run.Config.Profile.Prompt.Inline
	if prompt == "" {
		prompt = pkgagent.DefaultSystemPrompt
	}

	hist := pkgagent.History{
		Run:           run,
		Turn:          nextTurn,
		Checkpoint:    compact,
		Messages:      messages,
		Tools:         tools,
		Prompt:        prompt,
		MemoryIndexes: r.loadMemoryIndexes(ctx, sess.UserID, sess.WorkspaceID),
	}
	return state, hist, nil
}

// Append 实现 FactWriter，供 Engine 在步骤内写流式事实。
func (r *Runtime) Append(ctx context.Context, runID string, fact pkgagent.Fact) error {
	_, err := r.AppendFact(ctx, runID, fact)
	return err
}

// AppendFact 同事务递增 seq 并写入 AgentEvent，提交后发布到总线。
func (r *Runtime) AppendFact(ctx context.Context, runID string, fact pkgagent.Fact) (pkgagent.AgentEvent, error) {
	if r == nil || r.db == nil {
		return pkgagent.AgentEvent{}, cderr.Invalid("runtime is not initialized")
	}
	if runID == "" {
		return pkgagent.AgentEvent{}, cderr.Invalid("run id is required")
	}
	if _, ok := db.TxFromContext(ctx); ok {
		return r.insertEventForRun(ctx, runID, fact)
	}
	var ev pkgagent.AgentEvent
	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ev, err = r.insertEventForRun(ctx, runID, fact)
		return err
	})
	if err != nil {
		return pkgagent.AgentEvent{}, err
	}
	r.publish(ev)
	return ev, nil
}

// CommitStep 校验状态与步骤序号，持久化 Run / Turn / Message / checkpoint，并投递下一步。
// 逻辑：终态或旧步骤直接返回 → 事务写 Turn/消息/checkpoint/Run/事件 → 提交后发布、索引、标记 job → Enqueue Next。
func (r *Runtime) CommitStep(ctx context.Context, runID string, result pkgagent.StepResult) error {
	if r == nil || r.db == nil {
		return cderr.Invalid("runtime is not initialized")
	}
	if runID == "" {
		runID = result.State.RunID
	}
	if runID == "" {
		return cderr.Invalid("run id is required")
	}

	current, _, err := r.LoadAgentState(ctx, runID)
	if err != nil {
		return err
	}
	if pkgagent.IsTerminal(current.Status) {
		return nil
	}
	if result.State.StepIndex > 0 && result.State.StepIndex <= current.StepIndex {
		return nil
	}
	if err := canReach(current.Status, result.State.Status); err != nil {
		return err
	}

	state := result.State
	state.RunID = runID
	if state.SessionID == "" {
		state.SessionID = current.SessionID
	}
	now := util.Now()
	nowStr := util.FormatTime(now)
	var published []pkgagent.AgentEvent
	var indexed []pkgagent.Message
	sessionID := state.SessionID
	terminal := pkgagent.IsTerminal(state.Status)

	err = r.db.WithTx(ctx, func(ctx context.Context) error {
		q := r.q(ctx)
		runRow, err := q.GetRun(ctx, runID)
		if err != nil {
			return wrapDB(err)
		}
		run := mapRun(runRow)
		sessionID = run.SessionID
		if state.SessionID == "" {
			state.SessionID = run.SessionID
		}

		turnID := deref(state.TurnID)
		if turnID != "" {
			if _, err := q.GetTurn(ctx, turnID); err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				turns, err := q.ListRunTurns(ctx, runID)
				if err != nil {
					return err
				}
				status := string(pkgagent.TurnRunning)
				if state.Status == pkgagent.RunWaitingApproval {
					status = string(pkgagent.TurnWaitingApproval)
				}
				if terminal {
					status = turnStatusFor(state.Status)
				}
				if _, err := q.InsertTurn(ctx, sqlite.InsertTurnParams{
					ID:        turnID,
					RunID:     runID,
					Number:    int64(len(turns) + 1),
					Status:    status,
					StartedAt: nullString(nowStr),
					FinishedAt: func() sql.NullString {
						if terminal {
							return nullString(nowStr)
						}
						return sql.NullString{}
					}(),
				}); err != nil {
					return err
				}
			} else if terminal || state.Status == pkgagent.RunWaitingApproval {
				turnRow, _ := q.GetTurn(ctx, turnID)
				status := turnRow.Status
				finishedAt := turnRow.FinishedAt
				if state.Status == pkgagent.RunWaitingApproval {
					status = string(pkgagent.TurnWaitingApproval)
				}
				if terminal {
					status = turnStatusFor(state.Status)
					finishedAt = nullString(nowStr)
				}
				if _, err := q.UpdateTurn(ctx, sqlite.UpdateTurnParams{
					Status:         status,
					FirstEventSeq:  turnRow.FirstEventSeq,
					LastEventSeq:   turnRow.LastEventSeq,
					AssistantMsgID: turnRow.AssistantMsgID,
					UsageID:        turnRow.UsageID,
					StartedAt:      turnRow.StartedAt,
					FinishedAt:     finishedAt,
					ID:             turnID,
				}); err != nil {
					return err
				}
			}
		}

		for _, msg := range result.Messages {
			if msg.ID == "" {
				msg.ID = util.NewID()
			}
			if msg.SessionID == "" {
				msg.SessionID = sessionID
			}
			seq, err := q.IncrementEventSeq(ctx, sqlite.IncrementEventSeqParams{UpdatedAt: nowStr, ID: sessionID})
			if err != nil {
				return err
			}
			content := string(msg.Content)
			if content == "" {
				content = string(pkgagent.EncodeText(""))
			}
			var toolCalls sql.NullString
			if len(msg.ToolCalls) > 0 {
				toolCalls = nullString(marshalJSON(msg.ToolCalls))
			}
			if _, err := q.InsertMessage(ctx, sqlite.InsertMessageParams{
				ID:        msg.ID,
				SessionID: sessionID,
				RunID:     nullString(runID),
				TurnID:    nullString(deref(msg.TurnID)),
				Role:      string(msg.Role),
				Content:   content,
				ToolCalls: toolCalls,
				EventSeq:  seq,
				CreatedAt: nowStr,
			}); err != nil {
				return err
			}
			msg.EventSeq = seq
			msg.CreatedAt = now
			indexed = append(indexed, msg)
		}

		if _, err := q.UpsertRunToolCheckpoint(ctx, sqlite.UpsertRunToolCheckpointParams{
			RunID:          runID,
			TurnID:         state.Checkpoint.TurnID,
			CompletedCalls: marshalJSON(state.Checkpoint.Completed),
			PendingCalls:   marshalJSON(state.Checkpoint.Pending),
			Results:        marshalJSON(state.Checkpoint.Results),
			ApprovedCalls:  marshalJSON(state.Checkpoint.Approved),
			DeniedCalls:    marshalJSON(state.Checkpoint.Denied),
			UpdatedAt:      nowStr,
		}); err != nil {
			return err
		}

		if state.Status == pkgagent.RunWaitingApproval {
			if err := r.insertPendingApproval(ctx, sessionID, runID, state); err != nil {
				return err
			}
		}

		started := formatTimePtr(state.StartedAt)
		if !started.Valid && (state.Status == pkgagent.RunRunningLLM || state.Status == pkgagent.RunExecutingTools || state.Status == pkgagent.RunWaitingApproval) {
			started = nullString(nowStr)
		}
		finished := formatTimePtr(state.FinishedAt)
		if terminal && !finished.Valid {
			finished = nullString(nowStr)
		}
		stop := sql.NullString{}
		if state.StopReason != nil {
			stop = nullString(string(*state.StopReason))
		}
		if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{
			Status:          string(state.Status),
			CurrentTurnID:   nullString(deref(state.TurnID)),
			StopReason:      stop,
			CancelRequested: boolInt(state.CancelRequested || run.CancelRequested),
			StartedAt:       started,
			FinishedAt:      finished,
			ID:              runID,
		}); err != nil {
			return err
		}

		if current.Status != state.Status {
			ev, err := r.insertEventTx(ctx, sessionID, runID, deref(state.TurnID), pkgagent.Fact{
				Type:   pkgagent.EventRunStateChanged,
				TurnID: state.TurnID,
				Payload: pkgagent.MarshalPayload(pkgagent.RunStateChangedPayload{
					From:   current.Status,
					To:     state.Status,
					Reason: fmt.Sprintf("step %d", state.StepIndex),
				}),
			})
			if err != nil {
				return err
			}
			published = append(published, ev)
		}
		for _, fact := range result.Facts {
			ev, err := r.insertEventTx(ctx, sessionID, runID, deref(fact.TurnID), fact)
			if err != nil {
				return err
			}
			published = append(published, ev)
		}
		if terminal {
			if err := q.ClearActiveRun(ctx, sqlite.ClearActiveRunParams{
				UpdatedAt:   nowStr,
				ID:          sessionID,
				ActiveRunID: nullString(runID),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, ev := range published {
		r.publish(ev)
	}
	for _, msg := range indexed {
		r.indexMessage(ctx, sessionID, msg)
	}
	doneStatus := stepJobDone
	if terminal && state.Status == pkgagent.RunCancelled {
		doneStatus = stepJobCancelled
	}
	if state.StepIndex > 0 {
		r.markStepJobStatus(ctx, runID, state.StepIndex, doneStatus)
	}
	if result.Next != nil && !terminal && !state.CancelRequested {
		if err := r.Enqueue(ctx, *result.Next); err != nil {
			return err
		}
	}
	return nil
}

// RequestCancel 标记取消；queued / waiting_approval 立即终态并清 active。
// 逻辑：终态直接返回 → queued/审批中立刻 cancelled 并出队 → 运行中只标 cancel_requested，由 Worker 收束。
func (r *Runtime) RequestCancel(ctx context.Context, runID string) error {
	if r == nil || r.db == nil {
		return cderr.Invalid("runtime is not initialized")
	}
	if runID == "" {
		return cderr.Invalid("run id is required")
	}
	row, err := r.q(ctx).GetRun(ctx, runID)
	if err != nil {
		return wrapDB(err)
	}
	run := mapRun(row)
	if pkgagent.IsTerminal(run.Status) {
		return nil
	}
	immediate := run.Status == pkgagent.RunQueued || run.Status == pkgagent.RunWaitingApproval
	nowStr := util.FormatTime(util.Now())
	reason := pkgagent.StopCancelled
	var published []pkgagent.AgentEvent

	err = r.db.WithTx(ctx, func(ctx context.Context) error {
		q := r.q(ctx)
		status := run.Status
		finished := sql.NullString{}
		stop := sql.NullString{}
		if immediate {
			status = pkgagent.RunCancelled
			finished = nullString(nowStr)
			stop = nullString(string(reason))
		}
		if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{
			Status:          string(status),
			CurrentTurnID:   nullString(deref(run.CurrentTurnID)),
			StopReason:      stop,
			CancelRequested: 1,
			StartedAt:       formatTimePtr(run.StartedAt),
			FinishedAt:      finished,
			ID:              runID,
		}); err != nil {
			return err
		}
		if immediate {
			ev, err := r.insertEventTx(ctx, run.SessionID, runID, deref(run.CurrentTurnID), pkgagent.Fact{
				Type:    pkgagent.EventRunCancelled,
				Payload: pkgagent.MarshalPayload(pkgagent.RunTerminalPayload{Status: pkgagent.RunCancelled, StopReason: &reason}),
			})
			if err != nil {
				return err
			}
			published = append(published, ev)
			return q.ClearActiveRun(ctx, sqlite.ClearActiveRunParams{
				UpdatedAt:   nowStr,
				ID:          run.SessionID,
				ActiveRunID: nullString(runID),
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, ev := range published {
		r.publish(ev)
	}
	r.cancelOpenStepJobs(ctx, runID)
	if r.worker != nil {
		r.worker.Cancel(runID)
	}
	return nil
}

// RecoverRun 把指定 Run 的未完成 Job 重新入队。本进程已在跑则直接返回。
// waiting_approval 且尚未裁决时不入队。
// 逻辑：Busy/终态/未裁决审批跳过 → 优先用未完成 step_job（崩溃 running 则 attempt++）→ 未占会话则 Claim，抢不到只落库。
func (r *Runtime) RecoverRun(ctx context.Context, runID string) error {
	if r == nil || runID == "" {
		return cderr.Invalid("run id is required")
	}
	if r.worker != nil && r.worker.Busy(runID) {
		return nil
	}
	state, _, err := r.LoadAgentState(ctx, runID)
	if err != nil {
		return err
	}
	if pkgagent.IsTerminal(state.Status) {
		return nil
	}
	if state.Status == pkgagent.RunWaitingApproval && !checkpointHasDecision(state.Checkpoint) {
		return nil
	}

	job := pkgagent.StepJob{
		RunID:     runID,
		StepIndex: state.StepIndex + 1,
		Phase:     recoverPhase(state.Status, state),
	}
	if state.Status == pkgagent.RunWaitingApproval {
		job.Phase = pkgagent.PhaseHumanApproved
	}
	if row, ok, err := r.latestOpenStepJob(ctx, runID); err != nil {
		return err
	} else if ok {
		job = stepJobFromRow(row)
		if state.Status == pkgagent.RunWaitingApproval {
			job.Phase = pkgagent.PhaseHumanApproved
		}
		if row.Status == stepJobRunning {
			job.Attempt++
		}
	}

	sess, err := r.q(ctx).GetSession(ctx, state.SessionID)
	if err != nil {
		return wrapDB(err)
	}
	active := sess.ActiveRunID.Valid && sess.ActiveRunID.String == runID
	if !active {
		claimed, err := r.ClaimSession(ctx, state.SessionID, runID)
		if err != nil {
			return err
		}
		if !claimed {
			return r.persistStepJob(ctx, job, stepJobQueued)
		}
	}
	return r.Enqueue(ctx, job)
}

// NeedsRecover 判断指定 Run 是否因执行中断而需要用户显式恢复。
// Worker 仍在跑、等审批、取消中、已终态，或只是排在当前 active Run 之后时返回 false。
func (r *Runtime) NeedsRecover(ctx context.Context, runID string) (bool, error) {
	if r == nil || runID == "" {
		return false, nil
	}
	busy := r.worker != nil && r.worker.Busy(runID)
	row, err := r.q(ctx).GetRun(ctx, runID)
	if err != nil {
		return false, wrapDB(err)
	}
	if !pkgagent.NeedsUserRecover(pkgagent.RunStatus(row.Status), busy) {
		return false, nil
	}
	sess, err := r.q(ctx).GetSession(ctx, row.SessionID)
	if err != nil {
		return false, wrapDB(err)
	}
	return sess.ActiveRunID.Valid && sess.ActiveRunID.String == runID, nil
}

// RecoverActive 扫未完成 Run 并逐个 RecoverRun。仅供测试或内部扫表，启动时不调用。
func (r *Runtime) RecoverActive(ctx context.Context) error {
	if r == nil || r.queries == nil {
		return nil
	}
	q := r.q(ctx)
	runs, err := q.ListRecoverableRuns(ctx)
	if err != nil {
		return err
	}
	for _, row := range runs {
		if err := r.RecoverRun(ctx, row.ID); err != nil {
			r.logger().Error("recover run failed", "run_id", row.ID, "error", err)
		}
	}
	waiting, err := q.ListWaitingApprovalRuns(ctx)
	if err != nil {
		return err
	}
	for _, row := range waiting {
		if err := r.RecoverRun(ctx, row.ID); err != nil {
			r.logger().Error("recover approval failed", "run_id", row.ID, "error", err)
		}
	}
	return nil
}

// insertEventForRun 按 Run 查出 Session，再写入一条 AgentEvent。
func (r *Runtime) insertEventForRun(ctx context.Context, runID string, fact pkgagent.Fact) (pkgagent.AgentEvent, error) {
	run, err := r.q(ctx).GetRun(ctx, runID)
	if err != nil {
		return pkgagent.AgentEvent{}, wrapDB(err)
	}
	return r.insertEventTx(ctx, run.SessionID, runID, deref(fact.TurnID), fact)
}

// insertEventTx 在当前事务内递增 seq 并插入 AgentEvent。
func (r *Runtime) insertEventTx(ctx context.Context, sessionID, runID, turnID string, fact pkgagent.Fact) (pkgagent.AgentEvent, error) {
	q := r.q(ctx)
	nowStr := util.FormatTime(util.Now())
	seq, err := q.IncrementEventSeq(ctx, sqlite.IncrementEventSeqParams{UpdatedAt: nowStr, ID: sessionID})
	if err != nil {
		return pkgagent.AgentEvent{}, err
	}
	payload := string(fact.Payload)
	if payload == "" {
		payload = "{}"
	}
	row, err := q.InsertAgentEvent(ctx, sqlite.InsertAgentEventParams{
		EventID:    util.NewID(),
		SessionID:  sessionID,
		RunID:      runID,
		TurnID:     nullString(turnID),
		Seq:        seq,
		Type:       string(fact.Type),
		Version:    1,
		OccurredAt: nowStr,
		Payload:    payload,
	})
	if err != nil {
		return pkgagent.AgentEvent{}, err
	}
	return mapEvent(row), nil
}

// publish 把已落库的 AgentEvent 发到进程内总线，供 SSE 订阅。
func (r *Runtime) publish(ev pkgagent.AgentEvent) {
	if r == nil || r.bus == nil || ev.EventID == "" {
		return
	}
	r.bus.Publish(events.Event{
		Type:          string(ev.Type),
		ChatSessionID: ev.SessionID,
		Payload:       ev,
	})
}

// inferStepIndex 从 run.state_changed 事件推断已提交的最大步骤号。
func (r *Runtime) inferStepIndex(ctx context.Context, sessionID, runID string) int {
	rows, err := r.q(ctx).ListSessionEventsAfter(ctx, sqlite.ListSessionEventsAfterParams{SessionID: sessionID, Seq: 0})
	if err != nil {
		return 0
	}
	step := 0
	for _, row := range rows {
		if row.RunID != runID || pkgagent.EventType(row.Type) != pkgagent.EventRunStateChanged {
			continue
		}
		var payload pkgagent.RunStateChangedPayload
		if err := json.Unmarshal([]byte(row.Payload), &payload); err == nil && strings.HasPrefix(payload.Reason, "step ") {
			var n int
			if _, err := fmt.Sscanf(payload.Reason, "step %d", &n); err == nil && n > step {
				step = n
			}
			continue
		}
		step++
	}
	return step
}

// applyApprovalDecisions 把该 Run 的审批表裁决合并进 checkpoint 的 Approved/Denied。
func (r *Runtime) applyApprovalDecisions(ctx context.Context, sessionID, runID string, state *pkgagent.AgentState) {
	rows, err := r.q(ctx).ListSessionApprovals(ctx, sqlite.ListSessionApprovalsParams{
		SessionID: sessionID,
		SortBy:    "id",
		SortOrder: "asc",
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.RunID != runID {
			continue
		}
		approval := mapApproval(row)
		if approval.Status == pkgagent.ApprovalPending {
			id := approval.ID
			state.PendingApproval = &id
			continue
		}
		for _, call := range approval.ToolCalls {
			switch call.Status {
			case pkgagent.ApprovalApproved:
				if !containsString(state.Checkpoint.Approved, call.ID) {
					state.Checkpoint.Approved = append(state.Checkpoint.Approved, call.ID)
				}
			case pkgagent.ApprovalDenied, pkgagent.ApprovalExpired:
				if !containsString(state.Checkpoint.Denied, call.ID) {
					state.Checkpoint.Denied = append(state.Checkpoint.Denied, call.ID)
				}
			}
		}
	}
}

// insertPendingApproval 为等待审批的工具调用插入一条 pending 审批；已存在则跳过。
func (r *Runtime) insertPendingApproval(ctx context.Context, sessionID, runID string, state pkgagent.AgentState) error {
	approvalID := deref(state.PendingApproval)
	if approvalID == "" {
		approvalID = util.NewID()
	}
	if _, err := r.q(ctx).GetApproval(ctx, approvalID); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	calls := make([]pkgagent.ApprovalToolCall, 0, len(state.Checkpoint.Pending))
	for _, call := range state.Checkpoint.Pending {
		calls = append(calls, pkgagent.ApprovalToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
			Status:    pkgagent.ApprovalPending,
		})
	}
	first := ""
	if len(calls) > 0 {
		first = calls[0].ID
	}
	expiry := util.Now().Add(time.Hour)
	if state.Config.ApprovalPolicy.DefaultExpiry > 0 {
		expiry = util.Now().Add(state.Config.ApprovalPolicy.DefaultExpiry)
	}
	_, err := r.q(ctx).InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID:         approvalID,
		SessionID:  sessionID,
		RunID:      runID,
		ToolCallID: first,
		ToolCalls:  marshalJSON(calls),
		Scope:      string(pkgagent.ApprovalOnce),
		Status:     string(pkgagent.ApprovalPending),
		ExpiresAt:  util.FormatTime(expiry),
	})
	return err
}

// ReindexMessage 按 Session 工作区更新一条消息的冷层索引。
func (r *Runtime) ReindexMessage(ctx context.Context, sessionID string, msg pkgagent.Message) {
	if r == nil || sessionID == "" || msg.ID == "" {
		return
	}
	r.indexMessage(ctx, sessionID, msg)
}

// indexPersistedMessage 把该 Run 的触发用户消息写入冷层索引。
func (r *Runtime) indexPersistedMessage(ctx context.Context, runID string) {
	row, err := r.q(ctx).GetRun(ctx, runID)
	if err != nil {
		return
	}
	msg, err := r.q(ctx).GetMessage(ctx, row.TriggerMessageID)
	if err != nil {
		return
	}
	r.indexMessage(ctx, row.SessionID, mapMessage(msg))
}

// indexMessage 按 Session 工作区把一条已落库消息写入冷层 FTS。
func (r *Runtime) indexMessage(ctx context.Context, sessionID string, msg pkgagent.Message) {
	sess, err := r.q(ctx).GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	r.indexPersisted(ctx, sess.WorkspaceID, msg)
}

// recoverPhase 按 Run 粗状态推断恢复时应投递的 Phase。
func recoverPhase(status pkgagent.RunStatus, state pkgagent.AgentState) pkgagent.Phase {
	switch status {
	case pkgagent.RunQueued, pkgagent.RunLoadingContext:
		return pkgagent.PhaseUserInput
	case pkgagent.RunRunningLLM:
		return pkgagent.PhaseLLMResult
	case pkgagent.RunExecutingTools:
		if len(state.Checkpoint.Results) > 0 {
			return pkgagent.PhaseToolsBatchResult
		}
		return pkgagent.PhaseLLMResult
	default:
		return pkgagent.PhaseUserInput
	}
}

// checkpointHasDecision 判断 checkpoint 是否已有通过或拒绝的工具调用。
func checkpointHasDecision(cp pkgagent.ToolCheckpoint) bool {
	return len(cp.Approved) > 0 || len(cp.Denied) > 0
}

// canReach 判断 from 能否经合法一跳或多跳到达 to，供一次 Commit 跨中间态。
func canReach(from, to pkgagent.RunStatus) error {
	if from == to {
		return nil
	}
	if err := pkgagent.CanTransition(from, to); err == nil {
		return nil
	}
	type node struct {
		status pkgagent.RunStatus
	}
	queue := []node{{from}}
	seen := map[pkgagent.RunStatus]bool{from: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range neighbors(cur.status) {
			if seen[next] {
				continue
			}
			if next == to {
				return nil
			}
			seen[next] = true
			queue = append(queue, node{next})
		}
	}
	return pkgagent.CanTransition(from, to)
}

// neighbors 返回 from 的合法下一跳状态，供 canReach 做广度搜索。
func neighbors(from pkgagent.RunStatus) []pkgagent.RunStatus {
	all := []pkgagent.RunStatus{
		pkgagent.RunQueued,
		pkgagent.RunLoadingContext,
		pkgagent.RunRunningLLM,
		pkgagent.RunExecutingTools,
		pkgagent.RunWaitingApproval,
		pkgagent.RunCancelling,
		pkgagent.RunCompleted,
		pkgagent.RunFailed,
		pkgagent.RunCancelled,
	}
	out := make([]pkgagent.RunStatus, 0, 4)
	for _, next := range all {
		if pkgagent.CanTransition(from, next) == nil && from != next {
			out = append(out, next)
		}
	}
	return out
}

// turnStatusFor 把 Run 终态映射成对应的 Turn 状态。
func turnStatusFor(status pkgagent.RunStatus) string {
	switch status {
	case pkgagent.RunFailed:
		return string(pkgagent.TurnFailed)
	case pkgagent.RunCancelled:
		return string(pkgagent.TurnCancelled)
	default:
		return string(pkgagent.TurnCompleted)
	}
}

// clipSessionSummary 取用户正文首行并截断，用作 Session 摘要。
func clipSessionSummary(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if i := strings.IndexAny(content, "\r\n"); i >= 0 {
		content = strings.TrimSpace(content[:i])
	}
	runes := []rune(content)
	if len(runes) > sessionSummaryMaxRunes {
		return string(runes[:sessionSummaryMaxRunes])
	}
	return content
}

// containsString 判断字符串切片是否包含指定值。
func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
