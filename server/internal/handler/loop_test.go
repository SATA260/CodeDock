package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

func TestLoopPlainText(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hello",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "hello"}},
		}),
	})
	run := f.waitRun(t, runID, pkgagent.RunCompleted)
	if run.StopReason == nil || *run.StopReason != pkgagent.StopCompleted {
		t.Fatalf("stop=%v", run.StopReason)
	}
	msgs := listMessages(t, f, sessionID, "")
	if len(msgs.Messages) < 2 {
		t.Fatalf("messages=%d", len(msgs.Messages))
	}
	var sawAssistant bool
	for _, msg := range msgs.Messages {
		if msg.Role == pkgagent.RoleAssistant && pkgagent.DecodeText(msg.Content) == "hello" {
			sawAssistant = true
		}
	}
	if !sawAssistant {
		t.Fatalf("missing assistant text: %+v", msgs.Messages)
	}
}

func TestLoopPingAutoApprove(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "ping please",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{
				{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping"}}},
				{Text: "pong"},
			},
		}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	msgs := listMessages(t, f, sessionID, "")
	var sawTool, sawPong bool
	for _, msg := range msgs.Messages {
		if msg.Role == pkgagent.RoleTool {
			sawTool = true
		}
		if msg.Role == pkgagent.RoleAssistant && pkgagent.DecodeText(msg.Content) == "pong" {
			sawPong = true
		}
	}
	if !sawTool || !sawPong {
		t.Fatalf("tool=%v pong=%v messages=%+v", sawTool, sawPong, msgs.Messages)
	}
}

func TestLoopAskForApprovalApproveAndDeny(t *testing.T) {
	t.Run("approve", func(t *testing.T) {
		f := newFixture(t)
		sessionID := f.createSession(t)
		runID := startPingApproval(t, f, sessionID)
		f.waitRun(t, runID, pkgagent.RunWaitingApproval)
		approval := firstPendingApproval(t, f, sessionID)
		decideApproval(t, f, approval.ID, pkgagent.ApprovalApproved)
		f.waitRun(t, runID, pkgagent.RunCompleted)
	})
	t.Run("deny", func(t *testing.T) {
		f := newFixture(t)
		sessionID := f.createSession(t)
		runID := startPingApproval(t, f, sessionID)
		f.waitRun(t, runID, pkgagent.RunWaitingApproval)
		approval := firstPendingApproval(t, f, sessionID)
		decideApproval(t, f, approval.ID, pkgagent.ApprovalDenied)
		run := f.waitRun(t, runID)
		if !pkgagent.IsTerminal(run.Status) {
			t.Fatalf("deny should not kill-lock run: %s", run.Status)
		}
	})
}

func TestLoopCancel(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "never"}},
		}),
	})
	f.waitRun(t, runID, pkgagent.RunRunningLLM, pkgagent.RunLoadingContext, pkgagent.RunQueued)
	rec := f.do(t, http.MethodPost, "/runs/"+runID+"/cancel", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body.String())
	}
	f.waitRun(t, runID, pkgagent.RunCancelled)
}

func TestLoopInterrupt(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	oldID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "old"}},
		}),
	})
	f.waitRun(t, oldID, pkgagent.RunRunningLLM, pkgagent.RunLoadingContext, pkgagent.RunQueued)
	newID := f.start(t, sessionID, handler.StartRunRequest{
		Content:   "take over",
		InputMode: handler.InputInterrupt,
		Mode:      pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "new"}},
		}),
	})
	f.waitRun(t, oldID, pkgagent.RunCancelled)
	f.waitRun(t, newID, pkgagent.RunCompleted)
}

func TestLoopInterruptWithQueued(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	first := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "first"}},
		}),
	})
	f.waitRun(t, first, pkgagent.RunRunningLLM, pkgagent.RunLoadingContext, pkgagent.RunQueued)
	queued := f.start(t, sessionID, handler.StartRunRequest{
		Content:   "wait your turn",
		InputMode: handler.InputQueue,
		Mode:      pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "queued"}},
		}),
	})
	interrupt := f.start(t, sessionID, handler.StartRunRequest{
		Content:   "take over now",
		InputMode: handler.InputInterrupt,
		Mode:      pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "interrupt"}},
		}),
	})
	f.waitRun(t, first, pkgagent.RunCancelled)
	f.waitRun(t, interrupt, pkgagent.RunCompleted)
	rec := f.do(t, http.MethodGet, "/runs/"+queued, nil)
	var queuedResp handler.RunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &queuedResp); err != nil {
		t.Fatal(err)
	}
	if queuedResp.Run.Status != pkgagent.RunQueued && !pkgagent.IsTerminal(queuedResp.Run.Status) {
		t.Fatalf("queued run should stay queued until interrupt finishes, status=%s", queuedResp.Run.Status)
	}
	f.waitRun(t, queued, pkgagent.RunCompleted)
}

func TestLoopQueueThenDequeue(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	first := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "first"}},
		}),
	})
	f.waitRun(t, first, pkgagent.RunRunningLLM, pkgagent.RunLoadingContext, pkgagent.RunQueued)
	second := f.start(t, sessionID, handler.StartRunRequest{
		Content:   "next please",
		InputMode: handler.InputQueue,
		Mode:      pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "second"}},
		}),
	})
	rec := f.do(t, http.MethodGet, "/runs/"+second, nil)
	var resp handler.RunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Run.Status != pkgagent.RunQueued {
		t.Fatalf("queued run status=%s", resp.Run.Status)
	}
	if rec := f.do(t, http.MethodPost, "/runs/"+first+"/cancel", nil); rec.Code != http.StatusOK {
		t.Fatalf("cancel first %d %s", rec.Code, rec.Body.String())
	}
	f.waitRun(t, first, pkgagent.RunCancelled)
	f.waitRun(t, second, pkgagent.RunCompleted)
}

func startPingApproval(t *testing.T, f *fixture, sessionID string) string {
	t.Helper()
	return f.start(t, sessionID, handler.StartRunRequest{
		Content: "need ping",
		Mode:    pkgagent.ModeAskForApproval,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{
				{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping"}}},
				{Text: "done"},
			},
		}),
	})
}

func firstPendingApproval(t *testing.T, f *fixture, sessionID string) pkgagent.Approval {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list approvals %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListApprovalsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for _, item := range resp.Approvals {
		if item.Status == pkgagent.ApprovalPending {
			return item
		}
	}
	t.Fatalf("no pending approval: %+v", resp.Approvals)
	return pkgagent.Approval{}
}

func decideApproval(t *testing.T, f *fixture, approvalID string, status pkgagent.ApprovalStatus) {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/approvals/"+approvalID+"/decision", handler.DecideApprovalRequest{
		Status: status,
		Scope:  pkgagent.ApprovalOnce,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("decide %d %s", rec.Code, rec.Body.String())
	}
}
