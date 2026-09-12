package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"codedock/internal/agent/memory"
	agenttools "codedock/internal/agent/tools"
	"codedock/internal/events"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

// testRuntime 打开内存库并装配 Runtime；start 为真时启动 Worker。
func testRuntime(t *testing.T, start bool) (*Runtime, *sqlite.Queries, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	client, err := db.Open(ctx, db.Config{Engine: db.EngineSQLite, DSN: fmt.Sprintf("file:%s?mode=memory&cache=shared", name)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := db.Migrate(ctx, client.DB()); err != nil {
		t.Fatal(err)
	}
	q := db.SQLiteQueries(client)
	reg := tool.NewRegistry()
	rt := New(client, q, events.New(), reg, nil, agenttools.Ports{})
	if start {
		rt.Start(ctx)
	}
	return rt, q, ctx
}

// insertSession 插入一条测试用 Session 并返回 id。
func insertSession(t *testing.T, q *sqlite.Queries, ctx context.Context) string {
	t.Helper()
	now := util.FormatTime(util.Now())
	row, err := q.InsertSession(ctx, sqlite.InsertSessionParams{
		ID:          util.NewID(),
		TenantID:    "t1",
		UserID:      "u1",
		AgentID:     "default",
		WorkspaceID: "default",
		Status:      string(pkgagent.SessionActive),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return row.ID
}

// TestCreateClaimLoadAppend 覆盖创建、领取、加载状态与追加事件。
func TestCreateClaimLoadAppend(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "hello world", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	state, hist, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pkgagent.RunQueued || state.StepIndex != 0 {
		t.Fatalf("state=%+v", state)
	}
	if hist.Run.TriggerMessageID == "" || len(hist.Messages) != 1 {
		t.Fatalf("history messages=%d trigger=%s", len(hist.Messages), hist.Run.TriggerMessageID)
	}
	claimed, err := rt.ClaimSession(ctx, sessionID, runID)
	if err != nil || !claimed {
		t.Fatalf("claim1 %v %v", claimed, err)
	}
	claimed, err = rt.ClaimSession(ctx, sessionID, runID)
	if err != nil || !claimed {
		t.Fatalf("claim again %v %v", claimed, err)
	}
	run2, err := rt.CreateAgentState(ctx, sessionID, "queued", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = rt.ClaimSession(ctx, sessionID, run2)
	if err != nil || claimed {
		t.Fatalf("should not steal active: claimed=%v err=%v", claimed, err)
	}
	ev, err := rt.AppendFact(ctx, runID, pkgagent.Fact{
		Type:    pkgagent.EventAssistantDelta,
		Payload: pkgagent.MarshalPayload(pkgagent.AssistantDeltaPayload{MessageID: "m1", Delta: pkgagent.EncodeText("x")}),
	})
	if err != nil || ev.Seq == 0 {
		t.Fatalf("append %+v %v", ev, err)
	}
	if err := rt.Append(ctx, runID, pkgagent.Fact{Type: pkgagent.EventAssistantCompleted, Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
}

// TestCommitStepAndCancelQueued 覆盖提交完成与取消 queued Run。
func TestCommitStepAndCancelQueued(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "hi", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	reason := pkgagent.StopCompleted
	now := time.Now().UTC()
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			SessionID:  sessionID,
			RunID:      runID,
			Status:     pkgagent.RunCompleted,
			StepIndex:  1,
			Config:     cfg,
			StopReason: &reason,
			FinishedAt: &now,
		},
	}); err != nil {
		t.Fatal(err)
	}
	state, _, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pkgagent.RunCompleted || state.StepIndex != 1 {
		t.Fatalf("after commit %+v", state)
	}
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{State: state}); err != nil {
		t.Fatal(err)
	}

	run2, err := rt.CreateAgentState(ctx, sessionID, "later", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.RequestCancel(ctx, run2); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetRun(ctx, run2)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(pkgagent.RunCancelled) {
		t.Fatalf("queued cancel status=%s", row.Status)
	}
}

// TestTryClaimStepAndRecover 覆盖步骤互斥领取与 RecoverActive。
func TestTryClaimStepAndRecover(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "recover me", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := rt.TryClaimStep(ctx, runID, 1)
	if err != nil || !ok {
		t.Fatal(err)
	}
	ok, err = rt.TryClaimStep(ctx, runID, 1)
	if err != nil || ok {
		t.Fatal("second claim should fail")
	}
	rt.releaseStep(runID, 1)

	if err := rt.RecoverActive(ctx); err != nil {
		t.Fatal(err)
	}

	waitID, err := rt.CreateAgentState(ctx, sessionID, "wait", pkgagent.ModeAskForApproval, pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{Provider: "fake", Model: "fake"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{
		Status:          string(pkgagent.RunWaitingApproval),
		CancelRequested: 0,
		ID:              waitID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertRunToolCheckpoint(ctx, sqlite.UpsertRunToolCheckpointParams{
		RunID:          waitID,
		TurnID:         "turn-1",
		CompletedCalls: "[]",
		PendingCalls:   `[{"id":"c1","name":"ping"}]`,
		Results:        "[]",
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

// TestRequestCancelWaitingApproval 覆盖取消 waiting_approval 立即终态。
func TestRequestCancelWaitingApproval(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "approve", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{
		Status:          string(pkgagent.RunWaitingApproval),
		CancelRequested: 0,
		ID:              runID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RequestCancel(ctx, runID); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(pkgagent.RunCancelled) {
		t.Fatalf("status=%s", row.Status)
	}
}

// TestCommitWaitingApprovalInsertsApproval 覆盖提交等待审批时写入 approval 与 Turn 状态。
func TestCommitWaitingApprovalInsertsApproval(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "need ping", cfg.Mode, cfg)
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
			Checkpoint: pkgagent.ToolCheckpoint{
				TurnID:  turnID,
				Pending: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	approvalID := util.NewID()
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			SessionID:       sessionID,
			RunID:           runID,
			TurnID:          &turnID,
			Status:          pkgagent.RunWaitingApproval,
			StepIndex:       2,
			Config:          cfg,
			PendingApproval: &approvalID,
			Checkpoint: pkgagent.ToolCheckpoint{
				TurnID:  turnID,
				Pending: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}},
			},
		},
		Facts: []pkgagent.Fact{{
			Type:    pkgagent.EventApprovalRequired,
			Payload: pkgagent.MarshalPayload(pkgagent.ApprovalRequiredPayload{ApprovalID: approvalID}),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetApproval(ctx, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(pkgagent.ApprovalPending) {
		t.Fatalf("approval status=%s", row.Status)
	}
	turn, err := q.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != string(pkgagent.TurnWaitingApproval) {
		t.Fatalf("turn status=%s", turn.Status)
	}
	if turn.FinishedAt.Valid {
		t.Fatalf("waiting approval turn should not set finished_at: %s", turn.FinishedAt.String)
	}
}

// TestEnqueueFillsStepIndex 覆盖 StepIndex 为 0 时按已提交步骤补齐。
func TestEnqueueFillsStepIndex(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "step", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
}

// TestCreateAgentStateValidation 覆盖创建与加载的参数校验。
func TestCreateAgentStateValidation(t *testing.T) {
	rt, _, ctx := testRuntime(t, false)
	if _, err := rt.CreateAgentState(ctx, "", "x", "", pkgagent.RunConfigSnapshot{}); err == nil {
		t.Fatal("expected session id error")
	}
	if _, err := rt.CreateAgentState(ctx, "s", "  ", "", pkgagent.RunConfigSnapshot{}); err == nil {
		t.Fatal("expected content error")
	}
	if _, _, err := rt.LoadAgentState(ctx, "missing"); err == nil {
		t.Fatal("expected missing run")
	}
}

// TestLoadMemoryIndexesAndCompact 覆盖装载冻结目录与超限目录压缩。
func TestLoadMemoryIndexesAndCompact(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{
		Scope:   memory.ScopeUser,
		ScopeID: "u1",
		Name:    memory.NameIndex,
		Content: "# user index",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{
		Scope:   memory.ScopeWorkspace,
		ScopeID: "default",
		Name:    memory.NameIndex,
		Content: "# workspace index",
	}); err != nil {
		t.Fatal(err)
	}
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "with memory", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, hist, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.MemoryIndexes) != 2 {
		t.Fatalf("indexes=%d", len(hist.MemoryIndexes))
	}

	over := strings.Repeat("line\n", memory.IndexMaxLines+2)
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{
		Scope:   memory.ScopeUser,
		ScopeID: "u-compact",
		Name:    memory.NameIndex,
		Content: over,
	}); err != nil {
		t.Fatal(err)
	}
	rt.SetModel(pkgagent.ModelConfig{Provider: "fake", Model: "fake", Options: mustJSON(pkgagent.FakeOptions{IndexCompactSummary: "# short\n"})})
	rt.EnqueueIndexCompact(memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "u-compact", Kind: memory.KindIndex, Name: memory.NameIndex})
	rt.WaitIndexCompact()
	item, err := memory.Get(ctx, q, memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "u-compact", Kind: memory.KindIndex, Name: memory.NameIndex})
	if err != nil {
		t.Fatal(err)
	}
	if memory.IndexOverBudget(item.Content) {
		t.Fatal("compact should shrink index")
	}
}

// TestWorkerSubmitAndCancel 覆盖入队后 Worker 跑完一次文本回复。
func TestWorkerSubmitAndCancel(t *testing.T) {
	rt, q, ctx := testRuntime(t, true)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	runID, err := rt.CreateAgentState(ctx, sessionID, "go", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := rt.ClaimSession(ctx, sessionID, runID)
	if err != nil || !claimed {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		row, err := q.GetRun(ctx, runID)
		if err == nil && pkgagent.IsTerminal(pkgagent.RunStatus(row.Status)) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("run did not finish")
}

// TestNilRuntimeGuards 覆盖空 Runtime 上主要入口的防护。
func TestNilRuntimeGuards(t *testing.T) {
	var rt *Runtime
	if _, err := rt.CreateAgentState(context.Background(), "s", "c", "", pkgagent.RunConfigSnapshot{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := rt.ClaimSession(context.Background(), "s", "r"); err == nil {
		t.Fatal("expected error")
	}
	if err := rt.Enqueue(context.Background(), pkgagent.StepJob{}); err != nil {
		t.Fatal(err)
	}
	ok, _ := rt.TryClaimStep(context.Background(), "r", 1)
	if ok {
		t.Fatal("nil claim")
	}
	rt.releaseStep("r", 1)
	if err := rt.RecoverActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.AppendFact(context.Background(), "r", pkgagent.Fact{}); err == nil {
		t.Fatal("expected append error")
	}
}

// mustJSON 把值序列化为 RawMessage，失败则 panic。
func mustJSON(v any) json.RawMessage {
	body, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return body
}
