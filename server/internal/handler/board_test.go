package handler_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"codedock/internal/handler"
)

func TestBoardWorkHTTP(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	rec := f.do(t, http.MethodPost, "/works", handler.CreateWorkRequest{UserID: "u1", TenantID: "t1", Title: "修登录"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create work %d %s", rec.Code, rec.Body.String())
	}
	var created handler.WorkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Work.ID == "" || created.Work.Title != "修登录" {
		t.Fatalf("work %+v", created.Work)
	}
	rec = f.do(t, http.MethodPost, "/works/"+created.Work.ID+"/checkouts", handler.AttachCheckoutRequest{Path: dir, Kind: "folder"})
	if rec.Code != http.StatusOK {
		t.Fatalf("attach %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodPut, "/works/"+created.Work.ID+"/info", handler.PutInfoRequest{Body: "卡片说明"})
	if rec.Code != http.StatusOK {
		t.Fatalf("info %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodPost, "/works/"+created.Work.ID+"/sessions", handler.StartWorkSessionRequest{
		Engine: "agent", Kind: "talk", UserID: "u1", TenantID: "t1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("start talk %d %s", rec.Code, rec.Body.String())
	}
	var talk handler.PlacementResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &talk); err != nil {
		t.Fatal(err)
	}
	if talk.Engine != "agent" || talk.Placement.Checkout != "" {
		t.Fatalf("talk %+v", talk)
	}
	rec = f.do(t, http.MethodPost, "/works/"+created.Work.ID+"/sessions", handler.StartWorkSessionRequest{
		Engine: "agent", Kind: "dir", Checkout: dir, UserID: "u1", TenantID: "t1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("start dir %d %s", rec.Code, rec.Body.String())
	}
	sessionID := f.createSession(t)
	rec = f.do(t, http.MethodPost, "/works/"+created.Work.ID+"/placements", handler.AttachPlacementRequest{
		Engine: "agent", SessionID: sessionID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("attach session %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodGet, "/board?user_id=u1&tenant_id=t1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("board %d %s", rec.Code, rec.Body.String())
	}
	var board handler.BoardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if len(board.Board.Cards) != 1 {
		t.Fatalf("cards %+v", board.Board)
	}
	rec = f.do(t, http.MethodGet, "/placements/"+talk.Engine+"/"+talk.SessionID+"/packet", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("packet %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodGet, "/works/"+created.Work.ID+"/inbox", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("inbox %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodDelete, "/works/"+created.Work.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodGet, "/placements/agent/"+talk.SessionID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unbound placement %d %s", rec.Code, rec.Body.String())
	}
}

func TestBoardAttachRequiresExistingDir(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, http.MethodPost, "/works", handler.CreateWorkRequest{UserID: "u1", Title: "x"})
	var created handler.WorkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(os.TempDir(), "codedock-missing-checkout")
	rec = f.do(t, http.MethodPost, "/works/"+created.Work.ID+"/sessions", handler.StartWorkSessionRequest{
		Engine: "agent", Kind: "dir", Checkout: missing, UserID: "u1",
	})
	if rec.Code == http.StatusOK {
		t.Fatalf("missing checkout should fail: %s", rec.Body.String())
	}
}
