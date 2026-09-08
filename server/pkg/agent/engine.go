package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"codedock/pkg/agent/tool"
)

// Engine 执行一步：按 Brain 的指令调用对应执行器，自身不直接写库、不发事件、不调度下一步。
type Engine struct {
	brain *Brain
	facts FactWriter
	tools tool.Registry
}

// NewEngine 创建执行引擎。brain 为空时自动构造一个空 Brain。
func NewEngine(brain *Brain, facts FactWriter, tools tool.Registry) *Engine {
	if brain == nil {
		brain = &Brain{}
	}
	return &Engine{brain: brain, facts: facts, tools: tools}
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
	out := StepResult{State: in.State}
	for _, inst := range instructions {
		switch inst.Type {
		case InstructionCallLLM, InstructionLoadContext:
			out, err = e.callLLM(ctx, in, inst)
		case InstructionCallToolsBatch, InstructionRequestHumanApprove:
			out, err = e.callToolsBatch(ctx, in, inst)
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
	if limit := state.Config.Limits.MaxTurns; limit > 0 && hist.Turn.Number > limit {
		state.ForceFinish = true
		return e.finish(ctx, StepInput{State: state, Job: in.Job, History: hist}, finishInstructions(RunCompleted, StopMaxTurns)[0])
	}

	snapshot, err := Load(ctx, hist)
	if err != nil {
		return StepResult{}, err
	}
	snapshot, err = CompactIfNeeded(ctx, Compaction{Run: hist.Run, Turn: hist.Turn, Snapshot: snapshot})
	if err != nil {
		return StepResult{}, err
	}
	chat, err := Build(ctx, Prompt{Run: hist.Run, Turn: hist.Turn, Context: snapshot})
	if err != nil {
		return StepResult{}, err
	}
	chat.SessionID = state.SessionID
	chat.RunID = state.RunID
	chat.TurnID = turnID

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
	if len(result.ToolCalls) > 0 {
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

	out, err := tool.Dispatch(ctx, tool.Invocation{
		SessionID:        state.SessionID,
		RunID:            state.RunID,
		TurnID:           turnID,
		Calls:            calls,
		Mode:             execMode,
		FailurePolicy:    failPolicy,
		MaxParallel:      maxParallel,
		PermissionPolicy: state.Config.PermissionPolicy,
		ApprovalPolicy:   state.Config.ApprovalPolicy,
		AgentMode:        string(state.Config.Mode),
		Registry:         e.tools,
		ApprovedCallIDs:  state.Checkpoint.Approved,
		DeniedCallIDs:    state.Checkpoint.Denied,
		OnEvent:          e.toolEventHook(state),
	})
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
		toolCalls := make([]ApprovalToolCall, 0, len(out.ApprovalCalls))
		for _, call := range out.ApprovalCalls {
			toolCalls = append(toolCalls, ApprovalToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: call.Arguments,
				Status:    ApprovalPending,
			})
		}
		if len(toolCalls) == 0 {
			for _, call := range out.PendingCalls {
				toolCalls = append(toolCalls, ApprovalToolCall{
					ID:        call.ID,
					Name:      call.Name,
					Arguments: call.Arguments,
					Status:    ApprovalPending,
				})
			}
		}
		return StepResult{
			State: state,
			Facts: []Fact{{
				Type:   EventApprovalRequired,
				TurnID: state.TurnID,
				Payload: MarshalPayload(ApprovalRequiredPayload{
					ApprovalID: approvalID,
					ToolCalls:  toolCalls,
				}),
			}},
		}, nil
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
	return StepResult{
		State:    state,
		Messages: messages,
		Next: &StepJob{
			RunID:     state.RunID,
			StepIndex: state.StepIndex + 1,
			Phase:     PhaseToolsBatchResult,
		},
	}, nil
}

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
	return StepResult{
		State: state,
		Facts: []Fact{{
			Type:    TerminalEvent(payload.Status),
			TurnID:  state.TurnID,
			Payload: MarshalPayload(RunTerminalPayload{Status: payload.Status, StopReason: &payload.Reason}),
		}},
	}, nil
}

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

func (e *Engine) appendFact(ctx context.Context, runID string, fact Fact) error {
	if e == nil || e.facts == nil || runID == "" {
		return nil
	}
	return e.facts.Append(ctx, runID, fact)
}

func newEntityID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func ptrValue(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
