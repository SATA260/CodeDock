package agent

import (
	"context"
	"database/sql"
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
	"codedock/pkg/db/sqlite"
)

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
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
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
		Mode:             string(pkgagent.ModeAutoApprove),
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
	if _, err := rt.CreateAgentState(ctx, "missing-session", "x", "", pkgagent.RunConfigSnapshot{Mode: pkgagent.ModeAutoApprove}); err == nil {
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
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	if hist.Prompt != pkgagent.DefaultSystemPrompt {
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
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
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
