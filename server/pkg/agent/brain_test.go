package agent

import (
	"encoding/json"
	"testing"

	"codedock/pkg/agent/tool"
)

func TestBrainDecideTable(t *testing.T) {
	brain := &Brain{}
	pending := AgentState{Checkpoint: ToolCheckpoint{Pending: []tool.Call{{ID: "c1", Name: "ping"}}}}
	base := AgentState{}

	tests := []struct {
		name  string
		phase Phase
		state AgentState
		want  InstructionType
		stop  StopReason
	}{
		{name: "cancel", phase: PhaseUserInput, state: AgentState{CancelRequested: true}, want: InstructionFinish, stop: StopCancelled},
		{name: "max_turns", phase: PhaseUserInput, state: AgentState{ForceFinish: true, Config: RunConfigSnapshot{Limits: RunLimits{MaxTurns: 1}}}, want: InstructionFinish, stop: StopMaxTurns},
		{name: "max_turns_dirty", phase: PhaseUserInput, state: AgentState{ForceFinish: true, HadSideEffects: true}, want: InstructionVerify},
		{name: "max_turns_pending", phase: PhaseLLMResult, state: AgentState{ForceFinish: true, Checkpoint: ToolCheckpoint{Pending: []tool.Call{{ID: "c1", Name: "ping"}}}}, want: InstructionCallToolsBatch},
		{name: "max_turns_llm_dirty", phase: PhaseLLMResult, state: AgentState{ForceFinish: true, HadSideEffects: true}, want: InstructionVerify},
		{name: "user_input", phase: PhaseUserInput, state: base, want: InstructionCallLLM},
		{name: "init", phase: PhaseInit, state: base, want: InstructionCallLLM},
		{name: "tools_batch_result", phase: PhaseToolsBatchResult, state: base, want: InstructionCallLLM},
		{name: "compression_result", phase: PhaseCompressionResult, state: base, want: InstructionCallLLM},
		{name: "llm_result_tools", phase: PhaseLLMResult, state: pending, want: InstructionCallToolsBatch},
		{name: "llm_result_text", phase: PhaseLLMResult, state: base, want: InstructionFinish, stop: StopCompleted},
		{name: "llm_result_dirty", phase: PhaseLLMResult, state: AgentState{HadSideEffects: true}, want: InstructionVerify},
		{name: "llm_result_wrapup", phase: PhaseLLMResult, state: AgentState{WrapUpPending: true, HadSideEffects: true, Checkpoint: ToolCheckpoint{Pending: []tool.Call{{ID: "c1", Name: "write"}}}}, want: InstructionFinish, stop: StopCompleted},
		{name: "human_approved", phase: PhaseHumanApproved, state: pending, want: InstructionCallToolsBatch},
		{name: "human_abort", phase: PhaseHumanAbort, state: base, want: InstructionFinish, stop: StopApprovalDenied},
		{name: "error", phase: PhaseError, state: base, want: InstructionFinish, stop: StopModelError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := brain.Decide(tc.phase, json.RawMessage(`{}`), tc.state)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatalf("len=%d want 1", len(got))
			}
			if got[0].Type != tc.want {
				t.Fatalf("type=%s want %s", got[0].Type, tc.want)
			}
			if tc.want == InstructionFinish {
				var payload FinishPayload
				if err := json.Unmarshal(got[0].Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Reason != tc.stop {
					t.Fatalf("reason=%s want %s", payload.Reason, tc.stop)
				}
			}
		})
	}
}
