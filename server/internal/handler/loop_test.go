package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

func TestLoopPlainText(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hello",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
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
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
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
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
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
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
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
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "new"}},
		}),
	})
	f.waitRun(t, newID, pkgagent.RunCompleted)
}

func TestLoopModeSmoke(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)

	askID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "ask write",
		Mode:    pkgagent.WorkAsk,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{
				{ToolCalls: []pkgagent.FakeToolCall{{
					Name:      "write",
					Arguments: json.RawMessage(`{"path":"smoke.txt","content":"no"}`),
				}}},
				{Text: "ask-done"},
			},
		}),
	})
	askRun := f.waitRun(t, askID, pkgagent.RunCompleted)
	if askRun.Config.Mode != pkgagent.WorkAsk {
		t.Fatalf("ask mode=%s", askRun.Config.Mode)
	}
	if rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	} else {
		var resp handler.ListApprovalsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if len(resp.Approvals) != 0 {
			t.Fatalf("ask write must not open approval: %+v", resp.Approvals)
		}
	}

	planID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "plan only",
		Mode:    pkgagent.WorkPlan,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "plan-done"}},
		}),
	})
	f.waitRun(t, planID, pkgagent.RunCompleted)

	agentID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "agent ping",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{
				{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping"}}},
				{Text: "agent-done"},
			},
		}),
	})
	f.waitRun(t, agentID, pkgagent.RunCompleted)

	msgs := listMessages(t, f, sessionID, "")
	var sawAsk, sawPlan, sawAgent, sawUnbound, sawPing, sawDeveloper bool
	for _, msg := range msgs.Messages {
		if msg.Role == pkgagent.RoleDeveloper {
			sawDeveloper = true
		}
		text := pkgagent.DecodeText(msg.Content)
		switch msg.Role {
		case pkgagent.RoleAssistant:
			switch text {
			case "ask-done":
				sawAsk = true
			case "plan-done":
				sawPlan = true
			case "agent-done":
				sawAgent = true
			}
		case pkgagent.RoleTool:
			if strings.Contains(text, "tool is not bound") || strings.Contains(string(msg.Content), "tool is not bound") {
				sawUnbound = true
			}
			if strings.Contains(string(msg.Content), `"ok":true`) || strings.Contains(text, `"ok":true`) {
				sawPing = true
			}
		}
	}
	if sawDeveloper {
		t.Fatal("developer prompt must not be persisted")
	}
	if !sawAsk || !sawPlan || !sawAgent {
		t.Fatalf("missing mode replies ask=%v plan=%v agent=%v", sawAsk, sawPlan, sawAgent)
	}
	if !sawUnbound {
		t.Fatalf("ask write should fail as unbound: %+v", msgs.Messages)
	}
	if !sawPing {
		t.Fatalf("agent ping should execute: %+v", msgs.Messages)
	}
}

func TestLoopStartWhileActiveConflicts(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	first := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "first"}},
		}),
	})
	f.waitRun(t, first, pkgagent.RunRunningLLM, pkgagent.RunLoadingContext, pkgagent.RunQueued)
	rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{
		Content: "should not start",
		Mode:    pkgagent.WorkAgent,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", rec.Code, rec.Body.String())
	}
}

func startPingApproval(t *testing.T, f *fixture, sessionID string) string {
	t.Helper()
	return f.start(t, sessionID, handler.StartRunRequest{
		Content: "need ping",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.WorkAgent, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
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
	cfg := withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
		Turns: []pkgagent.FakeTurn{{Text: "resumed"}},
	})
	runID, err := f.runtime.CreateAgentState(ctx, sessionID, "resume me", pkgagent.WorkAgent, *cfg)
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
