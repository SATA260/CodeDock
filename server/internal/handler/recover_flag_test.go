package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

// TestNeedsRecoverHTTP 校验只有中断的 active Run 才带 needs_recover；执行中与等审批不带。
func TestNeedsRecoverHTTP(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	orphanedSess := f.createSession(t)
	cfg := withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
		Turns: []pkgagent.FakeTurn{{Text: "resumed"}},
	})
	orphaned, err := f.runtime.CreateAgentState(ctx, orphanedSess, "resume me", pkgagent.WorkAgent, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.ClaimSession(ctx, orphanedSess, orphaned); err != nil {
		t.Fatal(err)
	}
	if !getRun(t, f, orphaned).NeedsRecover {
		t.Fatal("orphaned claimed run should need recover")
	}
	if !getSession(t, f, orphanedSess).NeedsRecover {
		t.Fatal("orphaned session should need recover")
	}
	listed := listSessions(t, f, "")
	var sawOrphan bool
	for _, session := range listed.Sessions {
		if session.ID == orphanedSess {
			sawOrphan = true
			if !session.NeedsRecover {
				t.Fatal("listed orphaned session should need recover")
			}
		}
	}
	if !sawOrphan {
		t.Fatal("orphaned session missing from list")
	}

	liveSess := f.createSession(t)
	liveID := f.start(t, liveSess, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "late"}},
		}),
	})
	waitNeedsRecover(t, f, liveID, false)
	if getSession(t, f, liveSess).NeedsRecover {
		t.Fatal("live session should not need recover")
	}

	waitSess := f.createSession(t)
	waitID := startPingApproval(t, f, waitSess)
	f.waitRun(t, waitID, pkgagent.RunWaitingApproval)
	if getRun(t, f, waitID).NeedsRecover {
		t.Fatal("waiting approval should not need recover")
	}
	if getSession(t, f, waitSess).NeedsRecover {
		t.Fatal("waiting approval session should not need recover")
	}

	doneSess := f.createSession(t)
	doneID := f.start(t, doneSess, handler.StartRunRequest{
		Content: "done",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "ok"}},
		}),
	})
	f.waitRun(t, doneID, pkgagent.RunCompleted)
	if getRun(t, f, doneID).NeedsRecover {
		t.Fatal("completed run should not need recover")
	}
	if getSession(t, f, doneSess).NeedsRecover {
		t.Fatal("completed session should not need recover")
	}
}

func waitNeedsRecover(t *testing.T, f *fixture, runID string, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last pkgagent.Run
	for time.Now().Before(deadline) {
		last = getRun(t, f, runID)
		if last.NeedsRecover == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s needs_recover=%v want %v status=%s", runID, last.NeedsRecover, want, last.Status)
}

func getRun(t *testing.T, f *fixture, runID string) pkgagent.Run {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/runs/"+runID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get run %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.RunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Run
}

func getSession(t *testing.T, f *fixture, sessionID string) pkgagent.Session {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get session %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Session
}
