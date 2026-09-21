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
		messages = append(messages, harnessToolMessage(state, "verify", result.Output, result))
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

// runEvaluate 执行独立复审并进入 evaluate_result。驳回时把问题清单喂回。
func (e *Engine) runEvaluate(ctx context.Context, in StepInput) (StepResult, error) {
	if err := ctx.Err(); err != nil {
		return e.finish(ctx, in, finishInstructions(RunCancelled, StopCancelled)[0])
	}
	state := in.State
	state.StepIndex = in.Job.StepIndex
	if state.StepIndex <= 0 {
		state.StepIndex = 1
	}
	state.Status = RunEvaluating
	round := state.EvaluateRound + 1
	_ = e.appendFact(ctx, state.RunID, Fact{
		Type:    EventEvaluateStarted,
		TurnID:  state.TurnID,
		Payload: MarshalPayload(EvaluateStartedPayload{Round: round}),
	})

	result, err := e.evaluateWorkspace(ctx, state)
	if err != nil {
		result = EvaluationResult{Verdict: VerdictEscalate, Summary: err.Error()}
	}
	if result.Verdict == VerdictNeedsWork || result.Verdict == VerdictEscalate {
		state.EvaluateRound = round
	}
	if strings.TrimSpace(result.Summary) != "" {
		state.LastEvaluateSummary = result.Summary
	}
	leaveEvaluating(&state, result)

	_ = e.appendFact(ctx, state.RunID, Fact{
		Type:    EventEvaluateResult,
		TurnID:  state.TurnID,
		Payload: MarshalPayload(result),
	})

	var messages []Message
	if result.Verdict != VerdictPass {
		messages = append(messages, harnessToolMessage(state, "evaluate", result.Summary, result))
	}
	return StepResult{
		State:    state,
		Messages: messages,
		Next: &StepJob{
			RunID:     state.RunID,
			StepIndex: state.StepIndex + 1,
			Phase:     PhaseEvaluateResult,
			Payload:   MarshalPayload(result),
		},
	}, nil
}

// leaveEvaluating 复审已有结论就离开 evaluating，避免结果已发出而 Run 仍显示 Reviewing。
func leaveEvaluating(state *AgentState, result EvaluationResult) {
	if state == nil {
		return
	}
	switch result.Verdict {
	case VerdictPass:
		state.Status = RunRunningLLM
		if !result.Skipped {
			state.WrapUpPending = true
		}
	case VerdictNeedsWork:
		state.Status = RunRunningLLM
	}
}

// requestOverride 打开验证/复审人工单并停在 waiting_approval。
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

// rememberHarnessFingerprints 在验证/复审打回后记下指纹，供下一轮判重。
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
	case PhaseEvaluateResult:
		var result EvaluationResult
		if json.Unmarshal(payload, &result) == nil {
			if result.DiffFingerprint != "" {
				state.LastEvaluateFingerprint = result.DiffFingerprint
			}
			if strings.TrimSpace(result.Summary) != "" {
				state.LastEvaluateSummary = result.Summary
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
	return CheckWorkspace(ctx, state.WorkspaceRoot, diffFiles, round, state.LastVerifyFingerprint, e.runner)
}

// evaluateWorkspace 优先走 fake 脚本，否则清洗 diff 后调复审模型。
func (e *Engine) evaluateWorkspace(ctx context.Context, state AgentState) (EvaluationResult, error) {
	model := FallbackModel(state.Config.EvaluatorModel, state.Config.Model)
	if model.Provider == "" {
		model = state.Config.Model
	}
	opts := ParseFakeOptions(model.Options)
	if len(opts.Evaluate) == 0 {
		opts = ParseFakeOptions(state.Config.Model.Options)
	}
	if strings.EqualFold(model.Provider, "fake") || strings.EqualFold(state.Config.Model.Provider, "fake") {
		idx := state.EvaluateRound
		if len(opts.Evaluate) > 0 {
			if idx >= len(opts.Evaluate) {
				idx = len(opts.Evaluate) - 1
			}
			item := opts.Evaluate[idx]
			if item.Fail {
				return EvaluationResult{}, errFakeEvaluate
			}
			verdict := EvaluationVerdict(item.Verdict)
			if verdict == "" {
				verdict = VerdictPass
			}
			return EvaluationResult{Verdict: verdict, Issues: item.Issues, Summary: item.Summary, DiffFingerprint: "fake"}, nil
		}
	}
	rawDiff := collectWorkspaceDiff(e, state)
	sanitized, err := SanitizeDiff(rawDiff, defaultEvaluateMaxB)
	if err != nil {
		return EvaluationResult{Verdict: VerdictEscalate, Summary: err.Error()}, nil
	}
	return EvaluateRun(ctx, model, sanitized, LoadPlanItems(state.WorkspaceRoot, state.ActivePlan), state.LastVerifySummary, state.LastEvaluateFingerprint)
}

// summarizeVerify 把验证结果压成复审能读的短摘要。
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

// collectWorkspaceDiff 有快照时读相对基线的改动；没有基线则不编造 diff。
func collectWorkspaceDiff(e *Engine, state AgentState) string {
	if e == nil || e.snapshots == nil || strings.TrimSpace(state.WorkspaceRoot) == "" {
		return ""
	}
	if strings.TrimSpace(state.SnapshotID) == "" && strings.TrimSpace(state.SnapshotHead) == "" && len(state.UntrackedFiles) == 0 {
		return ""
	}
	text, err := e.snapshots.Diff(state.WorkspaceRoot, SnapshotFromState(state))
	if err != nil {
		return ""
	}
	return text
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

// harnessToolMessage 把验证或复审结果写成一条 tool 消息。
func harnessToolMessage(state AgentState, name, text string, body any) Message {
	raw := MarshalPayload(body)
	if text != "" && len(raw) < 3 {
		raw = MarshalPayload(map[string]string{"error": text})
	}
	return Message{
		ID:        newEntityID(),
		SessionID: state.SessionID,
		RunID:     ptrValue(state.RunID),
		TurnID:    state.TurnID,
		Role:      RoleTool,
		Content:   EncodeToolResult(name, raw),
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

var errFakeEvaluate = errString("fake evaluate failed")

type errString string

func (e errString) Error() string { return string(e) }

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
