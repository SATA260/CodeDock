package agent

import "encoding/json"

// Brain 根据当前状态决定下一步做什么，自身不执行 I/O。
type Brain struct{}

// Decide 由唤醒原因（phase）和当前状态推断出本步骤应执行的一条主指令。
// 禁止在同一步里串联「模型 → 工具 → 模型」。
func (b *Brain) Decide(phase Phase, payload json.RawMessage, state AgentState) ([]Instruction, error) {
	if state.CancelRequested {
		return finishInstructions(RunCancelled, StopCancelled), nil
	}
	switch phase {
	case PhaseVerifyResult:
		return stopLLMIfTurnsExhausted(decideVerifyResult(payload, state), state, ApprovalKindVerify), nil
	case PhaseEvaluateResult:
		return finishInstructions(RunCompleted, StopCompleted), nil
	case PhaseHumanOverride:
		return decideHumanOverride(payload), nil
	case PhaseHumanAbort:
		return finishInstructions(RunCancelled, StopApprovalDenied), nil
	}
	if state.ForceFinish {
		return finishDueToMaxTurns(state), nil
	}
	switch phase {
	case PhaseUserInput, PhaseInit, PhaseToolsBatchResult, PhaseCompressionResult:
		return []Instruction{{Type: InstructionCallLLM}}, nil
	case PhaseLLMResult:
		if state.WrapUpPending {
			return finishInstructions(RunCompleted, StopCompleted), nil
		}
		if hasPendingTools(state) {
			return []Instruction{{Type: InstructionCallToolsBatch}}, nil
		}
		if state.HadSideEffects {
			return []Instruction{{Type: InstructionVerify}}, nil
		}
		return finishInstructions(RunCompleted, StopCompleted), nil
	case PhaseHumanApproved:
		return []Instruction{{Type: InstructionCallToolsBatch}}, nil
	default:
		return finishInstructions(RunFailed, StopModelError), nil
	}
}

// decideVerifyResult 按验证结论决定收工、打回或开人工单。
func decideVerifyResult(payload json.RawMessage, state AgentState) []Instruction {
	var result VerifyResult
	_ = json.Unmarshal(payload, &result)
	switch result.Status {
	case VerifyStatusPassed:
		return finishInstructions(RunCompleted, StopCompleted)
	case VerifyStatusCannotRun:
		return overrideInstructions(ApprovalKindVerify, "验证环境无法执行")
	default:
		round := state.VerifyRound
		if IsLooping(result.Fingerprint, state.LastVerifyFingerprint, round, MaxVerifyRounds(state.Config.Limits)) {
			return overrideInstructions(ApprovalKindVerify, "验证失败重复或超出次数")
		}
		return []Instruction{{Type: InstructionCallLLM}}
	}
}

// decideHumanOverride 按用户按钮决定收工、重试或回滚取消。
func decideHumanOverride(payload json.RawMessage) []Instruction {
	var item HumanOverridePayload
	_ = json.Unmarshal(payload, &item)
	switch item.Action {
	case OverrideAccept:
		return finishInstructions(RunCompleted, StopAcceptedByUser)
	case OverrideRetry:
		return []Instruction{{Type: InstructionCallLLM}}
	case OverrideAbort:
		return finishInstructions(RunCancelled, StopCancelled)
	default:
		return finishInstructions(RunCancelled, StopCancelled)
	}
}

// overrideInstructions 构造一条开验证/复审人工单的指令。
func overrideInstructions(kind ApprovalKind, reason string) []Instruction {
	return []Instruction{{
		Type:    InstructionRequestHumanApprove,
		Payload: MarshalPayload(ApprovalRequestPayload{Kind: kind, Reason: reason}),
	}}
}

// hasPendingTools 判断当前 checkpoint 是否还有待执行的工具调用。
func hasPendingTools(state AgentState) bool {
	return len(state.Checkpoint.Pending) > 0
}

// stopLLMIfTurnsExhausted 回合用尽后不再自动唤模型；还要改代码则开对应人工单。
func stopLLMIfTurnsExhausted(inst []Instruction, state AgentState, kind ApprovalKind) []Instruction {
	if !state.ForceFinish || len(inst) == 0 || inst[0].Type != InstructionCallLLM {
		return inst
	}
	return overrideInstructions(kind, "回合已用尽，无法再自动修复")
}

// finishDueToMaxTurns 回合用尽：先做完待批工具和收尾验证，没有副作用才直接收束。
func finishDueToMaxTurns(state AgentState) []Instruction {
	if hasPendingTools(state) {
		return []Instruction{{Type: InstructionCallToolsBatch}}
	}
	if state.HadSideEffects {
		return []Instruction{{Type: InstructionVerify}}
	}
	return finishInstructions(RunCompleted, StopMaxTurns)
}

// finishInstructions 构造一条收束指令。
func finishInstructions(status RunStatus, reason StopReason) []Instruction {
	return []Instruction{{
		Type:    InstructionFinish,
		Payload: MarshalPayload(FinishPayload{Status: status, Reason: reason}),
	}}
}
