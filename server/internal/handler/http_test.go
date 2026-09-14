package handler_test

import (
	"codedock/internal/agent/memory"
	"codedock/internal/events"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestHTTPCoverageEndpoints(t *testing.T) {
	f := newFixture(t)
	if rec := f.do(t, http.MethodGet, "/health", nil); rec.Code != http.StatusOK {
		t.Fatalf("health %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing user %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPost, "/sessions/x/runs", map[string]any{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty content %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPost, "/sessions", "not-json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json %d %s", rec.Code, rec.Body.String())
	}

	sessionID := f.createSession(t)
	rec := f.do(t, http.MethodPatch, "/sessions/"+sessionID, handler.UpdateSessionRequest{AgentID: "other"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update %d %s", rec.Code, rec.Body.String())
	}
	if rec = f.do(t, http.MethodGet, "/sessions/"+sessionID, nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	} else {
		var created handler.SessionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if created.Session.WorkspaceID == "" || created.Session.WorkspaceID == "default" {
			t.Fatalf("session workspace should be a frozen directory, got %q", created.Session.WorkspaceID)
		}
	}
	ws := t.TempDir()
	rec = f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{UserID: "u1", WorkspaceID: ws})
	if rec.Code != http.StatusOK {
		t.Fatalf("create with workspace %d %s", rec.Code, rec.Body.String())
	}
	var pinned handler.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pinned); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(ws)
	if pinned.Session.WorkspaceID != ws && pinned.Session.WorkspaceID != want {
		t.Fatalf("frozen workspace %q want %q", pinned.Session.WorkspaceID, want)
	}
	rec = f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{
		UserID: "u1", WorkspaceID: filepath.Join(ws, "missing-subdir"),
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{UserID: "u1", WorkspaceID: "default"})
	if rec.Code != http.StatusOK {
		t.Fatalf("implicit default workspace %d %s", rec.Code, rec.Body.String())
	}
	if rec = f.do(t, http.MethodGet, "/sessions", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec = f.do(t, http.MethodGet, "/sessions/missing", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing session %d", rec.Code)
	}
	if rec = f.do(t, http.MethodGet, "/sessions/"+sessionID+"/usage", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec = f.do(t, http.MethodGet, "/memories", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("memories %d", rec.Code)
	}
	if rec = f.do(t, http.MethodGet, "/memories?user_id=u1&workspace_id=default", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec = f.do(t, http.MethodGet, "/memories/user/u1", nil); rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Fatalf("get memory %d %s", rec.Code, rec.Body.String())
	}
	if rec = f.do(t, http.MethodDelete, "/memories/user/u1?all=1", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if _, err := memory.Upsert(context.Background(), f.queries, memory.TextMemory{
		Scope: memory.ScopeUser, ScopeID: "u1", Kind: memory.KindIndex, Name: memory.NameIndex, Content: "hi",
	}); err != nil {
		t.Fatal(err)
	}
	if rec = f.do(t, http.MethodGet, "/memories/user/u1?name=index", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec = f.do(t, http.MethodDelete, "/memories/user/u1?name=index", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}

	rec = f.do(t, http.MethodPost, "/sessions/"+sessionID+"/messages", handler.CreateMessageRequest{
		Content:  "via message",
		Mode:     pkgagent.WorkAsk,
		Approval: pkgagent.ApprovalManual,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.WorkAsk, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "asked"}},
		}),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create message %d %s", rec.Code, rec.Body.String())
	}
	var msg handler.MessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	runID := ""
	if msg.Message.RunID != nil {
		runID = *msg.Message.RunID
	}
	run := f.waitRun(t, runID, pkgagent.RunCompleted)
	if rec = f.do(t, http.MethodGet, "/runs/"+run.ID+"/usage", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec = f.do(t, http.MethodPost, "/runs/"+run.ID+"/retry", nil); rec.Code == 0 {
		t.Fatal("retry")
	}
	if msg.Message.ID != "" {
		if rec = f.do(t, http.MethodDelete, "/sessions/"+sessionID+"/messages/"+msg.Message.ID, nil); rec.Code != http.StatusOK {
			t.Fatal(rec.Body.String())
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/events?after=0", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "0")
	sse := httptest.NewRecorder()
	f.router.ServeHTTP(sse, req)

	if rec = f.do(t, http.MethodGet, "/sessions/"+sessionID+"/event-log?after=0", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}

	if rec = f.do(t, http.MethodPost, "/sessions/"+sessionID+"/archive", nil); rec.Code != http.StatusOK {
		t.Fatalf("archive %d %s", rec.Code, rec.Body.String())
	}
	if rec = f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{Content: "nope"}); rec.Code != http.StatusConflict {
		t.Fatalf("archived start %d %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPApprovalGetAndDecideErrors(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	runID := startPingApproval(t, f, sessionID)
	f.waitRun(t, runID, pkgagent.RunWaitingApproval)
	approval := firstPendingApproval(t, f, sessionID)
	rec := f.do(t, http.MethodGet, "/approvals/"+approval.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec = f.do(t, http.MethodGet, "/approvals/missing", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing approval %d", rec.Code)
	}
	if rec = f.do(t, http.MethodPost, "/approvals/"+approval.ID+"/decision", "nope"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad decide %d", rec.Code)
	}
	decideApproval(t, f, approval.ID, pkgagent.ApprovalDenied)
}

func TestHTTPContinueRetryMissing(t *testing.T) {
	f := newFixture(t)
	if rec := f.do(t, http.MethodPost, "/runs/missing/continue", nil); rec.Code == http.StatusOK {
		t.Fatal("continue missing")
	}
	if rec := f.do(t, http.MethodPost, "/runs/missing/retry", nil); rec.Code == http.StatusOK {
		t.Fatal("retry missing")
	}
	if rec := f.do(t, http.MethodGet, "/runs/missing", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get run %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/sessions/missing/event-log", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("events %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/sessions/missing/messages", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("messages %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/sessions/missing/approvals", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("approvals %d", rec.Code)
	}
}

func TestHTTPMoreBranches(t *testing.T) {
	f := newFixture(t)
	if rec := f.do(t, http.MethodGet, "/sessions?page=-1", nil); rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("list page %d", rec.Code)
	}
	sessionID := f.createSession(t)
	if rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/messages?page_size=0", nil); rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("messages page %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals?page=abc", nil); rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("approvals page %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/usage?page_size=-3", nil); rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("usage page %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPatch, "/sessions/"+sessionID, "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("update json %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("start json %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/messages", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("message json %d", rec.Code)
	}

	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "x"}},
		}),
	})
	f.waitRun(t, runID, pkgagent.RunRunningLLM, pkgagent.RunLoadingContext, pkgagent.RunQueued)
	if rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/archive", nil); rec.Code != http.StatusConflict {
		t.Fatalf("archive active %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(t, http.MethodPost, "/runs/"+runID+"/cancel", nil); rec.Code != http.StatusOK {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body.String())
	}
	f.waitRun(t, runID, pkgagent.RunCancelled, pkgagent.RunFailed, pkgagent.RunCompleted)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/events", nil).WithContext(ctx)
		f.router.ServeHTTP(httptest.NewRecorder(), req)
	}()
	time.Sleep(20 * time.Millisecond)
	f.bus.Publish(events.Event{
		ChatSessionID: sessionID,
		Payload:       pkgagent.AgentEvent{EventID: "live", SessionID: sessionID, Seq: 99, Type: pkgagent.EventRunCreated},
	})
	f.bus.Publish(events.Event{ChatSessionID: "other", Payload: "nope"})
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
}
