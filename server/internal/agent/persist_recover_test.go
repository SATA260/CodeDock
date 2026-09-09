package agent

import (
	"testing"
	"time"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

// TestEnqueuePersistsStepJob 校验 Enqueue 会把 StepJob 写成 queued。
func TestEnqueuePersistsStepJob(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	runID, err := rt.CreateAgentState(ctx, sessionID, "persist", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Enqueue(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetStepJob(ctx, sqlite.GetStepJobParams{RunID: runID, StepIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != stepJobQueued || row.Phase != string(pkgagent.PhaseUserInput) {
		t.Fatalf("step job %+v", row)
	}
}

// TestStartDoesNotRecoverPersistedJob 校验 Start 不自动恢复库里的未完成 Job。
func TestStartDoesNotRecoverPersistedJob(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	runID, err := rt.CreateAgentState(ctx, sessionID, "dormant", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if err := rt.persistStepJob(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput}, stepJobQueued); err != nil {
		t.Fatal(err)
	}
	rt.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	row, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(pkgagent.RunQueued) {
		t.Fatalf("start should not recover, status=%s", row.Status)
	}
	if err := rt.RecoverRun(ctx, runID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, q, ctx, runID, pkgagent.RunCompleted)
}

// TestRecoverRunSkipsPendingApproval 校验未裁决的 waiting_approval 不会被 RecoverRun 入队。
func TestRecoverRunSkipsPendingApproval(t *testing.T) {
	rt, q, ctx := testRuntime(t, true)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "wait", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateRun(ctx, sqlite.UpdateRunParams{
		Status:          string(pkgagent.RunWaitingApproval),
		CancelRequested: 0,
		ID:              runID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RecoverRun(ctx, runID); err != nil {
		t.Fatal(err)
	}
	if rt.worker.Busy(runID) {
		t.Fatal("pending approval should not enqueue")
	}
}

// TestRecoverRunResetsCrashedRunningJob 校验崩溃的 running job 恢复时 attempt 递增。
func TestRecoverRunResetsCrashedRunningJob(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	runID, err := rt.CreateAgentState(ctx, sessionID, "crash", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if err := rt.persistStepJob(ctx, pkgagent.StepJob{RunID: runID, StepIndex: 1, Phase: pkgagent.PhaseUserInput, Attempt: 1}, stepJobRunning); err != nil {
		t.Fatal(err)
	}
	rt.Start(ctx)
	if err := rt.RecoverRun(ctx, runID); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetStepJob(ctx, sqlite.GetStepJobParams{RunID: runID, StepIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	if row.Attempt < 2 {
		t.Fatalf("attempt=%d want >=2", row.Attempt)
	}
	waitStatus(t, q, ctx, runID, pkgagent.RunCompleted)
}
