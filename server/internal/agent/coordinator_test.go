package agent

import (
	"codedock/internal/agent/memory"
	agenttools "codedock/internal/agent/tools"
	cderr "codedock/internal/errors"
	"codedock/internal/events"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
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

func histHasTool(hist pkgagent.History, name string) bool {
	for _, def := range hist.Tools {
		if def.Name == name {
			return true
		}
	}
	return false
}

func toolNames(hist pkgagent.History) []string {
	out := make([]string, 0, len(hist.Tools))
	for _, def := range hist.Tools {
		out = append(out, def.Name)
	}
	return out
}

// TestCreateClaimLoadAppend 覆盖创建、领取、加载状态与追加事件。
func TestCreateClaimLoadAppend(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	if state.WorkspaceRoot != "default" {
		t.Fatalf("workspace root=%q", state.WorkspaceRoot)
	}
	if hist.Run.TriggerMessageID == "" || len(hist.Messages) != 1 {
		t.Fatalf("history messages=%d trigger=%s", len(hist.Messages), hist.Run.TriggerMessageID)
	}
	if hist.Prompt != pkgagent.DefaultSystemPrompt {
		t.Fatalf("base prompt=%q", hist.Prompt)
	}
	if !histHasTool(hist, "write") || !histHasTool(hist, "plan_write") {
		t.Fatalf("model tools should be the full catalog: %v", toolNames(hist))
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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

	waitID, err := rt.CreateAgentState(ctx, sessionID, "wait", pkgagent.WorkAgent, pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"}))
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
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
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

func waitRunStatus(t *testing.T, q *sqlite.Queries, ctx context.Context, runID string, want ...pkgagent.RunStatus) pkgagent.RunStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last pkgagent.RunStatus
	for time.Now().Before(deadline) {
		row, err := q.GetRun(ctx, runID)
		if err == nil {
			last = pkgagent.RunStatus(row.Status)
			if len(want) == 0 && pkgagent.IsTerminal(last) {
				return last
			}
			for _, status := range want {
				if last == status {
					return last
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach %v last=%s", runID, want, last)
	return last
}

func TestPreStepWritesOverlay(t *testing.T) {
	rt, q, ctx := testRuntime(t, true)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	rt.SetDispatcher(seam.Func(func(_ context.Context, ev seam.Envelope) (seam.Envelope, error) {
		if ev.Type == seam.TypePreStep {
			ev.Payload = pkgagent.MarshalPayload(pkgagent.PreStepPayload{
				SystemPrompt: "overlay-prompt",
				Hidden:       []pkgagent.Message{{Role: pkgagent.RoleSystem, Content: pkgagent.EncodeText("hidden-note")}},
			})
		}
		return ev, nil
	}))
	runID, err := rt.CreateAgentState(ctx, sessionID, "go", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	waitRunStatus(t, q, ctx, runID, pkgagent.RunCompleted)
	_, hist, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if hist.Prompt != "overlay-prompt" {
		t.Fatalf("prompt=%q", hist.Prompt)
	}
	if len(hist.Hidden) != 1 || pkgagent.DecodeText(hist.Hidden[0].Content) != "hidden-note" {
		t.Fatalf("hidden=%+v", hist.Hidden)
	}
}

func TestPreStepDoesNotDuplicateHiddenOnReplay(t *testing.T) {
	rt, q, ctx := testRuntime(t, true)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	rt.SetDispatcher(seam.Func(func(_ context.Context, ev seam.Envelope) (seam.Envelope, error) {
		if ev.Type != seam.TypePreStep {
			return ev, nil
		}
		var payload pkgagent.PreStepPayload
		_ = json.Unmarshal(ev.Payload, &payload)
		payload.Hidden = append(payload.Hidden, pkgagent.Message{
			Role:    pkgagent.RoleSystem,
			Content: pkgagent.EncodeText("hidden-note"),
		})
		ev.Payload = pkgagent.MarshalPayload(payload)
		return ev, nil
	}))
	runID, err := rt.CreateAgentState(ctx, sessionID, "go", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	waitRunStatus(t, q, ctx, runID, pkgagent.RunCompleted)
	state, hist, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Hidden) != 1 {
		t.Fatalf("first hidden=%+v", hist.Hidden)
	}
	blocked, replayed, err := rt.worker.applyPreStep(ctx, pkgagent.StepJob{
		RunID:     runID,
		StepIndex: 1,
		Phase:     pkgagent.PhaseUserInput,
	}, state, hist)
	if err != nil || blocked {
		t.Fatalf("replay blocked=%v err=%v", blocked, err)
	}
	if len(replayed.Hidden) != 1 || pkgagent.DecodeText(replayed.Hidden[0].Content) != "hidden-note" {
		t.Fatalf("replay hidden=%+v", replayed.Hidden)
	}
}

func TestPreStepBlockedCancelsRun(t *testing.T) {
	rt, q, ctx := testRuntime(t, true)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	rt.SetDispatcher(seam.Func(func(_ context.Context, ev seam.Envelope) (seam.Envelope, error) {
		if ev.Type == seam.TypePreStep {
			ev.Type = seam.TypeRunBlocked
		}
		return ev, nil
	}))
	runID, err := rt.CreateAgentState(ctx, sessionID, "go", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	if got := waitRunStatus(t, q, ctx, runID, pkgagent.RunCancelled); got != pkgagent.RunCancelled {
		t.Fatalf("status=%s", got)
	}
}

type extraNamer struct {
	seam.Func
	names []string
}

// MethodNames 返回要并进本轮可执行绑定的插件方法名。
func (e extraNamer) MethodNames() []string { return e.names }

type extraEcho struct{}

// Definition 登记 echo 方法，默认 allow，便于测绑定并入。
func (extraEcho) Definition() tool.Definition {
	return tool.Definition{Name: "echo", Prompt: "echo", Permission: tool.Permission{Effect: tool.EffectAllow}}
}

// Execute 占位成功，不读参数。
func (extraEcho) Execute(_ context.Context, input tool.Input) (tool.Result, error) {
	return tool.Result{CallID: input.Call.ID, Name: "echo", Success: true}, nil
}

// TestLoadAgentStateMergesExtraMethods 确认插件方法在注册表里模型可见，并入 Names 后才可执行。
func TestLoadAgentStateMergesExtraMethods(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	if err := rt.Tools().Register(extraEcho{}); err != nil {
		t.Fatal(err)
	}
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "hi", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, hist, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, def := range hist.Tools {
		if def.Name == "echo" {
			found = true
		}
	}
	if !found {
		t.Fatal("registered methods stay in the full tool table")
	}
	if containsString(hist.Run.Config.Profile.Tools.Names, "echo") {
		t.Fatal("echo should not be bound without extra names")
	}
	rt.SetDispatcher(extraNamer{names: []string{"echo"}})
	_, hist, err = rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(hist.Run.Config.Profile.Tools.Names, "echo") {
		t.Fatalf("names=%+v", hist.Run.Config.Profile.Tools.Names)
	}

	askCfg := pkgagent.DefaultRunConfig(pkgagent.WorkAsk, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	askID, err := rt.CreateAgentState(ctx, sessionID, "ask", askCfg.Mode, askCfg)
	if err != nil {
		t.Fatal(err)
	}
	_, askHist, err := rt.LoadAgentState(ctx, askID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(askHist.Run.Config.Profile.Tools.Names, "echo") {
		t.Fatalf("ask names should still include plugin methods: %+v", askHist.Run.Config.Profile.Tools.Names)
	}
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
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	failCfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{FailTimes: 3, Turns: []pkgagent.FakeTurn{{Text: "x"}}}),
	})
	failCfg.RetryPolicy.Model = pkgagent.DefaultRetryConfig()
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

	hangCfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
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
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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

// TestSetConcurrencyAndPersistHelpers 覆盖 SetConcurrency 与 step_job 落库辅助函数的空值/payload 分支。
func TestSetConcurrencyAndPersistHelpers(t *testing.T) {
	ctx := context.Background()
	var nilRT *Runtime
	nilRT.SetConcurrency(1, 1)
	if err := nilRT.persistStepJob(ctx, pkgagent.StepJob{RunID: "r", StepIndex: 1}, stepJobQueued); err != nil {
		t.Fatal(err)
	}
	nilRT.markStepJobStatus(ctx, "r", 1, stepJobDone)
	nilRT.cancelOpenStepJobs(ctx, "r")
	if _, ok, err := nilRT.latestOpenStepJob(ctx, "r"); ok || err != nil {
		t.Fatalf("nil latest: ok=%v err=%v", ok, err)
	}

	bare := &Runtime{}
	if bare.q(ctx) != nil {
		t.Fatal("expected nil queries")
	}
	bare.markStepJobStatus(ctx, "r", 1, stepJobDone)
	bare.cancelOpenStepJobs(ctx, "r")
	if _, ok, err := bare.latestOpenStepJob(ctx, "r"); ok || err != nil {
		t.Fatalf("bare latest: ok=%v err=%v", ok, err)
	}

	rt, q, ctx := testRuntime(t, false)
	rt.SetConcurrency(2, 3)
	rt.SetConcurrency(0, 0)
	rt2 := New(rt.db, q, events.New(), nil, nil, agenttools.Ports{})
	if rt2.Tools() == nil {
		t.Fatal("nil tools should become a registry")
	}

	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "persist helpers", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	job := pkgagent.StepJob{
		RunID:     runID,
		StepIndex: 1,
		Phase:     pkgagent.PhaseUserInput,
		Attempt:   -3,
		Payload:   []byte(`{"k":1}`),
	}
	if err := rt.persistStepJob(ctx, job, stepJobQueued); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetStepJob(ctx, sqlite.GetStepJobParams{RunID: runID, StepIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	if row.Attempt != 0 || row.Payload != `{"k":1}` {
		t.Fatalf("upsert %+v", row)
	}
	mapped := stepJobFromRow(row)
	if string(mapped.Payload) != `{"k":1}` {
		t.Fatalf("payload %s", mapped.Payload)
	}

	rt.markStepJobStatus(ctx, "", 1, stepJobDone)
	rt.markStepJobStatus(ctx, runID, 0, stepJobDone)
	rt.cancelOpenStepJobs(ctx, "")
	if _, ok, err := rt.latestOpenStepJob(ctx, ""); ok || err != nil {
		t.Fatalf("empty latest: ok=%v err=%v", ok, err)
	}
	if _, ok, err := rt.latestOpenStepJob(ctx, "missing-run"); ok || err != nil {
		t.Fatalf("missing latest: ok=%v err=%v", ok, err)
	}
}

// TestWorkerNilGuardsAndBusyCancels 覆盖 Worker 空接收者，以及执行中 Busy 走 cancel 表。
func TestWorkerNilGuardsAndBusyCancels(t *testing.T) {
	var w *Worker
	w.InjectSubmitError(fmt.Errorf("x"))
	if err := w.Submit(context.Background(), pkgagent.StepJob{RunID: "r", StepIndex: 1}); err != nil {
		t.Fatal(err)
	}
	if w.Busy("r") {
		t.Fatal("nil busy")
	}
	w.Cancel("r")

	rt, q, ctx := testRuntime(t, true)
	if rt.worker.Busy("") {
		t.Fatal("empty busy")
	}
	rt.worker.Cancel("")
	rt.worker.CancelAndWait("nobody")

	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Hang: true, Turns: []pkgagent.FakeTurn{{Text: "late"}}}),
	})
	runID, err := rt.CreateAgentState(ctx, sessionID, "hang busy", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rt.worker.mu.Lock()
		_, running := rt.worker.cancels[runID]
		rt.worker.mu.Unlock()
		if running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !rt.worker.Busy(runID) {
		t.Fatal("running should be busy via cancel map")
	}
	if err := rt.RequestCancel(ctx, runID); err != nil {
		t.Fatal(err)
	}
	rt.worker.CancelAndWait(runID)
}

// TestRecoverRunEdgePaths 覆盖 RecoverRun 空 id、终态、已裁决审批和 RecoverActive 失败日志。
func TestRecoverRunEdgePaths(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	if err := rt.RecoverRun(ctx, ""); err == nil {
		t.Fatal("empty run")
	}
	if err := rt.RecoverRun(ctx, "missing"); err == nil {
		t.Fatal("missing run")
	}

	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	doneID, err := rt.CreateAgentState(ctx, sessionID, "done", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{Status: string(pkgagent.RunCompleted), ID: doneID}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RecoverRun(ctx, doneID); err != nil {
		t.Fatal(err)
	}

	waitID, err := rt.CreateAgentState(ctx, sessionID, "wait decided", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, waitID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{Status: string(pkgagent.RunWaitingApproval), ID: waitID}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertRunToolCheckpoint(ctx, sqlite.UpsertRunToolCheckpointParams{
		RunID:          waitID,
		TurnID:         "t1",
		CompletedCalls: "[]",
		PendingCalls:   `[{"id":"c1","name":"ping"}]`,
		Results:        "[]",
		ApprovedCalls:  `["c1"]`,
		DeniedCalls:    "[]",
		UpdatedAt:      util.FormatTime(util.Now()),
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.persistStepJob(ctx, pkgagent.StepJob{
		RunID:     waitID,
		StepIndex: 3,
		Phase:     pkgagent.PhaseLLMResult,
		Payload:   []byte(`{"from":"job"}`),
	}, stepJobQueued); err != nil {
		t.Fatal(err)
	}
	if err := rt.RecoverRun(ctx, waitID); err != nil {
		t.Fatal(err)
	}

	if _, err := q.InsertRun(ctx, sqlite.InsertRunParams{
		ID:               util.NewID(),
		SessionID:        "gone-session",
		TriggerMessageID: "m1",
		Mode:             string(pkgagent.WorkAgent),
		Config:           "{}",
		Status:           string(pkgagent.RunQueued),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertRun(ctx, sqlite.InsertRunParams{
		ID:               util.NewID(),
		SessionID:        "gone-session",
		TriggerMessageID: "m2",
		Mode:             string(pkgagent.WorkAgent),
		Config:           "{}",
		Status:           string(pkgagent.RunWaitingApproval),
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RecoverActive(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&Runtime{}).RecoverActive(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestCreateClaimAndEnqueueEdges 覆盖默认模式、Claim 空 active、Enqueue 补 StepIndex 与 worker 为空。
func TestCreateClaimAndEnqueueEdges(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	runID, err := rt.CreateAgentState(ctx, sessionID, "defaults", "", pkgagent.RunConfigSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	row, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Mode != string(pkgagent.WorkAgent) {
		t.Fatalf("mode=%s", row.Mode)
	}
	if _, err := rt.CreateAgentState(ctx, "missing-session", "x", "", pkgagent.RunConfigSnapshot{Mode: pkgagent.WorkAgent}); err == nil {
		t.Fatal("expected missing session")
	}
	if _, err := rt.ClaimSession(ctx, "missing-session", runID); err == nil {
		t.Fatal("expected claim miss")
	}

	now := util.FormatTime(util.Now())
	emptyActive, err := q.InsertSession(ctx, sqlite.InsertSessionParams{
		ID:          util.NewID(),
		TenantID:    "t1",
		UserID:      "u1",
		AgentID:     "default",
		WorkspaceID: "default",
		Status:      string(pkgagent.SessionActive),
		ActiveRunID: sql.NullString{Valid: true, String: ""},
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := rt.ClaimSession(ctx, emptyActive.ID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("empty active_run_id should not claim")
	}

	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: "missing-run", Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	rt.worker = nil
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 2, Phase: pkgagent.PhaseLLMResult}); err != nil {
		t.Fatal(err)
	}
}

// TestCommitStepCoverageBranches 覆盖 CommitStep 同态、审批插入、非法迁移、终态 Turn 与 Next 入队失败。
func TestCommitStepCoverageBranches(t *testing.T) {
	var nilRT *Runtime
	if err := nilRT.CommitStep(context.Background(), "r", pkgagent.StepResult{}); err == nil {
		t.Fatal("nil commit")
	}
	if err := nilRT.RequestCancel(context.Background(), "r"); err == nil {
		t.Fatal("nil cancel")
	}
	if _, _, err := nilRT.LoadAgentState(context.Background(), "r"); err == nil {
		t.Fatal("nil load")
	}
	if _, _, err := (&Runtime{}).LoadAgentState(context.Background(), "r"); err == nil {
		t.Fatal("empty load")
	}

	rt, q, ctx := testRuntime(t, false)
	if err := rt.CommitStep(ctx, "missing", pkgagent.StepResult{}); err == nil {
		t.Fatal("missing commit")
	}
	if err := rt.RequestCancel(ctx, "missing"); err == nil {
		t.Fatal("missing cancel")
	}

	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "commit branches", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}

	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{Status: pkgagent.RunQueued, StepIndex: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{Status: pkgagent.RunQueued, StepIndex: 1},
	}); err != nil {
		t.Fatal(err)
	}

	turnID := util.NewID()
	approvalID := util.NewID()
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
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
		Messages: []pkgagent.Message{{
			Role:      pkgagent.RoleAssistant,
			ToolCalls: []tool.Call{{ID: "c1", Name: "ping"}},
		}},
		Facts: []pkgagent.Fact{{
			Type:    pkgagent.EventApprovalRequired,
			Payload: pkgagent.MarshalPayload(pkgagent.ApprovalRequiredPayload{ApprovalID: approvalID}),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			RunID:           runID,
			TurnID:          &turnID,
			Status:          pkgagent.RunWaitingApproval,
			StepIndex:       3,
			PendingApproval: &approvalID,
			Checkpoint:      pkgagent.ToolCheckpoint{Pending: []tool.Call{{ID: "c1", Name: "ping"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{Status: pkgagent.RunQueued, StepIndex: 4},
	}); err == nil {
		t.Fatal("expected unreachable transition")
	}

	run2, err := rt.CreateAgentState(ctx, sessionID, "next commit", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	finishTurn := util.NewID()
	reason := pkgagent.StopCompleted
	if err := rt.CommitStep(ctx, run2, pkgagent.StepResult{
		State: pkgagent.AgentState{
			Status:     pkgagent.RunCompleted,
			StepIndex:  1,
			TurnID:     &finishTurn,
			StopReason: &reason,
		},
	}); err != nil {
		t.Fatal(err)
	}

	run3, err := rt.CreateAgentState(ctx, sessionID, "empty approval", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitTurn := util.NewID()
	if err := rt.CommitStep(ctx, run3, pkgagent.StepResult{
		State: pkgagent.AgentState{
			Status:    pkgagent.RunWaitingApproval,
			StepIndex: 1,
			TurnID:    &waitTurn,
			Config:    cfg,
			Checkpoint: pkgagent.ToolCheckpoint{
				Pending: []tool.Call{{ID: "c2", Name: "ping"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	run4, err := rt.CreateAgentState(ctx, sessionID, "next err", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt.worker.InjectSubmitError(fmt.Errorf("submit-fail"))
	if err := rt.CommitStep(ctx, run4, pkgagent.StepResult{
		State: pkgagent.AgentState{Status: pkgagent.RunRunningLLM, StepIndex: 1, StartedAt: ptrNow()},
		Next:  &pkgagent.StepJob{RunID: run4, StepIndex: 2, Phase: pkgagent.PhaseLLMResult},
	}); err == nil {
		t.Fatal("expected next enqueue error")
	}
}

// TestLoadForceFinishPromptAndInfer 覆盖超回合 ForceFinish、默认 Prompt 与按事件推断 StepIndex。
func TestLoadForceFinishPromptAndInfer(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	cfg.Limits.MaxTurns = 1
	cfg.Profile.Prompt.Inline = ""
	runID, err := rt.CreateAgentState(ctx, sessionID, "force", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertTurn(ctx, sqlite.InsertTurnParams{
		ID:     util.NewID(),
		RunID:  runID,
		Number: 1,
		Status: string(pkgagent.TurnCompleted),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.AppendFact(ctx, runID, pkgagent.Fact{
		Type:    pkgagent.EventRunStateChanged,
		Payload: pkgagent.MarshalPayload(pkgagent.RunStateChangedPayload{From: pkgagent.RunQueued, To: pkgagent.RunRunningLLM, Reason: "advance"}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.db.WithTx(ctx, func(ctx context.Context) error {
		_, err := rt.AppendFact(ctx, runID, pkgagent.Fact{Type: pkgagent.EventAssistantDelta, Payload: []byte(`{}`)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID:         util.NewID(),
		SessionID:  sessionID,
		RunID:      "other-run",
		ToolCallID: "cx",
		ToolCalls:  `[{"id":"cx","name":"ping","status":"approved"}]`,
		Scope:      string(pkgagent.ApprovalOnce),
		Status:     string(pkgagent.ApprovalApproved),
		ExpiresAt:  util.FormatTime(util.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	state, hist, err := rt.LoadAgentState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.ForceFinish {
		t.Fatal("expected force finish")
	}
	if hist.Prompt == "" || !strings.Contains(hist.Prompt, "CodeDock") {
		t.Fatalf("prompt=%q", hist.Prompt)
	}
}

// TestRuntimeHelperEdges 覆盖 publish、索引与目录压缩的边界路径。
func TestRuntimeHelperEdges(t *testing.T) {
	empty := &Runtime{}
	ok, _ := empty.TryClaimStep(context.Background(), "r", 1)
	if !ok {
		t.Fatal("empty claim should init map")
	}
	empty.publish(pkgagent.AgentEvent{EventID: "e1"})
	empty.indexPersisted(context.Background(), "", pkgagent.Message{ID: "m"})
	empty.indexPersisted(context.Background(), "ws", pkgagent.Message{ID: "m"})
	if (*Runtime)(nil).loadMemoryIndexes(context.Background(), "u", "w") != nil {
		t.Fatal("nil runtime indexes")
	}

	rt, q, ctx := testRuntime(t, false)
	rt.bus = nil
	rt.publish(pkgagent.AgentEvent{EventID: "e2", SessionID: "s"})
	rt.indexPersistedMessage(ctx, "missing")
	rt.indexMessage(ctx, "missing", pkgagent.Message{ID: "m"})
	if ptrTime(nullString("not-a-time")) != nil {
		t.Fatal("bad ptr time")
	}
	ap := mapApproval(sqlite.Approval{ToolCalls: `[{"id":"c9","name":"ping"}]`})
	if ap.ToolCallID != "c9" {
		t.Fatalf("approval first=%s", ap.ToolCallID)
	}

	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "idx", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	key := memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "dup", Kind: memory.KindIndex, Name: memory.NameIndex}
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{
		Scope:   key.Scope,
		ScopeID: key.ScopeID,
		Name:    key.Name,
		Content: strings.Repeat("line\n", memory.IndexMaxLines+2),
	}); err != nil {
		t.Fatal(err)
	}
	rt.SetModel(pkgagent.ModelConfig{
		Provider: "openai",
		Model:    "gpt",
		Options:  mustJSON(map[string]string{"base_url": "http://127.0.0.1:1", "api_key": "x"}),
	})
	rt.EnqueueIndexCompact(key)
	rt.EnqueueIndexCompact(key)
	rt.WaitIndexCompact()

	rt.SetModel(pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{IndexCompactSummary: ""}),
	})
	rt.EnqueueIndexCompact(key)
	rt.WaitIndexCompact()
	_ = runID
}

// TestClosedDBErrorPaths 覆盖数据库关闭后 persist / Recover / Commit 的错误日志路径。
func TestClosedDBErrorPaths(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "close-me", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.persistStepJob(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}, stepJobQueued); err != nil {
		t.Fatal(err)
	}
	if err := rt.db.Close(); err != nil {
		t.Fatal(err)
	}
	rt.markStepJobStatus(ctx, runID, 1, stepJobRunning)
	rt.cancelOpenStepJobs(ctx, runID)
	if _, _, err := rt.latestOpenStepJob(ctx, runID); err == nil {
		t.Fatal("expected latest error")
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 2, Phase: pkgagent.PhaseUserInput}); err == nil {
		t.Fatal("expected persist error")
	}
	_ = rt.RecoverActive(ctx)
	_ = rt.RecoverRun(ctx, runID)
	_ = rt.RequestCancel(ctx, runID)
	_, _ = rt.ClaimSession(ctx, sessionID, runID)
	_, _ = rt.CreateAgentState(ctx, sessionID, "after close", cfg.Mode, cfg)
	_, _ = rt.AppendFact(ctx, runID, pkgagent.Fact{Type: pkgagent.EventAssistantDelta, Payload: []byte(`{}`)})
	_ = rt.CommitStep(ctx, runID, pkgagent.StepResult{State: pkgagent.AgentState{Status: pkgagent.RunCompleted, StepIndex: 1}})
	rt.indexPersisted(ctx, "default", pkgagent.Message{ID: util.NewID(), Content: pkgagent.EncodeText("hi")})
	rt.compactIndex(ctx, memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "u1", Kind: memory.KindIndex, Name: memory.NameIndex})
}
