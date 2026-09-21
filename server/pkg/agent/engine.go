package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
)

// Engine 执行一步：按 Brain 的指令调用对应执行器，自身不直接写库、不发事件、不调度下一步。
type Engine struct {
	brain      *Brain
	facts      FactWriter
	tools      tool.Registry
	llmGate    tool.Gate
	toolGate   tool.Gate
	dispatcher seam.Dispatcher
	snapshots  SnapshotPort
	runner     CommandRunner
}

// NewEngine 创建执行引擎。brain 为空时自动构造一个空 Brain。
func NewEngine(brain *Brain, facts FactWriter, tools tool.Registry) *Engine {
	if brain == nil {
		brain = &Brain{}
	}
	return &Engine{brain: brain, facts: facts, tools: tools}
}

// SetGates 设置进程级 LLM / 工具占槽。nil 表示不限制。
func (e *Engine) SetGates(llm, tools tool.Gate) {
	if e == nil {
		return
	}
	e.llmGate = llm
	e.toolGate = tools
}

// SetDispatcher 设置步骤内各口使用的喊话器。nil 表示各口原样通过。
func (e *Engine) SetDispatcher(d seam.Dispatcher) {
	if e == nil {
		return
	}
	e.dispatcher = d
}

// SetHarness 注入快照与命令执行；空值时验证走默认 shell，快照则跳过。
func (e *Engine) SetHarness(snapshots SnapshotPort, runner CommandRunner) {
	if e == nil {
		return
	}
	e.snapshots = snapshots
	e.runner = runner
}

// Step 执行一步：先让 Brain 决策，再按指令类型分发到对应执行器。
func (e *Engine) Step(ctx context.Context, in StepInput) (StepResult, error) {
	if e == nil {
		return StepResult{State: in.State}, nil
	}
	if in.State.RunID == "" {
		in.State.RunID = in.Job.RunID
	}
	if in.State.CancelRequested || ctx.Err() != nil {
		return e.finish(ctx, in, finishInstructions(RunCancelled, StopCancelled)[0])
	}
	instructions, err := e.brain.Decide(in.Job.Phase, in.Job.Payload, in.State)
	if err != nil {
		return StepResult{}, err
	}
	in.State = rememberHarnessFingerprints(in.Job.Phase, in.Job.Payload, in.State)
	if in.Job.Phase == PhaseHumanOverride {
		var override HumanOverridePayload
		_ = json.Unmarshal(in.Job.Payload, &override)
		if override.Action == OverrideAbort {
			e.applyAbortRestore(in.State)
		}
	}
	out := StepResult{State: in.State}
	for _, inst := range instructions {
		switch inst.Type {
		case InstructionCallLLM, InstructionLoadContext:
			out, err = e.callLLM(ctx, in, inst)
		case InstructionCallToolsBatch:
			out, err = e.callToolsBatch(ctx, in, inst)
		case InstructionRequestHumanApprove:
			if kind, ok := approvalKindOf(inst.Payload); ok && kind != ApprovalKindTools {
				out, err = e.requestOverride(ctx, in, inst)
			} else {
				out, err = e.callToolsBatch(ctx, in, inst)
			}
		case InstructionVerify:
			out, err = e.runVerify(ctx, in)
		case InstructionEvaluate:
			out, err = e.runEvaluate(ctx, in)
		case InstructionFinish:
			out, err = e.finish(ctx, in, inst)
		case InstructionCompressContext:
			continue
		default:
			out, err = e.finish(ctx, in, finishInstructions(RunFailed, StopModelError)[0])
		}
		if err != nil {
			return StepResult{}, err
		}
		in.State = out.State
	}
	return out, nil
}

// callLLM 占槽后压缩上下文、调模型，把流式增量写成 Fact，再产出 assistant 消息与下一步。
// 逻辑：校验取消 → 超回合则有副作用先验证否则收束 → CompactIfNeeded + Stream → 收齐文本/工具调用 → 有 Tool 则下一步 llm_result。
// 复审通过后的 wrap-up 回合清空工具表；模型空正文时用改动/验证/复审事实拼装。
func (e *Engine) callLLM(ctx context.Context, in StepInput, _ Instruction) (StepResult, error) {
	if err := ctx.Err(); err != nil {
		return e.finish(ctx, in, finishInstructions(RunCancelled, StopCancelled)[0])
	}
	state := in.State
	hist := in.History
	if hist.Run.ID == "" {
		hist.Run.ID = state.RunID
		hist.Run.SessionID = state.SessionID
		hist.Run.Config = state.Config
	}
	turnID := newEntityID()
	state.TurnID = &turnID
	hist.Turn.ID = turnID
	if hist.Turn.Number <= 0 {
		hist.Turn.Number = 1
	}
	wrapUp := in.Job.Phase == PhaseEvaluateResult || state.WrapUpPending
	if wrapUp {
		state.WrapUpPending = true
		state.Status = RunRunningLLM
	}
	if limit := state.Config.Limits.MaxTurns; limit > 0 && hist.Turn.Number > limit {
		state.ForceFinish = true
		in.State = state
		in.History = hist
		if wrapUp {
			return e.finish(ctx, in, finishInstructions(RunCompleted, StopMaxTurns)[0])
		}
		if state.HadSideEffects {
			return e.runVerify(ctx, in)
		}
		return e.finish(ctx, in, finishInstructions(RunCompleted, StopMaxTurns)[0])
	}

	snapshot, err := Load(ctx, hist)
	if err != nil {
		return StepResult{}, err
	}
	if snapshot.WorkspaceRoot == "" {
		snapshot.WorkspaceRoot = state.WorkspaceRoot
	}
	scope := ResolvePlanScope(state.ActivePlan, hist.Messages)
	state.ActivePlan = scope.ActivePlan
	snapshot.ActivePlan = scope.ActivePlan
	if e.llmGate != nil {
		if err := e.llmGate.Acquire(ctx); err != nil {
			return e.finish(ctx, StepInput{State: state, Job: in.Job}, finishInstructions(RunCancelled, StopCancelled)[0])
		}
		defer e.llmGate.Release()
	}
	snapshot, err = CompactIfNeeded(ctx, Compaction{Run: hist.Run, Turn: hist.Turn, Snapshot: snapshot})
	if err != nil {
		return StepResult{}, err
	}
	chat, err := Build(ctx, Prompt{Run: hist.Run, Turn: hist.Turn, Context: snapshot})
	if err != nil {
		return StepResult{}, err
	}
	if wrapUp {
		chat.Tools = nil
		chat.Messages = append(chat.Messages, Message{Role: RoleDeveloper, Content: EncodeText(wrapUpDeveloperNote)})
	}
	chat.SessionID = state.SessionID
	chat.RunID = state.RunID
	chat.TurnID = turnID
	chat.Dispatcher = e.dispatcher
	chat = applyRequestSeam(ctx, e.dispatcher, chat)

	stream, err := Stream(ctx, chat)
	if err != nil {
		if ctx.Err() != nil || state.CancelRequested {
			return e.finish(ctx, StepInput{State: state, Job: in.Job}, finishInstructions(RunCancelled, StopCancelled)[0])
		}
		return StepResult{}, err
	}
	defer stream.Close()

	msgID := newEntityID()
	_ = e.appendFact(ctx, state.RunID, Fact{
		Type:   EventAssistantStarted,
		TurnID: state.TurnID,
		Payload: MarshalPayload(AssistantStartedPayload{
			MessageID: msgID,
		}),
	})
	for event := range stream.Events() {
		if ctx.Err() != nil {
			return e.finish(ctx, StepInput{State: state, Job: in.Job}, finishInstructions(RunCancelled, StopCancelled)[0])
		}
		switch event.Type {
		case ModelStreamTextDelta, ModelStreamToolDelta:
			_ = e.appendFact(ctx, state.RunID, Fact{
				Type:   EventAssistantDelta,
				TurnID: state.TurnID,
				Payload: MarshalPayload(AssistantDeltaPayload{
					MessageID: msgID,
					Delta:     event.Delta,
				}),
			})
		}
	}
	result, err := stream.Result(ctx)
	if err != nil {
		if ctx.Err() != nil || state.CancelRequested {
			return e.finish(ctx, StepInput{State: state, Job: in.Job}, finishInstructions(RunCancelled, StopCancelled)[0])
		}
		return StepResult{}, err
	}

	assistant := result.Message
	assistant.ID = msgID
	assistant.SessionID = state.SessionID
	assistant.RunID = ptrValue(state.RunID)
	assistant.TurnID = state.TurnID
	if wrapUp {
		result.ToolCalls = nil
		if strings.TrimSpace(DecodeText(assistant.Content)) == "" {
			assistant.Content = EncodeText(composeWrapUpText(e, state, hist.Messages))
		}
	}
	if len(assistant.Content) == 0 {
		assistant.Content = EncodeText("")
	}
	_ = e.appendFact(ctx, state.RunID, Fact{
		Type:   EventAssistantCompleted,
		TurnID: state.TurnID,
		Payload: MarshalPayload(AssistantCompletedPayload{
			MessageID: msgID,
			Text:      DecodeText(assistant.Content),
			ToolCalls: result.ToolCalls,
		}),
	})

	now := time.Now().UTC()
	if state.StartedAt == nil {
		state.StartedAt = &now
	}
	state.Status = RunRunningLLM
	state.StepIndex = in.Job.StepIndex
	if state.StepIndex <= 0 {
		state.StepIndex = 1
	}
	if !wrapUp && len(result.ToolCalls) > 0 {
		state.Checkpoint.Pending = result.ToolCalls
		state.Checkpoint.TurnID = turnID
		state.Checkpoint.Results = nil
		state.Checkpoint.Completed = nil
	}
	return StepResult{
		State:    state,
		Messages: []Message{assistant},
		Next: &StepJob{
			RunID:     state.RunID,
			StepIndex: state.StepIndex + 1,
			Phase:     PhaseLLMResult,
		},
	}, nil
}

// callToolsBatch 按配置派发本批工具。auto 整批、以及任何离开工作区的调用先走独立复审；仍待批则暂停，否则写入 tool 消息并进入 tools_batch_result。
// auto 先复审再发工具事件：通过则当场执行，升级人才发 approval_required。
func (e *Engine) callToolsBatch(ctx context.Context, in StepInput, inst Instruction) (StepResult, error) {
	if err := ctx.Err(); err != nil {
		return e.finish(ctx, in, finishInstructions(RunCancelled, StopCancelled)[0])
	}
	state := in.State
	calls := state.Checkpoint.Pending
	if len(calls) == 0 && len(inst.Payload) > 0 {
		var payload CallToolsBatchPayload
		if err := json.Unmarshal(inst.Payload, &payload); err == nil {
			calls = payload.Calls
		}
	}
	turnID := derefString(state.TurnID)
	if turnID == "" {
		turnID = state.Checkpoint.TurnID
	}
	if turnID == "" {
		turnID = newEntityID()
		state.TurnID = &turnID
	}

	execMode := state.Config.ToolExecutionMode
	if execMode == "" {
		execMode = tool.ExecutionSerial
	}
	failPolicy := state.Config.ToolFailurePolicy
	if failPolicy == "" {
		failPolicy = tool.FailureBestEffort
	}
	maxParallel := state.Config.Limits.MaxParallelTools
	if maxParallel <= 0 {
		maxParallel = 1
	}

	pendingNames := make([]toolNameArg, 0, len(calls))
	for _, call := range calls {
		pendingNames = append(pendingNames, toolNameArg{Name: call.Name})
	}
	var snapFacts []Fact
	state, snapFacts = e.captureSnapshotIfNeeded(state, pendingNames)

	liveHook := e.toolEventHook(state)
	scope := ResolvePlanScope(state.ActivePlan, in.History.Messages)
	state.ActivePlan = scope.ActivePlan
	inv := tool.Invocation{
		SessionID:       state.SessionID,
		RunID:           state.RunID,
		TurnID:          turnID,
		WorkspaceRoot:   state.WorkspaceRoot,
		ActivePlan:      scope.ActivePlan,
		MentionedPlans:  scope.Mentioned,
		AllowListAll:    scope.AllowListAll,
		Calls:           calls,
		Mode:            execMode,
		FailurePolicy:   failPolicy,
		MaxParallel:     maxParallel,
		BoundNames:      state.Config.Profile.Tools.Names,
		Effects:         state.Config.Profile.Tools.Effects,
		Approval:        tool.ApprovalMode(state.Config.Approval),
		Registry:        e.tools,
		ApprovedCallIDs: state.Checkpoint.Approved,
		DeniedCallIDs:   state.Checkpoint.Denied,
		OnEvent:         liveHook,
		Gate:            e.toolGate,
		Dispatcher:      e.dispatcher,
	}
	if state.Config.Approval == ApprovalAuto {
		inv.OnEvent = nil
	}
	out, err := tool.Dispatch(ctx, inv)
	if err != nil {
		if ctx.Err() != nil || state.CancelRequested {
			return e.finish(ctx, StepInput{State: state, Job: in.Job}, finishInstructions(RunCancelled, StopCancelled)[0])
		}
		return StepResult{}, err
	}

	state.StepIndex = in.Job.StepIndex
	if state.StepIndex <= 0 {
		state.StepIndex = 1
	}
	if state.Config.Approval == ApprovalAuto || out.NeedsExternalReview {
		if out.WaitingApproval {
			out, state = e.reviewThenDispatch(ctx, state, inv, out, liveHook)
		} else if state.Config.Approval == ApprovalAuto {
			replayDispatchEvents(liveHook, calls, out)
		}
	}
	if out.WaitingApproval {
		approvalID := derefString(state.PendingApproval)
		if approvalID == "" {
			approvalID = newEntityID()
		}
		state.PendingApproval = &approvalID
		state.Status = RunWaitingApproval
		state.Checkpoint.Pending = out.PendingCalls
		if state.Checkpoint.TurnID == "" {
			state.Checkpoint.TurnID = turnID
		}
		// 与 insertPendingApproval 一致：整批复检暂停时，审批行和事件都带上本批全部调用。
		src := out.PendingCalls
		if len(src) == 0 {
			src = out.ApprovalCalls
		}
		toolCalls := make([]ApprovalToolCall, 0, len(src))
		for _, call := range src {
			toolCalls = append(toolCalls, ApprovalToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: call.Arguments,
				Status:    ApprovalPending,
			})
		}
		facts := append([]Fact{}, snapFacts...)
		facts = append(facts, Fact{
			Type:   EventApprovalRequired,
			TurnID: state.TurnID,
			Payload: MarshalPayload(ApprovalRequiredPayload{
				ApprovalID: approvalID,
				ToolCalls:  toolCalls,
			}),
		})
		return StepResult{State: state, Facts: facts}, nil
	}

	completed := make([]string, 0, len(out.Results))
	messages := make([]Message, 0, len(out.Results))
	for _, result := range out.Results {
		if result.CallID != "" {
			completed = append(completed, result.CallID)
		}
		content := EncodeToolResult(result.CallID, result.Output)
		if !result.Success {
			content = EncodeToolError(result.CallID, result.Error)
		}
		messages = append(messages, Message{
			ID:        newEntityID(),
			SessionID: state.SessionID,
			RunID:     ptrValue(state.RunID),
			TurnID:    state.TurnID,
			Role:      RoleTool,
			Content:   content,
		})
	}
	state.Status = RunExecutingTools
	state.Checkpoint.Results = out.Results
	state.Checkpoint.Completed = completed
	state.Checkpoint.Pending = nil
	state.PendingApproval = nil
	state.ActivePlan = BindActivePlan(state.ActivePlan, out.Results)
	if !state.HadSideEffects {
		for _, result := range out.Results {
			if !result.Success || !isMutatingTool(result.Name) {
				continue
			}
			if result.Name == "write" || result.Name == "edit" || e.workspaceChanged(state) {
				state.HadSideEffects = true
				break
			}
		}
	}
	return StepResult{
		State:    state,
		Facts:    snapFacts,
		Messages: messages,
		Next: &StepJob{
			RunID:     state.RunID,
			StepIndex: state.StepIndex + 1,
			Phase:     PhaseToolsBatchResult,
		},
	}, nil
}

// finish 按指令把 Run 收成终态，并产出对应的终态事件。
func (e *Engine) finish(_ context.Context, in StepInput, inst Instruction) (StepResult, error) {
	state := in.State
	payload := FinishPayload{Status: RunCompleted, Reason: StopCompleted}
	if len(inst.Payload) > 0 {
		_ = json.Unmarshal(inst.Payload, &payload)
	}
	if state.CancelRequested && payload.Status != RunCancelled {
		payload.Status = RunCancelled
		payload.Reason = StopCancelled
	}
	if payload.Status == "" {
		payload.Status = RunCompleted
	}
	if payload.Reason == "" {
		payload.Reason = StopCompleted
	}
	now := time.Now().UTC()
	state.Status = payload.Status
	state.StopReason = &payload.Reason
	state.FinishedAt = &now
	state.StepIndex = in.Job.StepIndex
	if state.StepIndex <= 0 {
		state.StepIndex = 1
	}
	var messages []Message
	facts := []Fact{{
		Type:    TerminalEvent(payload.Status),
		TurnID:  state.TurnID,
		Payload: MarshalPayload(RunTerminalPayload{Status: payload.Status, StopReason: &payload.Reason}),
	}}
	if shouldComposeWrapUp(state, payload) {
		text := composeWrapUpText(e, state, in.History.Messages)
		msg, wrapFacts := wrapUpAssistantFacts(state, text)
		messages = append(messages, msg)
		facts = append(wrapFacts, facts...)
	}
	return StepResult{
		State:    state,
		Facts:    facts,
		Messages: messages,
	}, nil
}

// reviewThenDispatch 用独立复审模型裁定待批调用。auto 整批走这里；离开工作区的调用任何模式都走这里。复审失败或说不清则仍等待人批，不发工具事件。
func (e *Engine) reviewThenDispatch(ctx context.Context, state AgentState, inv tool.Invocation, out tool.DispatchResult, liveHook tool.DispatchHook) (tool.DispatchResult, AgentState) {
	src := out.PendingCalls
	if len(src) == 0 {
		src = out.ApprovalCalls
	}
	calls := make([]ApprovalToolCall, 0, len(src))
	for _, call := range src {
		calls = append(calls, ApprovalToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
			Status:    ApprovalPending,
		})
	}
	result, err := Review(ctx, FallbackModel(state.Config.EvaluatorModel, state.Config.Model), calls)
	if err != nil || result.Escalate || len(result.Decisions) == 0 {
		return out, state
	}
	for _, item := range result.Decisions {
		switch item.Status {
		case ApprovalApproved:
			state.Checkpoint.Approved = appendUniqueID(state.Checkpoint.Approved, item.ToolCallID)
		case ApprovalDenied:
			state.Checkpoint.Denied = appendUniqueID(state.Checkpoint.Denied, item.ToolCallID)
		}
	}
	inv.ApprovedCallIDs = state.Checkpoint.Approved
	inv.DeniedCallIDs = state.Checkpoint.Denied
	inv.OnEvent = liveHook
	second, err := tool.Dispatch(ctx, inv)
	if err != nil {
		return out, state
	}
	return second, state
}

// replayDispatchEvents 把 auto 首趟静默执行的结果补成工具事件，供前端显示。
func replayDispatchEvents(hook tool.DispatchHook, calls []tool.Call, out tool.DispatchResult) {
	if hook == nil {
		return
	}
	byID := make(map[string]tool.Result, len(out.Results))
	for _, result := range out.Results {
		byID[result.CallID] = result
	}
	for _, call := range calls {
		attempt := max(1, call.Attempt)
		hook("call_started", call, attempt, nil)
		result, ok := byID[call.ID]
		if !ok {
			continue
		}
		copied := result
		hook("execution_started", call, attempt, nil)
		hook("execution_result", call, attempt, &copied)
	}
}

// appendUniqueID 按出现顺序追加非空且未出现过的 id。
func appendUniqueID(ids []string, id string) []string {
	if id == "" {
		return ids
	}
	for _, item := range ids {
		if item == id {
			return ids
		}
	}
	return append(ids, id)
}

// toolEventHook 把工具派发过程中的进度转成 Fact 写入。
func (e *Engine) toolEventHook(state AgentState) tool.DispatchHook {
	return func(kind string, call tool.Call, attempt int, result *tool.Result) {
		eventType := EventToolExecutionStarted
		switch kind {
		case "approval_required":
			return
		case "call_started":
			eventType = EventToolCallStarted
		case "execution_retry":
			eventType = EventToolExecutionRetry
		case "execution_result":
			eventType = EventToolExecutionResult
		case "execution_started":
			eventType = EventToolExecutionStarted
		}
		payload := ToolCallPayload{
			CallID:    call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
			Attempt:   attempt,
		}
		if result != nil {
			payload.Success = &result.Success
			payload.Error = result.Error
			payload.Output = result.Output
		}
		_ = e.appendFact(context.Background(), state.RunID, Fact{
			Type:    eventType,
			TurnID:  state.TurnID,
			Payload: MarshalPayload(payload),
		})
	}
}

// appendFact 通过 FactWriter 落一条步骤内事实；Engine 或 Writer 为空则跳过。
func (e *Engine) appendFact(ctx context.Context, runID string, fact Fact) error {
	if e == nil || e.facts == nil || runID == "" {
		return nil
	}
	return e.facts.Append(ctx, runID, fact)
}

// applyRequestSeam 在发模型前递系统提示和消息；出错或换向都保留原数据。
func applyRequestSeam(ctx context.Context, d seam.Dispatcher, chat Chat) Chat {
	ev, err := seam.Dispatch(ctx, d, seam.Envelope{
		Type:      seam.TypeRequest,
		SessionID: chat.SessionID,
		RunID:     chat.RunID,
		TurnID:    chat.TurnID,
		Payload: MarshalPayload(RequestPayload{
			SystemPrompt: chat.SystemPrompt,
			Messages:     chat.Messages,
		}),
	})
	if err != nil {
		slog.Warn("agent/request dispatch failed", "run_id", chat.RunID, "error", err)
		return chat
	}
	if ev.Type != seam.TypeRequest || len(ev.Payload) == 0 {
		return chat
	}
	var payload RequestPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return chat
	}
	chat.SystemPrompt = payload.SystemPrompt
	chat.Messages = payload.Messages
	return chat
}

// newEntityID 生成去掉连字符的 UUID，用作消息/Turn 等实体 id。
func newEntityID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// derefString 解引用字符串指针，nil 返回空串。
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ptrValue 把非空字符串转成指针，空串返回 nil。
func ptrValue(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
