package agent

import (
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDecideApprovalAndAutoReview(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options: mustJSON(pkgagent.FakeOptions{
			Review: &pkgagent.FakeReview{
				Decisions: []pkgagent.ApprovalDecision{{ToolCallID: "c1", Status: pkgagent.ApprovalApproved, Reason: "ok"}},
			},
		}),
	})
	cfg.Approval = pkgagent.ApprovalAuto
	runID, err := rt.CreateAgentState(ctx, sessionID, "need ping", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	turnID := util.NewID()
	approvalID := util.NewID()
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			SessionID: sessionID,
			RunID:     runID,
			TurnID:    &turnID,
			Status:    pkgagent.RunRunningLLM,
			StepIndex: 1,
			Config:    cfg,
		},
	}); err != nil {
		t.Fatal(err)
	}
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
	}); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetApproval(ctx, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(pkgagent.ApprovalPending) {
		t.Fatalf("waiting approval status=%s", row.Status)
	}

	got, err := rt.DecideApproval(ctx, ApprovalVerdict{
		ApprovalID: approvalID,
		Decisions:  []pkgagent.ApprovalDecision{{ToolCallID: "c1", Status: pkgagent.ApprovalApproved, Reason: "ok"}},
		ActorID:    "user",
	})
	if err != nil || got.Status != pkgagent.ApprovalApproved {
		t.Fatalf("decide %+v %v", got, err)
	}
	again, err := rt.DecideApproval(ctx, ApprovalVerdict{ApprovalID: approvalID})
	if err != nil || again.Status != pkgagent.ApprovalApproved {
		t.Fatalf("resubmit %+v %v", again, err)
	}
}

func TestDecideApprovalDenyAndValidate(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	if _, err := rt.DecideApproval(ctx, ApprovalVerdict{}); err == nil {
		t.Fatal("empty id")
	}
	var nilRT *Runtime
	if _, err := nilRT.DecideApproval(context.Background(), ApprovalVerdict{ApprovalID: "x"}); err == nil {
		t.Fatal("nil runtime")
	}
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "x", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	turnID := util.NewID()
	approvalID := util.NewID()
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			SessionID: sessionID, RunID: runID, TurnID: &turnID, Status: pkgagent.RunRunningLLM, StepIndex: 1, Config: cfg,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.CommitStep(ctx, runID, pkgagent.StepResult{
		State: pkgagent.AgentState{
			SessionID: sessionID, RunID: runID, TurnID: &turnID, Status: pkgagent.RunWaitingApproval, StepIndex: 2, Config: cfg,
			PendingApproval: &approvalID,
			Checkpoint:      pkgagent.ToolCheckpoint{TurnID: turnID, Pending: []tool.Call{{ID: "c1", Name: "ping"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := rt.DecideApproval(ctx, ApprovalVerdict{
		ApprovalID: approvalID,
		Status:     pkgagent.ApprovalDenied,
		Reason:     "no",
		Scope:      pkgagent.ApprovalOnce,
		ActorID:    "user",
	})
	if err != nil || got.Status != pkgagent.ApprovalDenied {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := rt.DecideApproval(ctx, ApprovalVerdict{ApprovalID: "missing"}); err == nil {
		t.Fatal("missing approval")
	}
}

func TestNormalizeVerdictHelpers(t *testing.T) {
	calls := []pkgagent.ApprovalToolCall{{ID: "a"}, {ID: "b"}}
	got, err := normalizeVerdict(ApprovalVerdict{
		Decisions: []pkgagent.ApprovalDecision{{ToolCallID: "a", Status: pkgagent.ApprovalApproved}},
	}, calls)
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := normalizeVerdict(ApprovalVerdict{}, calls); err == nil {
		t.Fatal("empty decisions")
	}
	if _, err := normalizeVerdict(ApprovalVerdict{
		Decisions: []pkgagent.ApprovalDecision{{ToolCallID: "a", Status: "weird"}, {ToolCallID: "b", Status: pkgagent.ApprovalDenied}},
	}, calls); err == nil {
		t.Fatal("invalid status")
	}
	if _, err := normalizeVerdict(ApprovalVerdict{
		Decisions: []pkgagent.ApprovalDecision{{ToolCallID: "z", Status: pkgagent.ApprovalApproved}, {ToolCallID: "a", Status: pkgagent.ApprovalDenied}},
	}, calls); err == nil {
		t.Fatal("unknown id")
	}
	if _, err := normalizeVerdict(ApprovalVerdict{
		Decisions: []pkgagent.ApprovalDecision{
			{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
			{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
		},
	}, calls); err == nil {
		t.Fatal("duplicate")
	}
}

func TestAutoReviewEscalate(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	rt.autoReviewPending(ctx, "", "")
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	cfg.Approval = pkgagent.ApprovalAuto
	runID, err := rt.CreateAgentState(ctx, sessionID, "x", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID: "ap1", SessionID: sessionID, RunID: runID, ToolCallID: "c1",
		ToolCalls: `[{"id":"c1","name":"ping","status":"pending"}]`,
		Scope:     string(pkgagent.ApprovalOnce), Status: string(pkgagent.ApprovalPending),
		ExpiresAt: util.FormatTime(util.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	rt.autoReviewPending(ctx, runID, "ap1")
	row, err := q.GetApproval(ctx, "ap1")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(pkgagent.ApprovalPending) {
		t.Fatalf("escalate should keep pending, got %s", row.Status)
	}
}

func TestDecideApprovalExpiredAndFill(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "x", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID: "expired-1", SessionID: sessionID, RunID: runID, ToolCallID: "c1",
		ToolCalls: `[{"id":"c1","name":"ping","status":"pending"}]`,
		Scope:     string(pkgagent.ApprovalOnce), Status: string(pkgagent.ApprovalPending),
		ExpiresAt: util.FormatTime(util.Now().Add(-time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := rt.DecideApproval(ctx, ApprovalVerdict{ApprovalID: "expired-1", ActorID: "user"})
	if err != nil || got.Status != pkgagent.ApprovalExpired {
		t.Fatalf("%+v %v", got, err)
	}

	mixed := fillVerdict([]pkgagent.ApprovalDecision{
		{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
		{ToolCallID: "b", Status: pkgagent.ApprovalDenied},
	}, []pkgagent.ApprovalToolCall{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	if len(mixed) != 2 {
		t.Fatalf("%+v", mixed)
	}
	filled := fillVerdict([]pkgagent.ApprovalDecision{
		{ToolCallID: "", Status: pkgagent.ApprovalApproved},
		{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
		{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
	}, []pkgagent.ApprovalToolCall{{ID: "a"}, {ID: "b"}})
	if len(filled) != 2 {
		t.Fatalf("%+v", filled)
	}

	rt.ReindexMessage(ctx, "", pkgagent.Message{})
	rt.ReindexMessage(ctx, sessionID, pkgagent.Message{ID: "m1", Content: pkgagent.EncodeText("hi")})
	rt.autoReviewPending(ctx, runID, "missing")
}

func TestAutoReviewFailAndAlreadyDecided(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Review: &pkgagent.FakeReview{Fail: true}}),
	})
	cfg.Approval = pkgagent.ApprovalAuto
	runID, err := rt.CreateAgentState(ctx, sessionID, "x", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID: "fail-1", SessionID: sessionID, RunID: runID, ToolCallID: "c1",
		ToolCalls: `[{"id":"c1","name":"ping","status":"pending"}]`,
		Scope:     string(pkgagent.ApprovalOnce), Status: string(pkgagent.ApprovalPending),
		ExpiresAt: util.FormatTime(util.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	rt.autoReviewPending(ctx, runID, "fail-1")
	row, err := q.GetApproval(ctx, "fail-1")
	if err != nil || row.Status != string(pkgagent.ApprovalPending) {
		t.Fatalf("%+v %v", row, err)
	}
	if _, err := q.UpdateApproval(ctx, sqlite.UpdateApprovalParams{
		Scope: string(pkgagent.ApprovalOnce), Status: string(pkgagent.ApprovalApproved),
		ToolCalls: `[{"id":"c1","name":"ping","status":"approved"}]`, ID: "fail-1",
	}); err != nil {
		t.Fatal(err)
	}
	rt.autoReviewPending(ctx, runID, "fail-1")
	_ = tool.Call{}
}

func TestDecideApprovalInvalidPending(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	cfg := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{Provider: "fake", Model: "fake"})
	runID, err := rt.CreateAgentState(ctx, sessionID, "x", cfg.Mode, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID: "bad-1", SessionID: sessionID, RunID: runID, ToolCallID: "c1",
		ToolCalls: `[{"id":"c1","name":"ping","status":"pending"}]`,
		Scope:     string(pkgagent.ApprovalOnce), Status: string(pkgagent.ApprovalPending),
		ExpiresAt: util.FormatTime(util.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.DecideApproval(ctx, ApprovalVerdict{
		ApprovalID: "bad-1",
		Decisions:  []pkgagent.ApprovalDecision{{ToolCallID: "c1", Status: "weird"}},
	}); err == nil {
		t.Fatal("expected invalid")
	}
}

func TestNeedsRecoverHelpers(t *testing.T) {
	var nilRT *Runtime
	ok, err := nilRT.NeedsRecover(context.Background(), "x")
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	rt, _, ctx := testRuntime(t, false)
	ok, err = rt.NeedsRecover(ctx, "")
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	if _, err := rt.NeedsRecover(ctx, "missing"); err == nil {
		t.Fatal("missing run")
	}
	_ = rt.RecoverActive(ctx)
}

func TestDecideApprovalRecoverMissingRun(t *testing.T) {
	rt, q, ctx := testRuntime(t, false)
	sessionID := insertSession(t, q, ctx)
	if _, err := q.InsertApproval(ctx, sqlite.InsertApprovalParams{
		ID: "orphan-1", SessionID: sessionID, RunID: "missing-run", ToolCallID: "c1",
		ToolCalls: `[{"id":"c1","name":"ping","status":"pending"}]`,
		Scope:     string(pkgagent.ApprovalOnce), Status: string(pkgagent.ApprovalPending),
		ExpiresAt: util.FormatTime(util.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.DecideApproval(ctx, ApprovalVerdict{
		ApprovalID: "orphan-1",
		Status:     pkgagent.ApprovalDenied,
	}); err == nil {
		t.Fatal("recover missing run")
	}
}
