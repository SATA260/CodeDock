package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// runVerify 执行收尾验证并进入 verify_result。失败时把日志写成 tool 消息喂回。
func (e *Engine) runVerify(ctx context.Context, in StepInput) (StepResult, error) {
	if err := ctx.Err(); err != nil {
		return e.finish(ctx, in, finishInstructions(RunCancelled, StopCancelled)[0])
	}
	state := in.State
	if strings.TrimSpace(state.ActivePlan) == "" {
		state.ActivePlan = ResolvePlanScope(state.ActivePlan, in.History.Messages).ActivePlan
	}
	state.StepIndex = in.Job.StepIndex
	if state.StepIndex <= 0 {
		state.StepIndex = 1
	}
	state.Status = RunVerifying
	round := state.VerifyRound + 1
	_ = e.appendFact(ctx, state.RunID, Fact{
		Type:    EventVerifyStarted,
		TurnID:  state.TurnID,
		Payload: MarshalPayload(VerifyStartedPayload{Round: round}),
	})

	result, err := e.verifyWorkspace(ctx, state, round)
	if err != nil {
		result = VerifyResult{Status: VerifyStatusCannotRun, Output: err.Error(), Round: round}
	}
	result.Round = round
	state.VerifyRound = round
	state.LastVerifySummary = summarizeVerify(result)

	eventType := EventVerifyResult
	if result.Skipped {
		eventType = EventVerifySkipped
		result.Status = VerifyStatusPassed
	}
	_ = e.appendFact(ctx, state.RunID, Fact{
		Type:    eventType,
		TurnID:  state.TurnID,
		Payload: MarshalPayload(result),
	})

	var messages []Message
	if result.Status != VerifyStatusPassed {
		messages = append(messages, harnessFeedbackMessage(state, "verify", result.Output, result))
	}
	return StepResult{
		State:    state,
		Messages: messages,
		Next: &StepJob{
			RunID:     state.RunID,
			StepIndex: state.StepIndex + 1,
			Phase:     PhaseVerifyResult,
			Payload:   MarshalPayload(result),
		},
	}, nil
}

// requestOverride 打开验证人工单并停在 waiting_approval。
func (e *Engine) requestOverride(_ context.Context, in StepInput, inst Instruction) (StepResult, error) {
	state := in.State
	state.StepIndex = in.Job.StepIndex
	if state.StepIndex <= 0 {
		state.StepIndex = 1
	}
	var payload ApprovalRequestPayload
	_ = json.Unmarshal(inst.Payload, &payload)
	kind := NormalizeApprovalKind(payload.Kind)
	approvalID := derefString(state.PendingApproval)
	if approvalID == "" {
		approvalID = newEntityID()
	}
	state.PendingApproval = &approvalID
	state.ApprovalKind = kind
	state.Status = RunWaitingApproval
	callID := string(kind)
	return StepResult{
		State: state,
		Facts: []Fact{{
			Type:   EventApprovalRequired,
			TurnID: state.TurnID,
			Payload: MarshalPayload(ApprovalRequiredPayload{
				ApprovalID: approvalID,
				Kind:       kind,
				ToolCalls: []ApprovalToolCall{{
					ID:     callID,
					Name:   callID,
					Status: ApprovalPending,
					Reason: payload.Reason,
				}},
			}),
		}},
	}, nil
}

// captureSnapshotIfNeeded 在首次破坏性工具执行前拍快照。
func (e *Engine) captureSnapshotIfNeeded(state AgentState, calls []toolNameArg) (AgentState, []Fact) {
	if state.HadSideEffects {
		return state, nil
	}
	mutating := false
	for _, call := range calls {
		if isMutatingTool(call.Name) {
			mutating = true
			break
		}
	}
	if !mutating {
		return state, nil
	}
	if e == nil || e.snapshots == nil || strings.TrimSpace(state.WorkspaceRoot) == "" {
		return state, []Fact{{
			Type:    EventSnapshotSkipped,
			TurnID:  state.TurnID,
			Payload: MarshalPayload(map[string]string{"reason": "snapshot port is not configured"}),
		}}
	}
	snap, err := e.snapshots.Take(state.WorkspaceRoot, state.RunID)
	if err != nil {
		return state, []Fact{{
			Type:    EventSnapshotSkipped,
			TurnID:  state.TurnID,
			Payload: MarshalPayload(map[string]string{"reason": err.Error()}),
		}}
	}
	state.SnapshotID = snap.SnapshotOID
	state.SnapshotHead = snap.Head
	state.UntrackedFiles = snap.UntrackedFiles
	return state, nil
}

// markSideEffects 工具成功执行后确认本次任务动过工作区。
func markSideEffects(state AgentState, results []struct {
	Name    string
	Success bool
}) AgentState {
	if state.HadSideEffects {
		return state
	}
	for _, result := range results {
		if result.Success && isMutatingTool(result.Name) {
			state.HadSideEffects = true
			return state
		}
	}
	return state
}

// rememberHarnessFingerprints 在验证打回后记下指纹，供下一轮判重。
func rememberHarnessFingerprints(phase Phase, payload json.RawMessage, state AgentState) AgentState {
	switch phase {
	case PhaseVerifyResult:
		var result VerifyResult
		if json.Unmarshal(payload, &result) == nil {
			if result.Fingerprint != "" {
				state.LastVerifyFingerprint = result.Fingerprint
			}
			if summary := summarizeVerify(result); summary != "" {
				state.LastVerifySummary = summary
			}
		}
	case PhaseHumanOverride:
		var item HumanOverridePayload
		if json.Unmarshal(payload, &item) == nil && item.Action == OverrideRetry {
			state.LastVerifyFingerprint = ""
			state.OverrideAction = ""
		}
	}
	return state
}

// verifyWorkspace 优先走 fake 脚本，否则按规则跑命令。
func (e *Engine) verifyWorkspace(ctx context.Context, state AgentState, round int) (VerifyResult, error) {
	opts := ParseFakeOptions(state.Config.Model.Options)
	if strings.EqualFold(state.Config.Model.Provider, "fake") && len(opts.Verify) == 0 {
		return VerifyResult{Status: VerifyStatusPassed, Skipped: true, Round: round}, nil
	}
	if len(opts.Verify) > 0 && strings.EqualFold(state.Config.Model.Provider, "fake") {
		idx := round - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(opts.Verify) {
			idx = len(opts.Verify) - 1
		}
		item := opts.Verify[idx]
		status := item.Status
		if status == "" {
			status = VerifyStatusPassed
		}
		fp := item.Fingerprint
		if fp == "" && status == VerifyStatusFailed {
			fp = VerifyFingerprint(item.FailedCommand, item.Output)
		}
		return VerifyResult{
			Status:        status,
			Output:        item.Output,
			Fingerprint:   fp,
			FailedCommand: item.FailedCommand,
			Round:         round,
		}, nil
	}
	diffFiles := []string(nil)
	if e != nil && e.snapshots != nil && (state.SnapshotID != "" || len(state.UntrackedFiles) > 0) {
		if files, err := e.snapshots.ChangedFiles(state.WorkspaceRoot, SnapshotFromState(state)); err == nil {
			diffFiles = files
		}
	}
	return CheckWorkspacePlan(ctx, state.WorkspaceRoot, state.ActivePlan, diffFiles, round, state.LastVerifyFingerprint, e.runner)
}

// summarizeVerify 把验证结果压成短摘要。
func summarizeVerify(result VerifyResult) string {
	var parts []string
	if result.Status != "" {
		parts = append(parts, "status="+string(result.Status))
	}
	if result.Skipped {
		parts = append(parts, "skipped=true")
	}
	if result.FailedCommand != "" {
		parts = append(parts, "failed_command="+result.FailedCommand)
	}
	if result.CoverageNote != "" {
		parts = append(parts, result.CoverageNote)
	}
	if strings.TrimSpace(result.Output) != "" {
		parts = append(parts, strings.TrimSpace(result.Output))
	}
	return strings.Join(parts, "\n")
}

// workspaceChanged 根据快照判断工作区是否真有文件改动；没有基线时不当成改过。
func (e *Engine) workspaceChanged(state AgentState) bool {
	if e == nil || e.snapshots == nil || strings.TrimSpace(state.WorkspaceRoot) == "" {
		return false
	}
	if strings.TrimSpace(state.SnapshotID) == "" && strings.TrimSpace(state.SnapshotHead) == "" && len(state.UntrackedFiles) == 0 {
		return false
	}
	files, err := e.snapshots.ChangedFiles(state.WorkspaceRoot, SnapshotFromState(state))
	if err != nil {
		return false
	}
	return len(files) > 0
}

// harnessFeedbackMessage 把验证结果写成用户可见说明，避免伪造无 tool_calls 的 tool 消息。
func harnessFeedbackMessage(state AgentState, name, text string, body any) Message {
	summary := strings.TrimSpace(text)
	if summary == "" {
		summary = string(MarshalPayload(body))
	}
	return Message{
		ID:        newEntityID(),
		SessionID: state.SessionID,
		RunID:     ptrValue(state.RunID),
		TurnID:    state.TurnID,
		Role:      RoleUser,
		Content:   EncodeText("【" + name + "】未通过：\n" + summary + "\n请根据输出修复，不要把这段当成用户改口。"),
	}
}

// approvalKindOf 读出请求人工审批指令上的 kind。
func approvalKindOf(raw json.RawMessage) (ApprovalKind, bool) {
	var payload ApprovalRequestPayload
	if json.Unmarshal(raw, &payload) != nil || payload.Kind == "" {
		return "", false
	}
	return NormalizeApprovalKind(payload.Kind), true
}

// isMutatingTool 判断工具是否会改文件或跑命令。
func isMutatingTool(name string) bool {
	switch name {
	case "write", "edit", "bash", "powershell":
		return true
	default:
		return false
	}
}

type toolNameArg struct {
	Name string
}

// RestoreSnapshot 按指定粒度还原工作区。
func (e *Engine) RestoreSnapshot(state AgentState, mode RestoreMode) {
	if e == nil || e.snapshots == nil || state.SnapshotID == "" {
		return
	}
	if mode == RestoreMessages {
		return
	}
	if mode == "" {
		mode = RestoreFiles
	}
	_ = e.snapshots.Restore(state.WorkspaceRoot, SnapshotFromState(state), mode)
}

// applyAbortRestore 在 abort 时按快照还原工作区。
func (e *Engine) applyAbortRestore(state AgentState) {
	e.RestoreSnapshot(state, RestoreAll)
}

// nowUnixMilli 返回当前 UTC 毫秒时间戳。
func nowUnixMilli() int64 {
	return time.Now().UTC().UnixMilli()
}
