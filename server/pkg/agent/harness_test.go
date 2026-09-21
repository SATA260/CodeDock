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

// TestBrainVerifyAndEvaluateGates 覆盖验证、复审和人工裁决的大脑分叉。
func TestBrainVerifyAndEvaluateGates(t *testing.T) {
	brain := &Brain{}
	passed, _ := json.Marshal(VerifyResult{Status: VerifyStatusPassed})
	failed, _ := json.Marshal(VerifyResult{Status: VerifyStatusFailed, Fingerprint: "fp-1"})
	loop, _ := json.Marshal(VerifyResult{Status: VerifyStatusFailed, Fingerprint: "fp-1"})
	evalPass, _ := json.Marshal(EvaluationResult{Verdict: VerdictPass})
	evalWork, _ := json.Marshal(EvaluationResult{Verdict: VerdictNeedsWork, Summary: "弱化断言"})
	accept, _ := json.Marshal(HumanOverridePayload{Action: OverrideAccept})
	retry, _ := json.Marshal(HumanOverridePayload{Action: OverrideRetry})
	abort, _ := json.Marshal(HumanOverridePayload{Action: OverrideAbort})

	got, err := brain.Decide(PhaseVerifyResult, passed, AgentState{})
	if err != nil || len(got) != 1 || got[0].Type != InstructionEvaluate {
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
	got, err = brain.Decide(PhaseEvaluateResult, evalPass, AgentState{})
	if err != nil || len(got) != 1 || got[0].Type != InstructionFinish {
		t.Fatalf("eval pass recovered: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseEvaluateResult, evalPass, AgentState{WrapUpPending: true})
	if err != nil || len(got) != 1 || got[0].Type != InstructionCallLLM {
		t.Fatalf("eval pass wrap-up: %+v %v", got, err)
	}
	evalSkip, _ := json.Marshal(EvaluationResult{Verdict: VerdictPass, Skipped: true})
	got, err = brain.Decide(PhaseEvaluateResult, evalSkip, AgentState{WrapUpPending: true})
	if err != nil || len(got) != 1 || got[0].Type != InstructionFinish {
		t.Fatalf("eval skip: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseEvaluateResult, evalPass, AgentState{ForceFinish: true})
	if err != nil || len(got) != 1 || got[0].Type != InstructionFinish {
		t.Fatalf("eval pass at max turns: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseEvaluateResult, evalWork, AgentState{EvaluateRound: 1})
	if err != nil || len(got) != 1 || got[0].Type != InstructionCallLLM {
		t.Fatalf("eval work: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseEvaluateResult, evalWork, AgentState{ForceFinish: true, EvaluateRound: 1})
	if err != nil || len(got) != 1 || got[0].Type != InstructionRequestHumanApprove {
		t.Fatalf("eval work at max turns: %+v %v", got, err)
	}
	got, err = brain.Decide(PhaseEvaluateResult, evalWork, AgentState{EvaluateRound: 2})
	if err != nil || len(got) != 1 || got[0].Type != InstructionRequestHumanApprove {
		t.Fatalf("eval cap: %+v %v", got, err)
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

// TestIsLoopingAndSanitizeDiff 校验指纹熔断与 diff 瘦身。
func TestIsLoopingAndSanitizeDiff(t *testing.T) {
	if !IsLooping("a", "a", 1, 3) || !IsLooping("b", "c", 3, 3) || IsLooping("b", "c", 1, 3) {
		t.Fatal("looping rules")
	}
	raw := "diff --git a/go.sum b/go.sum\n--- a/go.sum\n+++ b/go.sum\n+abc\n" +
		"diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n+func main() {}\n"
	got, err := SanitizeDiff(raw, 1024)
	if err != nil || strings.Contains(got, "go.sum") || !strings.Contains(got, "main.go") {
		t.Fatalf("sanitize=%q err=%v", got, err)
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

// TestEvaluateFakePassAndReuse 无脚本时放行，相同 diff 复用驳回；空材料直接通过。
func TestEvaluateFakePassAndReuse(t *testing.T) {
	got, err := EvaluateRun(context.Background(), ModelConfig{Provider: "fake", Model: "fake"}, "diff", nil, "", "")
	if err != nil || got.Verdict != VerdictPass {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	reuse, err := EvaluateRun(context.Background(), ModelConfig{Provider: "fake"}, "same", nil, "", DiffFingerprint("same"))
	if err != nil || reuse.Verdict != VerdictNeedsWork {
		t.Fatalf("reuse=%+v err=%v", reuse, err)
	}
	empty, err := EvaluateRun(context.Background(), ModelConfig{Provider: "openai", Model: "x"}, "", nil, "", "")
	if err != nil || empty.Verdict != VerdictPass || !strings.Contains(empty.Summary, "跳过复审") {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	withPlan, err := EvaluateRun(context.Background(), ModelConfig{Provider: "openai", Model: "x"}, "", []PlanItem{{ID: "a", Description: "写计时器"}}, "", "")
	if err != nil || withPlan.Verdict != VerdictPass {
		t.Fatalf("empty diff with plan must skip, got=%+v err=%v", withPlan, err)
	}
}

// TestParseEvaluationContentAcceptsAliases 复审正文用别名或只有 issues 时也能读出来。
func TestParseEvaluationContentAcceptsAliases(t *testing.T) {
	got, ok := parseEvaluationContent(`{"verdict":"通过","summary":"可以"}`)
	if !ok || got.Verdict != VerdictPass {
		t.Fatalf("pass alias=%+v ok=%v", got, ok)
	}
	got, ok = parseEvaluationContent(`{"verdict":"needs-work","summary":"弱断言","issues":[]}`)
	if !ok || got.Verdict != VerdictNeedsWork || len(got.Issues) != 1 {
		t.Fatalf("needs_work without issues=%+v ok=%v", got, ok)
	}
	got, ok = parseEvaluationContent(`{"issues":[{"category":"logic_defect","reason":"错了"}]}`)
	if !ok || got.Verdict != VerdictNeedsWork {
		t.Fatalf("issues only=%+v ok=%v", got, ok)
	}
}

// TestResolveEmptyEvaluationContent 空复审正文且无验收项时放行，有验收项仍说不清。
func TestResolveEmptyEvaluationContent(t *testing.T) {
	got, ok := resolveEvaluationContent("", nil)
	if !ok || got.Verdict != VerdictPass {
		t.Fatalf("empty without plan=%+v ok=%v", got, ok)
	}
	if _, ok = resolveEvaluationContent("", []PlanItem{{ID: "a", Description: "写文件"}}); ok {
		t.Fatal("empty with plan must stay unclear")
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

// TestLeaveEvaluatingOnPass 复审通过后不能还停在 evaluating。
func TestLeaveEvaluatingOnPass(t *testing.T) {
	state := AgentState{Status: RunEvaluating}
	leaveEvaluating(&state, EvaluationResult{Verdict: VerdictPass})
	if state.Status != RunRunningLLM || !state.WrapUpPending {
		t.Fatalf("pass=%+v", state)
	}
	state = AgentState{Status: RunEvaluating}
	leaveEvaluating(&state, EvaluationResult{Verdict: VerdictPass, Skipped: true})
	if state.Status != RunRunningLLM || state.WrapUpPending {
		t.Fatalf("skip=%+v", state)
	}
	state = AgentState{Status: RunEvaluating}
	leaveEvaluating(&state, EvaluationResult{Verdict: VerdictNeedsWork})
	if state.Status != RunRunningLLM || state.WrapUpPending {
		t.Fatalf("needs_work=%+v", state)
	}
	state = AgentState{Status: RunEvaluating}
	leaveEvaluating(&state, EvaluationResult{Verdict: VerdictEscalate})
	if state.Status != RunEvaluating {
		t.Fatalf("escalate=%+v", state)
	}
}

// TestEngineSkipEvaluateFinishesWithoutWrapUp 无代码改动时复审跳过，不再卡在 evaluating、也不再调收尾模型。
func TestEngineSkipEvaluateFinishesWithoutWrapUp(t *testing.T) {
	engine, facts, _ := testEngine(t)
	engine.SetHarness(stubSnap{oid: "snap"}, nil)
	cfg := DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{
		Verify: []FakeVerifyResult{{Status: VerifyStatusPassed}},
	})})
	state := AgentState{RunID: "run-skip", SessionID: "s", Config: cfg, HadSideEffects: true, WorkspaceRoot: t.TempDir()}
	got, err := engine.Step(context.Background(), StepInput{
		State: state,
		Job:   StepJob{RunID: "run-skip", StepIndex: 4, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = engine.Step(context.Background(), StepInput{State: got.State, Job: *got.Next})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunRunningLLM || got.State.WrapUpPending || got.State.LastEvaluateSummary == "" || got.Next == nil || got.Next.Phase != PhaseEvaluateResult {
		t.Fatalf("skip evaluate=%+v", got)
	}
	got, err = engine.Step(context.Background(), StepInput{State: got.State, Job: *got.Next})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCompleted {
		t.Fatalf("status=%s", got.State.Status)
	}
	if len(got.Messages) != 1 || !strings.Contains(DecodeText(got.Messages[0].Content), "没有可审的代码改动") {
		t.Fatalf("wrap-up=%+v", got.Messages)
	}
	var persisted []EventType
	for _, fact := range facts.facts {
		persisted = append(persisted, fact.Type)
	}
	if !containsEvent(persisted, EventEvaluateResult) {
		t.Fatalf("persisted=%v", persisted)
	}
	var finishTypes []EventType
	for _, fact := range got.Facts {
		finishTypes = append(finishTypes, fact.Type)
	}
	if !containsEvent(finishTypes, EventAssistantCompleted) {
		t.Fatalf("finish facts=%v", finishTypes)
	}
}

// TestEngineVerifyThenEvaluateFinish 改过代码后必须先验证再复审才能收工。
func TestEngineVerifyThenEvaluateFinish(t *testing.T) {
	engine, facts, _ := testEngine(t)
	engine.SetHarness(stubSnap{oid: "snap"}, nil)
	cfg := DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{
		Verify:   []FakeVerifyResult{{Status: VerifyStatusPassed}},
		Evaluate: []FakeEvaluateResult{{Verdict: "pass"}},
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
	if got.State.Status != RunRunningLLM || !got.State.WrapUpPending || got.Next == nil || got.Next.Phase != PhaseEvaluateResult {
		t.Fatalf("evaluate step=%+v", got)
	}
	got, err = engine.Step(context.Background(), StepInput{
		State:   got.State,
		Job:     *got.Next,
		History: fakeHistory("run-v", FakeOptions{Turns: []FakeTurn{{Text: ""}}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.State.WrapUpPending || len(got.Messages) != 1 || !strings.Contains(DecodeText(got.Messages[0].Content), "已完成") {
		t.Fatalf("wrap-up=%+v", got)
	}
	if got.Next == nil || got.Next.Phase != PhaseLLMResult {
		t.Fatalf("wrap-up next=%+v", got.Next)
	}
	got, err = engine.Step(context.Background(), StepInput{State: got.State, Job: *got.Next})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCompleted {
		t.Fatalf("status=%s", got.State.Status)
	}
	var types []EventType
	for _, fact := range facts.facts {
		types = append(types, fact.Type)
	}
	if !containsEvent(types, EventVerifyStarted) || !containsEvent(types, EventEvaluateResult) || !containsEvent(types, EventAssistantCompleted) {
		t.Fatalf("facts=%v", types)
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

// TestEvaluateWorkspaceFeedsDiffAndPlan 复审必须带上快照 diff 与计划验收项。
func TestEvaluateWorkspaceFeedsDiffAndPlan(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := "---\n{\"title\":\"t\",\"items\":[{\"id\":\"a\",\"description\":\"改 server/pkg/agent\",\"verify_cmd\":\"go test ./server/pkg/agent\"}]}\n---\n# plan\n"
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "task.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{}
	snap := &recordingSnap{diff: "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n+func main() {}\n"}
	engine.SetHarness(snap, nil)
	got, err := engine.evaluateWorkspace(context.Background(), AgentState{
		WorkspaceRoot:     dir,
		SnapshotID:        "snap",
		ActivePlan:        "task.md",
		LastVerifySummary: "status=passed",
		Config:            RunConfigSnapshot{Model: ModelConfig{Provider: "fake", Model: "fake"}},
	})
	if err != nil || got.Verdict != VerdictPass {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if snap.got.SnapshotOID != "snap" {
		t.Fatalf("snap=%+v", snap.got)
	}
	items := LoadPlanItems(dir, "task.md")
	if len(items) != 1 || items[0].ID != "a" {
		t.Fatalf("items=%+v", items)
	}
}

// TestEvaluateWorkspaceSkipsWhenNoBaseline 没有快照基线时即使有计划也不要去问复审模型。
func TestEvaluateWorkspaceSkipsWhenNoBaseline(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := "---\n{\"title\":\"t\",\"items\":[{\"id\":\"a\",\"description\":\"改 server/pkg/agent\",\"verify_cmd\":\"go test ./server/pkg/agent\"}]}\n---\n# plan\n"
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "task.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{}
	engine.SetHarness(&recordingSnap{diff: "should-not-read"}, nil)
	got, err := engine.evaluateWorkspace(context.Background(), AgentState{
		WorkspaceRoot: dir,
		ActivePlan:    "task.md",
		Config:        RunConfigSnapshot{Model: ModelConfig{Provider: "openai", Model: "x"}},
	})
	if err != nil || got.Verdict != VerdictPass || !strings.Contains(got.Summary, "跳过复审") {
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
