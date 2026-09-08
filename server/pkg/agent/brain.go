package agent

import "encoding/json"

// Brain 根据当前状态决定下一步做什么，自身不执行 I/O。
type Brain struct{}

// Decide 由唤醒原因（phase）和当前状态推断出本步骤应执行的一条主指令。
// 禁止在同一步里串联「模型 → 工具 → 模型」。
func (b *Brain) Decide(phase Phase, payload json.RawMessage, state AgentState) ([]Instruction, error) {
	_ = payload
	if state.CancelRequested {
		return finishInstructions(RunCancelled, StopCancelled), nil
	}
	if overMaxTurns(state) || state.ForceFinish {
		return finishInstructions(RunCompleted, StopMaxTurns), nil
	}
	switch phase {
	case PhaseUserInput, PhaseInit, PhaseToolsBatchResult, PhaseCompressionResult:
		return []Instruction{{Type: InstructionCallLLM}}, nil
	case PhaseLLMResult:
		if hasPendingTools(state) {
			return []Instruction{{Type: InstructionCallToolsBatch}}, nil
		}
		return finishInstructions(RunCompleted, StopCompleted), nil
	case PhaseHumanApproved:
		return []Instruction{{Type: InstructionCallToolsBatch}}, nil
	case PhaseHumanAbort:
		return finishInstructions(RunCancelled, StopApprovalDenied), nil
	default:
		return finishInstructions(RunFailed, StopModelError), nil
	}
}

func hasPendingTools(state AgentState) bool {
	return len(state.Checkpoint.Pending) > 0
}

func overMaxTurns(state AgentState) bool {
	limit := state.Config.Limits.MaxTurns
	if limit <= 0 || state.ForceFinish {
		return state.ForceFinish
	}
	return false
}

func finishInstructions(status RunStatus, reason StopReason) []Instruction {
	return []Instruction{{
		Type:    InstructionFinish,
		Payload: MarshalPayload(FinishPayload{Status: status, Reason: reason}),
	}}
}
