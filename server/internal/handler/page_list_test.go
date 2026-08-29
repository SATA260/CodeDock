package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

func TestListSessionsPagination(t *testing.T) {
	f := newFixture(t)
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = f.createSession(t)
	}

	page1 := listSessions(t, f, "?page=1&page_size=2&sort_by=created_at&sort_order=asc")
	if page1.Total != 5 || page1.Page != 1 || page1.PageSize != 2 {
		t.Fatalf("page1 meta = %+v", page1.PageInfo)
	}
	if len(page1.Sessions) != 2 {
		t.Fatalf("page1 len=%d", len(page1.Sessions))
	}

	page2 := listSessions(t, f, "?page=2&page_size=2&sort_by=created_at&sort_order=asc")
	if len(page2.Sessions) != 2 {
		t.Fatalf("page2 len=%d", len(page2.Sessions))
	}

	page3 := listSessions(t, f, "?page=3&page_size=2&sort_by=created_at&sort_order=asc")
	if len(page3.Sessions) != 1 {
		t.Fatalf("page3 len=%d", len(page3.Sessions))
	}

	overflow := listSessions(t, f, "?page=99&page_size=2")
	if overflow.Total != 5 || len(overflow.Sessions) != 0 {
		t.Fatalf("overflow total=%d len=%d", overflow.Total, len(overflow.Sessions))
	}

	seen := map[string]int{}
	for _, page := range []handler.ListSessionsResponse{page1, page2, page3} {
		for _, session := range page.Sessions {
			seen[session.ID]++
		}
	}
	if len(seen) != 5 {
		t.Fatalf("unique=%d want 5", len(seen))
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("id %s count=%d", id, seen[id])
		}
	}

	bad := f.do(t, http.MethodGet, "/sessions?sort_by=id", nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid sort status=%d %s", bad.Code, bad.Body.String())
	}
}

func TestListMessagesPagination(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	for i := 0; i < 3; i++ {
		runID := f.start(t, sessionID, handler.StartRunRequest{
			Content: "m",
			Mode:    pkgagent.ModeAutoApprove,
			Config:  withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
		})
		f.waitRun(t, runID, pkgagent.RunCompleted)
	}

	all := listMessages(t, f, sessionID, "?page=1&page_size=20&sort_by=event_seq&sort_order=asc")
	if all.Total < 6 {
		t.Fatalf("total=%d body messages=%d", all.Total, len(all.Messages))
	}
	if all.AsOfEventSeq == 0 {
		t.Fatal("as_of_event_seq should be set")
	}

	page1 := listMessages(t, f, sessionID, "?page=1&page_size=2&sort_by=event_seq&sort_order=asc")
	page2 := listMessages(t, f, sessionID, "?page=2&page_size=2&sort_by=event_seq&sort_order=asc")
	if len(page1.Messages) != 2 || len(page2.Messages) != 2 {
		t.Fatalf("page lens %d %d", len(page1.Messages), len(page2.Messages))
	}
	if page1.Messages[0].EventSeq > page1.Messages[1].EventSeq {
		t.Fatalf("page1 not asc: %+v", page1.Messages)
	}
	if page1.Messages[1].EventSeq > page2.Messages[0].EventSeq {
		t.Fatalf("pages not contiguous: %d then %d", page1.Messages[1].EventSeq, page2.Messages[0].EventSeq)
	}

	desc := listMessages(t, f, sessionID, "?page=1&page_size=2&sort_by=event_seq&sort_order=desc")
	if desc.Messages[0].EventSeq < desc.Messages[1].EventSeq {
		t.Fatalf("desc not descending: %+v", desc.Messages)
	}

	overflow := listMessages(t, f, sessionID, "?page=99&page_size=2")
	if overflow.Total != all.Total || len(overflow.Messages) != 0 {
		t.Fatalf("overflow total=%d len=%d", overflow.Total, len(overflow.Messages))
	}
}

func listSessions(t *testing.T, f *fixture, query string) handler.ListSessionsResponse {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions"+query, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListSessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestListEventsReplay(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hello",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "ok"}}}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)

	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/event-log", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list events %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) == 0 {
		t.Fatal("expected persisted events")
	}
	if resp.Events[0].Seq <= 0 {
		t.Fatalf("seq=%d", resp.Events[0].Seq)
	}
}

func listMessages(t *testing.T, f *fixture, sessionID, query string) handler.ListMessagesResponse {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/messages"+query, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list messages %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListMessagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}
