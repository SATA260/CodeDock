package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"codedock/pkg/agent/tool"
)

// TestComposeWrapUpTextUsesWritePaths 没有快照时从 write 调用收集路径。
func TestComposeWrapUpTextUsesWritePaths(t *testing.T) {
	state := AgentState{
		LastVerifySummary: "status=passed",
	}
	messages := []Message{{
		Role: RoleAssistant,
		ToolCalls: []tool.Call{{
			Name:      "write",
			Arguments: json.RawMessage(`{"path":"ok.txt"}`),
		}},
	}}
	got := composeWrapUpText(nil, state, messages)
	if !strings.Contains(got, "已完成") || !strings.Contains(got, "ok.txt") || !strings.Contains(got, "status=passed") {
		t.Fatalf("compose=%q", got)
	}
}

// TestShouldComposeWrapUpSkipsCancelAndDone 取消或已写过收尾说明时不再拼装。
func TestShouldComposeWrapUpSkipsCancelAndDone(t *testing.T) {
	if shouldComposeWrapUp(AgentState{HadSideEffects: true}, FinishPayload{Status: RunCancelled, Reason: StopCancelled}) {
		t.Fatal("cancelled")
	}
	if shouldComposeWrapUp(AgentState{HadSideEffects: true, WrapUpPending: true}, FinishPayload{Status: RunCompleted, Reason: StopCompleted}) {
		t.Fatal("already wrapped")
	}
	if !shouldComposeWrapUp(AgentState{HadSideEffects: true}, FinishPayload{Status: RunCompleted, Reason: StopCompleted}) {
		t.Fatal("dirty complete should compose")
	}
}

// TestEngineWrapUpKeepsModelText wrap-up 回合优先用模型正文，并丢掉工具调用。
func TestEngineWrapUpKeepsModelText(t *testing.T) {
	engine, facts, _ := testEngine(t)
	got, err := engine.Step(context.Background(), StepInput{
		State: AgentState{
			RunID:     "run-w",
			SessionID: "s",
			Config: DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{
				Turns: []FakeTurn{{Text: "改了 memo.md，验证已通过。", ToolCalls: []FakeToolCall{{Name: "write"}}}},
			})}),
			HadSideEffects: true,
			WrapUpPending:  true,
		},
		Job:     StepJob{RunID: "run-w", StepIndex: 6, Phase: PhaseUserInput},
		History: fakeHistory("run-w", FakeOptions{Turns: []FakeTurn{{Text: "改了 memo.md，验证已通过。", ToolCalls: []FakeToolCall{{Name: "write"}}}}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.State.WrapUpPending || len(got.State.Checkpoint.Pending) != 0 {
		t.Fatalf("wrap-up leaked tools: %+v", got.State.Checkpoint)
	}
	if len(got.Messages) != 1 || DecodeText(got.Messages[0].Content) != "改了 memo.md，验证已通过。" {
		t.Fatalf("wrap-up text=%+v", got.Messages)
	}
	got, err = engine.Step(context.Background(), StepInput{State: got.State, Job: *got.Next})
	if err != nil || got.State.Status != RunCompleted {
		t.Fatalf("finish=%+v err=%v", got, err)
	}
	var types []EventType
	for _, fact := range facts.facts {
		types = append(types, fact.Type)
	}
	if !containsEvent(types, EventAssistantCompleted) {
		t.Fatalf("facts=%v", types)
	}
}
