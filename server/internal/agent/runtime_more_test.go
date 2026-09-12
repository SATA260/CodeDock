package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"codedock/internal/agent/memory"
	cderr "codedock/internal/errors"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
)

// TestRuntimeAccessorsAndSubmitErrors 覆盖访问器、注入提交错误与队列满。
func TestRuntimeAccessorsAndSubmitErrors(t *testing.T) {
	rt, _, ctx := testRuntime(t, false)
	if rt.Worker() == nil || rt.Tools() == nil {
		t.Fatal("expected worker and tools")
	}
	rt.SetModel(pkgagent.ModelConfig{})
	rt.logger()
	if err := rt.worker.Submit(ctx, pkgagent.StepJob{}); !cderr.IsInvalid(err) {
		t.Fatalf("empty run: %v", err)
	}
	rt.worker.InjectSubmitError(fmt.Errorf("boom"))
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: "r1", StepIndex: 1}); err == nil {
		t.Fatal("expected injected error")
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: "r1", StepIndex: 1}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: "r1", StepIndex: 1}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < workerQueueSize+2; i++ {
		_ = rt.worker.Submit(ctx, pkgagent.StepJob{RunID: fmt.Sprintf("full-%d", i), StepIndex: 1})
	}
	rt.EnqueueIndexCompact(memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "x", Kind: memory.KindTopic, Name: "t"})
	rt.WaitIndexCompact()
}

// TestLoadApprovalsCompactionAndRecoverPhases 覆盖装审批、压缩检查点与按状态恢复。
func TestLoadApprovalsCompactionAndRecoverPhases(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "first line\nsecond", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertCompactionCheckpoint(ctx, sqlite.InsertCompactionCheckpointParams{
		ID:           util.NewID(),
		SessionID:    sessionID,
		BaseEventSeq: 0,
		Summary:      "old chat",
		CreatedByRun: runID,
		CreatedAt:    util.FormatTime(util.Now()),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID:         util.NewID(),
		SessionID:  sessionID,
		RunID:      runID,
		ToolCallID: "c1",
		ToolCalls:  `[{"id":"c1","name":"ping","status":"approved"},{"id":"c2","name":"ping","status":"denied"}]`,
		Scope:      string(pkgagent.ApprovalOnce),
		Status:     string(pkgagent.ApprovalApproved),
		ExpiresAt:  util.FormatTime(util.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID:         util.NewID(),
		SessionID:  sessionID,
		RunID:      runID,
		ToolCallID: "c3",
		ToolCalls:  `[{"id":"c3","name":"ping","status":"pending"}]`,
		Scope:      string(pkgagent.ApprovalOnce),
		Status:     string(pkgagent.ApprovalPending),
		ExpiresAt:  util.FormatTime(util.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	state, hist, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if hist.Checkpoint == nil || hist.Checkpoint.Summary != "old chat" {
		t.Fatalf("compaction %+v", hist.Checkpoint)
	}
	if state.PendingApproval == nil {
		t.Fatal("expected pending approval")
	}
	if !containsString(state.Checkpoint.Approved, "c1") || !containsString(state.Checkpoint.Denied, "c2") {
		t.Fatalf("decisions %+v", state.Checkpoint)
	}

	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{Status: string(pkgagent.RunRunningLLM), ID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RecoverActive(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{Status: string(pkgagent.RunExecutingTools), ID: runID}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertRunToolCheckpoint(ctx, sqlite.UpsertRunToolCheckpointParams{
		RunID:          runID,
		TurnID:         "t1",
		CompletedCalls: `["c1"]`,
		PendingCalls:   "[]",
		Results:        `[{"call_id":"c1","name":"ping","success":true}]`,
		ApprovedCalls:  `["c1"]`,
		DeniedCalls:    "[]",
		UpdatedAt:      util.FormatTime(util.Now()),
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RecoverActive(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestCommitMessagesTurnAndFailed 覆盖提交助手消息后失败收束 Turn。
func TestCommitMessagesTurnAndFailed(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, strings.Repeat("你", 240), cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	turnID := util.NewID()
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			SessionID: sessionID,
			RunID:     runID,
			TurnID:    &turnID,
			Status:    pkgagent.RunRunningLLM,
			StepIndex: 1,
			Config:    cfg,
			StartedAt: ptrNow(),
			Checkpoint: pkgagent.ToolCheckpoint{
				TurnID:  turnID,
				Pending: []tool.Call{{ID: "c1", Name: "ping"}},
			},
		},
		Messages: []pkgagent.Message{{
			Role:    pkgagent.RoleAssistant,
			Content: pkgagent.EncodeText("hello"),
			TurnID:  &turnID,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	reason := pkgagent.StopModelError
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			SessionID:  sessionID,
			RunID:      runID,
			TurnID:     &turnID,
			Status:     pkgagent.RunFailed,
			StepIndex:  2,
			Config:     cfg,
			StopReason: &reason,
		},
	}); err != nil {
		t.Fatal(err)
	}
	turn, err := q.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != string(pkgagent.TurnFailed) {
		t.Fatalf("turn status=%s", turn.Status)
	}
	if err := rt.RequestCancel(ctx, runID); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerFailAndCancelAndWait 覆盖模型失败收束与挂起 Run 的 CancelAndWait。
func TestWorkerFailAndCancelAndWait(t *testing.T) {
	rt, q, ctx := testRuntime(t, true)
	sessionID := insertSession(t, q, ctx)
	failCfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{FailTimes: 3, Turns: []pkgagent.FakeTurn{{Text: "x"}}}),
	})
	failID, err := rt.CreateAgentState(ctx, sessionID, "fail", failCfg.Mode, failCfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, failID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: failID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, q, ctx, failID, pkgagent.RunFailed)

	hangCfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Hang: true, Turns: []pkgagent.FakeTurn{{Text: "late"}}}),
	})
	hangID, err := rt.CreateAgentState(ctx, sessionID, "hang", hangCfg.Mode, hangCfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, hangID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: hangID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, q, ctx, hangID, pkgagent.RunRunningLLM, pkgagent.RunQueued, pkgagent.RunLoadingContext)
	if err := rt.RequestCancel(ctx, hangID); err != nil {
		t.Fatal(err)
	}
	rt.Worker().CancelAndWait(hangID)
	waitStatus(t, q, ctx, hangID, pkgagent.RunCancelled)
}

// TestCompactIndexBranches 覆盖目录压缩对缺失、未超限与超限内容的处理。
func TestCompactIndexBranches(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	rt.SetModel(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	rt.EnqueueIndexCompact(memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "missing", Kind: memory.KindIndex, Name: memory.NameIndex})
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{
		Scope:   memory.ScopeUser,
		ScopeID: "short",
		Name:    memory.NameIndex,
		Content: "# ok",
	}); err != nil {
		t.Fatal(err)
	}
	rt.EnqueueIndexCompact(memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "short", Kind: memory.KindIndex, Name: memory.NameIndex})
	over := strings.Repeat("line\n", memory.IndexMaxLines+2)
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{
		Scope:   memory.ScopeUser,
		ScopeID: "clip",
		Name:    memory.NameIndex,
		Content: over,
	}); err != nil {
		t.Fatal(err)
	}
	rt.EnqueueIndexCompact(memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "clip", Kind: memory.KindIndex, Name: memory.NameIndex})
	rt.WaitIndexCompact()
}

// TestCreateClaimValidation 覆盖 Claim/Append/Commit/Cancel/Load 的空参数校验。
func TestCreateClaimValidation(t *testing.T) {
	rt, _, ctx := testRuntime(t, false)
	if _, err := rt.ClaimSession(ctx, "", ""); err == nil {
		t.Fatal("expected claim validation")
	}
	if _, err := rt.AppendFact(ctx, "", pkgagent.Fact{Type: pkgagent.EventRunCreated}); err == nil {
		t.Fatal("expected append validation")
	}
	if err := rt.CommitStep(ctx, "", pkgagent.StepResult{}); err == nil {
		t.Fatal("expected commit validation")
	}
	if err := rt.RequestCancel(ctx, ""); err == nil {
		t.Fatal("expected cancel validation")
	}
	if _, _, err := rt.LoadAgentState(ctx, ""); err == nil {
		t.Fatal("expected load validation")
	}
}

// waitStatus 轮询直到 Run 进入期望状态之一，超时则失败。
func waitStatus(t *testing.T, q *sqlite.Queries, ctx context.Context, runID string, want ...pkgagent.RunStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		row, err := q.GetRun(ctx, runID)
		if err == nil {
			status := pkgagent.RunStatus(row.Status)
			for _, item := range want {
				if status == item {
					return
				}
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	row, _ := q.GetRun(ctx, runID)
	t.Fatalf("run %s status=%s want %v", runID, row.Status, want)
}

// ptrNow 返回当前 UTC 时间的指针，供测试填 StartedAt。
func ptrNow() *time.Time {
	now := time.Now().UTC()
	return &now
}

// TestRecoverPhaseHelpers 覆盖 recoverPhase、turnStatusFor 与摘要截断。
func TestRecoverPhaseHelpers(t *testing.T) {
	if recoverPhase(pkgagent.RunQueued, pkgagent.AgentState{}) != pkgagent.PhaseUserInput {
		t.Fatal("queued")
	}
	if recoverPhase(pkgagent.RunRunningLLM, pkgagent.AgentState{}) != pkgagent.PhaseLLMResult {
		t.Fatal("llm")
	}
	if recoverPhase(pkgagent.RunExecutingTools, pkgagent.AgentState{}) != pkgagent.PhaseLLMResult {
		t.Fatal("tools empty")
	}
	if recoverPhase(pkgagent.RunExecutingTools, pkgagent.AgentState{Checkpoint: pkgagent.ToolCheckpoint{Results: []tool.Result{{CallID: "c"}}}}) != pkgagent.PhaseToolsBatchResult {
		t.Fatal("tools results")
	}
	if turnStatusFor(pkgagent.RunCancelled) != string(pkgagent.TurnCancelled) {
		t.Fatal("cancelled turn")
	}
	if clipSessionSummary("") != "" {
		t.Fatal("empty summary")
	}
	if !strings.HasPrefix(clipSessionSummary("a\nb"), "a") {
		t.Fatal("first line")
	}
}

// TestWorkerExecuteCancelAndMiss 覆盖重复领取、缺失 Run 与跳过执行后取消。
func TestWorkerExecuteCancelAndMiss(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "x"}}}),
	})
	runID, err := rt.CreateAgentState(ctx, sessionID, "exec", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	ok, _ := rt.TryClaimStep(ctx, runID, 1)
	if !ok {
		t.Fatal("claim")
	}
	rt.worker.execute(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput})
	rt.releaseStep(runID, 1)

	rt.worker.execute(ctx, pkgagent.StepJob{RunID: "missing", StepIndex: 1, Phase: pkgagent.PhaseUserInput})

	run2, err := rt.CreateAgentState(ctx, sessionID, "skip me", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt.worker.Cancel(run2)
	rt.worker.execute(ctx, pkgagent.StepJob{RunID: run2, StepIndex: 1, Phase: pkgagent.PhaseUserInput})
	row, err := q.GetRun(ctx, run2)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(pkgagent.RunCancelled) {
		t.Fatalf("skipped execute status=%s", row.Status)
	}
}

// TestTxQueriesAndNilAccessors 覆盖事务内 Queries 与空 Runtime 访问器。
func TestTxQueriesAndNilAccessors(t *testing.T) {
	rt, _, ctx := testRuntime(t, false)
	if err := rt.db.WithTx(ctx, func(ctx context.Context) error {
		if rt.q(ctx) == nil {
			t.Fatal("expected tx queries")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var empty *Runtime
	empty.logger()
	empty.SetModel(pkgagent.ModelConfig{Provider: "x"})
	empty.WaitIndexCompact()
	empty.EnqueueIndexCompact(memory.TextMemoryKey{})
	if empty.Worker() != nil || empty.Tools() != nil {
		t.Fatal("nil accessors")
	}
}

// TestRequestCancelRunning 覆盖取消运行中 Run 只标 cancel_requested。
func TestRequestCancelRunning(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "running", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{Status: string(pkgagent.RunRunningLLM), ID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RequestCancel(ctx, runID); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if row.CancelRequested == 0 || row.Status != string(pkgagent.RunRunningLLM) {
		t.Fatalf("running cancel %+v", row)
	}
	sess, err := q.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ActiveRunID.String != runID {
		t.Fatalf("should keep active %s", sess.ActiveRunID.String)
	}
}

// TestMapHelpers 覆盖时间/JSON/审批映射等纯函数。
func TestMapHelpers(t *testing.T) {
	if !parseTime("bad").IsZero() {
		t.Fatal("bad time")
	}
	if ptrTime(nullString("")) != nil {
		t.Fatal("empty ptr time")
	}
	if boolInt(false) != 0 || boolInt(true) != 1 {
		t.Fatal("bool int")
	}
	if marshalJSON(func() {}) != "[]" {
		t.Fatal("marshal fallback")
	}
	if deref(nil) != "" {
		t.Fatal("deref nil")
	}
	_ = mapApproval(sqlite.Approval{ToolCallID: "c1", Status: "pending"})
	_ = mapMessage(sqlite.Message{Content: `{"text":"x"}`, Attachments: nullString(`[{"id":"a"}]`), ToolCalls: nullString(`[{"id":"c"}]`)})
	if wrapDB(nil) != nil {
		t.Fatal("wrap nil")
	}
}
