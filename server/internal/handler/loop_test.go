package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
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
		if !hasEventType(t, f, sessionID, pkgagent.EventApprovalDecided) {
			t.Fatal("expected tool.approval_decided after approve")
		}
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
		if !hasEventType(t, f, sessionID, pkgagent.EventApprovalDecided) {
			t.Fatal("expected tool.approval_decided after deny")
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
	if rec := f.do(t, http.MethodPost, "/runs/"+oldID+"/cancel", nil); rec.Code != http.StatusOK {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body.String())
	}
	f.waitRun(t, oldID, pkgagent.RunCancelled)
	newID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "take over",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "new"}},
		}),
	})
	f.waitRun(t, newID, pkgagent.RunCompleted)
}

func TestLoopStartInputHandled(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	f.runtime.SetDispatcher(seam.Func(func(_ context.Context, ev seam.Envelope) (seam.Envelope, error) {
		if ev.Type == seam.TypeInput {
			ev.Type = seam.TypeInputHandled
		}
		return ev, nil
	}))
	rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{
		Content: "skip me",
		Mode:    pkgagent.ModeAutoApprove,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("handled start %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.StartRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Handled || resp.RunID != "" || resp.SessionID != sessionID {
		t.Fatalf("resp=%+v", resp)
	}
	f.runtime.SetDispatcher(nil)
	second := f.start(t, sessionID, handler.StartRunRequest{
		Content: "now start",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "ok"}},
		}),
	})
	f.waitRun(t, second, pkgagent.RunCompleted)
}

func TestLoopStartInputRewritesContent(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	f.runtime.SetDispatcher(seam.Func(func(_ context.Context, ev seam.Envelope) (seam.Envelope, error) {
		if ev.Type != seam.TypeInput {
			return ev, nil
		}
		var payload pkgagent.InputPayload
		_ = json.Unmarshal(ev.Payload, &payload)
		payload.Content = "rewritten"
		ev.Payload = pkgagent.MarshalPayload(payload)
		return ev, nil
	}))
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "original",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "ok"}},
		}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	msgs := listMessages(t, f, sessionID, "")
	if len(msgs.Messages) == 0 || pkgagent.DecodeText(msgs.Messages[0].Content) != "rewritten" {
		t.Fatalf("messages=%+v", msgs.Messages)
	}
}

func TestLoopStartInputDispatchError(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	f.runtime.SetDispatcher(seam.Func(func(context.Context, seam.Envelope) (seam.Envelope, error) {
		return seam.Envelope{}, fmt.Errorf("plugin down")
	}))
	rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{
		Content: "x",
		Mode:    pkgagent.ModeAutoApprove,
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoopStartWhileActiveConflicts(t *testing.T) {
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
	rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{
		Content: "should not start",
		Mode:    pkgagent.ModeAutoApprove,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", rec.Code, rec.Body.String())
	}
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

func TestContinueRecoversCreatedRun(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	ctx := context.Background()
	cfg := withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
		Turns: []pkgagent.FakeTurn{{Text: "resumed"}},
	})
	runID, err := f.runtime.CreateAgentState(ctx, sessionID, "resume me", pkgagent.ModeAutoApprove, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.ClaimSession(ctx, sessionID, runID); err != nil {
		t.Fatal(err)
	}
	rec := f.do(t, http.MethodPost, "/runs/"+runID+"/continue", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("continue %d %s", rec.Code, rec.Body.String())
	}
	run := f.waitRun(t, runID, pkgagent.RunCompleted)
	if run.StopReason == nil || *run.StopReason != pkgagent.StopCompleted {
		t.Fatalf("stop=%v", run.StopReason)
	}
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

// hasEventType 判断会话事件日志是否包含指定类型。
func hasEventType(t *testing.T, f *fixture, sessionID string, typ pkgagent.EventType) bool {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/event-log?after=0", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("event-log %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for _, ev := range resp.Events {
		if ev.Type == typ {
			return true
		}
	}
	return false
}
