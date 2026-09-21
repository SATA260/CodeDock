package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codedock/pkg/agent/tool"
)

// TestBrainVerifyAndEvaluateGates 覆盖验证和人工裁决的大脑分叉。
func TestBrainVerifyAndEvaluateGates(t *testing.T) {
	brain := &Brain{}
	passed, _ := json.Marshal(VerifyResult{Status: VerifyStatusPassed})
	failed, _ := json.Marshal(VerifyResult{Status: VerifyStatusFailed, Fingerprint: "fp-1"})
	loop, _ := json.Marshal(VerifyResult{Status: VerifyStatusFailed, Fingerprint: "fp-1"})
	accept, _ := json.Marshal(HumanOverridePayload{Action: OverrideAccept})
	retry, _ := json.Marshal(HumanOverridePayload{Action: OverrideRetry})
	abort, _ := json.Marshal(HumanOverridePayload{Action: OverrideAbort})

	got, err := brain.Decide(PhaseVerifyResult, passed, AgentState{})
	if err != nil || len(got) != 1 || got[0].Type != InstructionFinish {
		t.Fatalf("passed verify: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseVerifyResult, failed, AgentState{VerifyRound: 1})
	if err != nil || len(got) != 1 || got[0].Type != InstructionCallLLM {
		t.Fatalf("failed verify: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseVerifyResult, failed, AgentState{ForceFinish: true, VerifyRound: 1})
	if err != nil || len(got) != 1 || got[0].Type != InstructionRequestHumanApprove {
		t.Fatalf("failed verify at max turns: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseVerifyResult, loop, AgentState{VerifyRound: 2, LastVerifyFingerprint: "fp-1"})
	if err != nil || len(got) != 1 || got[0].Type != InstructionRequestHumanApprove {
		t.Fatalf("loop verify: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseEvaluateResult, nil, AgentState{})
	if err != nil || len(got) != 1 || got[0].Type != InstructionFinish {
		t.Fatalf("legacy evaluate: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseHumanOverride, accept, AgentState{})
	if err != nil || finishReason(got) != StopAcceptedByUser {
		t.Fatalf("accept: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseHumanOverride, retry, AgentState{})
	if err != nil || len(got) != 1 || got[0].Type != InstructionCallLLM {
		t.Fatalf("retry: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseHumanOverride, abort, AgentState{})
	if err != nil || finishReason(got) != StopCancelled {
		t.Fatalf("abort: %+v %v", got, err)
	}
}

// finishReason 读出收束指令上的停止原因。
func finishReason(got []Instruction) StopReason {
	if len(got) != 1 || got[0].Type != InstructionFinish {
		return ""
	}
	var payload FinishPayload
	_ = json.Unmarshal(got[0].Payload, &payload)
	return payload.Reason
}

// TestIsLoopingAndCoverageNote 校验指纹熔断与覆盖率备注。
func TestIsLoopingAndCoverageNote(t *testing.T) {
	if !IsLooping("a", "a", 1, 3) || !IsLooping("b", "c", 3, 3) || IsLooping("b", "c", 1, 3) {
		t.Fatal("looping rules")
	}
	if CoverageNote(40, 70) == "" || CoverageNote(90, 70) == "" {
		t.Fatal("coverage note")
	}
}

// TestParseVerifyYAMLCommands 确认嵌套 commands 列表能解析。
func TestParseVerifyYAMLCommands(t *testing.T) {
	file, err := parseVerifyFile(`
rules:
  - scope_glob: "server/**"
    timeout_sec: 180
    commands:
      - go test ./server/pkg/agent
      - go test ./server/internal/agent
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Rules) != 1 || file.Rules[0].ScopeGlob != "server/**" || len(file.Rules[0].Commands) != 2 {
		t.Fatalf("rules=%+v", file.Rules)
	}
}

// TestCheckWorkspaceSkipsMissingFile 没有 verify.yaml 时按通过处理。
func TestCheckWorkspaceSkipsMissingFile(t *testing.T) {
	got, err := CheckWorkspace(context.Background(), t.TempDir(), nil, 1, "", nil)
	if err != nil || !got.Skipped || got.Status != VerifyStatusPassed {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// TestCheckWorkspaceRunsActivePlanCmd 绑定计划的 verify_cmd 必须进入收尾验证。
func TestCheckWorkspaceRunsActivePlanCmd(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\n{\"title\":\"fail\",\"items\":[{\"id\":\"F1\",\"description\":\"must fail\",\"verify_cmd\":\"false\"}]}\n---\n# fail\n"
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "task.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := CheckWorkspacePlan(context.Background(), dir, "task.md", nil, 1, "", nil)
	if err != nil || got.Skipped || got.Status != VerifyStatusFailed || got.FailedCommand != "false" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// TestCheckNewTestsFailOnBase 断言新测试在旧代码上变绿会被拒绝。
func TestCheckNewTestsFailOnBase(t *testing.T) {
	err := CheckNewTestsFailOnBase(context.Background(), t.TempDir(), []string{"pkg/foo_test.go"}, func(_ context.Context, _, command string, _ time.Duration) (int, string, error) {
		if !strings.Contains(command, "go test") {
			t.Fatalf("cmd=%q", command)
		}
		return 0, "ok", nil
	})
	if err == nil || !strings.Contains(err.Error(), "没有覆盖本次改动") {
		t.Fatalf("want red-on-base error, got %v", err)
	}
	if err := CheckNewTestsFailOnBase(context.Background(), t.TempDir(), []string{"pkg/foo_test.go"}, func(_ context.Context, _, _ string, _ time.Duration) (int, string, error) {
		return 1, "FAIL", nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPlanContractOnlyAddAndPass 验收清单只增不删，打勾必须有证据。
func TestPlanContractOnlyAddAndPass(t *testing.T) {
	raw := "---\n{\"title\":\"t\",\"items\":[{\"id\":\"a\",\"description\":\"改 server/pkg/agent\",\"verify_cmd\":\"go test ./server/pkg/agent\"}]}\n---\n# plan\n"
	first, err := ValidatePlan(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	weak := "---\n{\"title\":\"t\",\"items\":[]}\n---\n# plan\n"
	if _, err := ValidatePlan(weak, &first); err == nil {
		t.Fatal("deleted items should fail")
	}
	bad := "---\n{\"title\":\"t\",\"items\":[{\"id\":\"a\",\"description\":\"改 server/pkg/agent\",\"verify_cmd\":\"lint\"}]}\n---\n"
	if _, err := ValidatePlan(bad, nil); err == nil {
		t.Fatal("backend item needs test command")
	}
	passed, err := MarkPassed(first, "a", "go test ./server/pkg/agent")
	if err != nil || !passed.Items[0].Passes {
		t.Fatalf("pass=%+v err=%v", passed, err)
	}
}

// TestExploreRequiresQuery 探索入参为空时必须说清失败。
func TestExploreRequiresQuery(t *testing.T) {
	out, err := Explore(context.Background(), ExploreRequest{Registry: tool.NewRegistry()})
	if err != nil || out.Error == "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

type stubSnap struct {
	oid string
}

// Take 返回固定快照号。
func (s stubSnap) Take(_, _ string) (WorkspaceSnapshot, error) {
	return WorkspaceSnapshot{SnapshotOID: s.oid, Head: "HEAD"}, nil
}

// Restore 测试里不改磁盘。
func (stubSnap) Restore(string, WorkspaceSnapshot, RestoreMode) error { return nil }

// ChangedFiles 测试里没有改动文件。
func (stubSnap) ChangedFiles(string, WorkspaceSnapshot) ([]string, error) { return nil, nil }

// Diff 测试里没有 diff。
func (stubSnap) Diff(string, WorkspaceSnapshot) (string, error) { return "", nil }

// TestEngineVerifyThenFinish 改过代码后必须先验证才能收工。
func TestEngineVerifyThenFinish(t *testing.T) {
	engine, facts, _ := testEngine(t)
	engine.SetHarness(stubSnap{oid: "snap"}, nil)
	cfg := DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{
		Verify: []FakeVerifyResult{{Status: VerifyStatusPassed}},
	})})
	state := AgentState{RunID: "run-v", SessionID: "s", Config: cfg, HadSideEffects: true, WorkspaceRoot: t.TempDir()}
	got, err := engine.Step(context.Background(), StepInput{
		State: state,
		Job:   StepJob{RunID: "run-v", StepIndex: 4, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunVerifying || got.Next == nil || got.Next.Phase != PhaseVerifyResult {
		t.Fatalf("verify step=%+v", got)
	}
	got, err = engine.Step(context.Background(), StepInput{State: got.State, Job: *got.Next})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCompleted {
		t.Fatalf("status=%s", got.State.Status)
	}
	if len(got.Messages) != 1 || !strings.Contains(DecodeText(got.Messages[0].Content), "已完成") {
		t.Fatalf("wrap-up=%+v", got.Messages)
	}
	var types []EventType
	for _, fact := range facts.facts {
		types = append(types, fact.Type)
	}
	if !containsEvent(types, EventVerifyStarted) {
		t.Fatalf("facts=%v", types)
	}
	var finishTypes []EventType
	for _, fact := range got.Facts {
		finishTypes = append(finishTypes, fact.Type)
	}
	if !containsEvent(finishTypes, EventAssistantCompleted) {
		t.Fatalf("finish facts=%v", finishTypes)
	}
}

// TestHarnessFeedbackIsSystemNote 验证失败回灌不得伪装成孤立 tool 消息。
func TestHarnessFeedbackIsSystemNote(t *testing.T) {
	msg := harnessFeedbackMessage(AgentState{RunID: "r", SessionID: "s"}, "verify", "exit 1", VerifyResult{Status: VerifyStatusFailed})
	if msg.Role != RoleUser || !strings.Contains(DecodeText(msg.Content), "exit 1") {
		t.Fatalf("msg=%+v", msg)
	}
}

// TestEngineVerifyFingerprintLoop 连续相同验证失败会转人工。
func TestEngineVerifyFingerprintLoop(t *testing.T) {
	engine, _, _ := testEngine(t)
	cfg := DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{
		Verify: []FakeVerifyResult{{Status: VerifyStatusFailed, Output: "FAIL same", Fingerprint: "same"}},
	})})
	state := AgentState{RunID: "run-l", Config: cfg, HadSideEffects: true}
	first, err := engine.Step(context.Background(), StepInput{State: state, Job: StepJob{RunID: "run-l", StepIndex: 1, Phase: PhaseLLMResult}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Step(context.Background(), StepInput{State: first.State, Job: *first.Next})
	if err != nil || second.Next == nil || second.Next.Phase != PhaseLLMResult {
		t.Fatalf("first bounce=%+v err=%v", second, err)
	}
	third, err := engine.Step(context.Background(), StepInput{State: second.State, Job: StepJob{RunID: "run-l", StepIndex: 3, Phase: PhaseLLMResult}})
	if err != nil {
		t.Fatal(err)
	}
	fourth, err := engine.Step(context.Background(), StepInput{State: third.State, Job: *third.Next})
	if err != nil || fourth.State.Status != RunWaitingApproval {
		t.Fatalf("loop should escalate: %+v err=%v", fourth, err)
	}
}

// TestEngineVerifyLLMErrorOpensTicket 验证打回后模型调用失败时改开人工单。
func TestEngineVerifyLLMErrorOpensTicket(t *testing.T) {
	engine, _, _ := testEngine(t)
	failed, err := json.Marshal(VerifyResult{Status: VerifyStatusFailed, Output: "FAIL", Fingerprint: "x"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultYoloConfig(ModelConfig{Provider: "openai", Model: "x", Options: mustRaw(map[string]string{"base_url": "http://127.0.0.1:1", "api_key": "k"})})
	got, err := engine.Step(context.Background(), StepInput{
		State: AgentState{RunID: "run-e", Config: cfg, HadSideEffects: true, VerifyRound: 1},
		Job:   StepJob{RunID: "run-e", StepIndex: 2, Phase: PhaseVerifyResult, Payload: failed},
		History: History{
			Run:      Run{ID: "run-e", Config: cfg},
			Messages: []Message{{Role: RoleUser, Content: EncodeText("按 .cursor/task.md 创建 fail.txt。")}},
		},
	})
	if err != nil || got.State.Status != RunWaitingApproval || got.State.ApprovalKind != ApprovalKindVerify {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// TestWorkspaceChangedNeedsSnapshot 没有快照时 bash 不能被当成改过文件。
func TestWorkspaceChangedNeedsSnapshot(t *testing.T) {
	engine := &Engine{}
	engine.SetHarness(&recordingSnap{diff: "x"}, nil)
	if engine.workspaceChanged(AgentState{WorkspaceRoot: t.TempDir()}) {
		t.Fatal("no snapshot must not look dirty")
	}
	if !engine.workspaceChanged(AgentState{WorkspaceRoot: t.TempDir(), SnapshotID: "snap"}) {
		t.Fatal("snapshot with changed files should look dirty")
	}
}

type recordingSnap struct {
	diff string
	got  WorkspaceSnapshot
}

// Take 测试里不拍真实快照。
func (s *recordingSnap) Take(_, _ string) (WorkspaceSnapshot, error) {
	return WorkspaceSnapshot{SnapshotOID: "snap", Head: "HEAD"}, nil
}

// Restore 测试里不改磁盘。
func (*recordingSnap) Restore(string, WorkspaceSnapshot, RestoreMode) error { return nil }

// ChangedFiles 有预设 diff 时假装工作区有改动。
func (s *recordingSnap) ChangedFiles(string, WorkspaceSnapshot) ([]string, error) {
	if s.diff == "" {
		return nil, nil
	}
	return []string{"main.go"}, nil
}

// Diff 记下传入的快照并返回预设 diff。
func (s *recordingSnap) Diff(_ string, snap WorkspaceSnapshot) (string, error) {
	s.got = snap
	return s.diff, nil
}

// containsEvent 判断事件列表是否包含指定类型。
func containsEvent(items []EventType, want EventType) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
