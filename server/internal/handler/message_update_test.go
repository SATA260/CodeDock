package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

// TestUpdateQueuedMessage 校验只有 queued Run 的触发消息可以改正文。
func TestUpdateQueuedMessage(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	first := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang first",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Hang:  true,
			Turns: []pkgagent.FakeTurn{{Text: "late"}},
		}),
	})
	waitNeedsRecover(t, f, first, false)

	queued := f.start(t, sessionID, handler.StartRunRequest{
		Content:   "queued original",
		InputMode: handler.InputQueue,
		Mode:      pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "later"}},
		}),
	})
	queuedRun := getRun(t, f, queued)
	if queuedRun.Status != pkgagent.RunQueued {
		t.Fatalf("queued status=%s", queuedRun.Status)
	}

	rec := f.do(t, http.MethodPatch, "/sessions/"+sessionID+"/messages/"+queuedRun.TriggerMessageID, handler.UpdateMessageRequest{
		Content: "queued edited",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("edit queued %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.MessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if pkgagent.DecodeText(resp.Message.Content) != "queued edited" {
		t.Fatalf("content=%s", pkgagent.DecodeText(resp.Message.Content))
	}

	doneSess := f.createSession(t)
	doneID := f.start(t, doneSess, handler.StartRunRequest{
		Content: "already done",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "ok"}},
		}),
	})
	f.waitRun(t, doneID, pkgagent.RunCompleted)
	doneRun := getRun(t, f, doneID)
	blocked := f.do(t, http.MethodPatch, "/sessions/"+doneSess+"/messages/"+doneRun.TriggerMessageID, handler.UpdateMessageRequest{
		Content: "should fail",
	})
	if blocked.Code != http.StatusConflict {
		t.Fatalf("edit finished %d %s", blocked.Code, blocked.Body.String())
	}

	empty := f.do(t, http.MethodPatch, "/sessions/"+sessionID+"/messages/"+queuedRun.TriggerMessageID, handler.UpdateMessageRequest{
		Content: "   ",
	})
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty edit %d %s", empty.Code, empty.Body.String())
	}
}
