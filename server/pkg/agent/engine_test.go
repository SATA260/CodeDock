package agent

import (
	"context"
	"testing"
)

// TestEngineStepCallsDecide 验证 Engine.Step 会调用 Brain.Decide，并在空实现时返回 AgentState。
func TestEngineStepCallsDecide(t *testing.T) {
	engine := NewEngine(&Brain{})
	got, err := engine.Step(context.Background(), AgentState{RunID: "run-1"}, StepJob{
		RunID:     "run-1",
		StepIndex: 1,
		Phase:     PhaseUserInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.RunID != "run-1" {
		t.Fatalf("state run_id = %q, want run-1", got.State.RunID)
	}
	if got.Next != nil {
		t.Fatal("空 Decide 不应产生下一步")
	}
}
