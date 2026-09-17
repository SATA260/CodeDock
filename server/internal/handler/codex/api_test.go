package codex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	intcodex "codedock/internal/codex"
	pkg "codedock/pkg/codex"
)

func newCodexAPI(t *testing.T, fake *intcodex.FakeHandler) (*API, *intcodex.Runtime) {
	t.Helper()
	if fake == nil {
		fake = intcodex.NewFakeHandler()
	}
	rt := intcodex.New(intcodex.Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.149.0", nil },
		Start:    intcodex.LoopbackStarter(fake.Handle),
	})
	t.Cleanup(func() { _ = rt.Close() })
	return New(rt), rt
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestHTTPCatalogAndSession(t *testing.T) {
	api, _ := newCodexAPI(t, nil)
	r := chi.NewRouter()
	api.Mount(r)

	if rec := doJSON(t, r, http.MethodGet, "/codex/status", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodGet, "/codex/models", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodGet, "/codex/modes", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodGet, "/codex/commands", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec := doJSON(t, r, http.MethodPost, "/codex/sessions", `{"settings":{"cwd":"/tmp"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Body.String())
	}
	var created struct {
		Session pkg.Session `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Session.ID
	if rec := doJSON(t, r, http.MethodGet, "/codex/sessions", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodGet, "/codex/sessions/"+id, ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPatch, "/codex/sessions/"+id, `{"title":"Hi"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodGet, "/codex/sessions/"+id+"/settings", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/settings", `{"effort":"high"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/attachments/mention", `{"path":"a.go"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/attachments/image", `{"path":"a.png"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/commands", `{"name":"mcp"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/turns", `{"content":"hello"}`); rec.Code != http.StatusAccepted {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/fork", ""); rec.Code != http.StatusCreated {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/compact", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/review", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodGet, "/codex/sessions/"+id+"/asks", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/archive", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/asks/missing/decision", `{"approved":true}`); rec.Code != 404 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/turns/x/interrupt", `{}`); rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}

func TestHTTPAskAndSSE(t *testing.T) {
	fake := intcodex.NewFakeHandler()
	fake.SendAsk = true
	api, rt := newCodexAPI(t, fake)
	r := chi.NewRouter()
	api.Mount(r)
	rec := doJSON(t, r, http.MethodPost, "/codex/sessions", `{"settings":{}}`)
	var created struct {
		Session pkg.Session `json:"session"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id := created.Session.ID
	turnRec := doJSON(t, r, http.MethodPost, "/codex/sessions/"+id+"/turns", `{"content":"hello"}`)
	if turnRec.Code != http.StatusAccepted {
		t.Fatal(turnRec.Body.String())
	}
	var turnBody struct {
		Turn pkg.Turn `json:"turn"`
	}
	_ = json.Unmarshal(turnRec.Body.Bytes(), &turnBody)
	deadline := time.Now().Add(2 * time.Second)
	var asks struct {
		Asks []pkg.ApprovalAsk `json:"asks"`
	}
	for time.Now().Before(deadline) {
		list := doJSON(t, r, http.MethodGet, "/codex/sessions/"+id+"/asks", "")
		_ = json.Unmarshal(list.Body.Bytes(), &asks)
		if len(asks.Asks) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(asks.Asks) == 0 {
		t.Fatal("no asks")
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/asks/"+asks.Asks[0].ID+"/decision", `{"approved":false}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := doJSON(t, r, http.MethodPost, "/codex/turns/"+turnBody.Turn.ID+"/interrupt", `{"session_id":"`+id+`"}`); rec.Code != 200 && rec.Code != 404 {
		t.Fatal(rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/codex/sessions/"+id+"/events?after=0", nil)
	req.Header.Set("Last-Event-ID", "0")
	ctx, cancel := context.WithTimeout(req.Context(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	sse := httptest.NewRecorder()
	r.ServeHTTP(sse, req)
	if sse.Code != 200 || !strings.Contains(sse.Body.String(), "event:") {
		t.Fatalf("sse code=%d body=%s", sse.Code, sse.Body.String())
	}
	_ = rt
}

func TestMountNil(t *testing.T) {
	r := chi.NewRouter()
	(&API{}).Mount(r)
	New(nil).Mount(r)
}

func TestHTTPErrorPaths(t *testing.T) {
	fake := intcodex.NewFakeHandler()
	fake.Authorized = false
	api, _ := newCodexAPI(t, fake)
	r := chi.NewRouter()
	api.Mount(r)
	if rec := doJSON(t, r, http.MethodPost, "/codex/sessions", `{"settings":{}}`); rec.Code != http.StatusUnauthorized {
		t.Fatal(rec.Code, rec.Body.String())
	}

	okAPI, _ := newCodexAPI(t, nil)
	ok := chi.NewRouter()
	okAPI.Mount(ok)
	if rec := doJSON(t, ok, http.MethodPost, "/codex/sessions", `{`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code, rec.Body.String())
	}
	created := doJSON(t, ok, http.MethodPost, "/codex/sessions", `{"settings":{}}`)
	var body struct {
		Session pkg.Session `json:"session"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &body)
	id := body.Session.ID
	if rec := doJSON(t, ok, http.MethodGet, "/codex/sessions/missing", ""); rec.Code != http.StatusNotFound {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, ok, http.MethodPost, "/codex/sessions/"+id+"/attachments/mention", `{"path":""}`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, ok, http.MethodPost, "/codex/sessions/"+id+"/commands", `{"name":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, ok, http.MethodPost, "/codex/asks/missing/expire", ""); rec.Code != http.StatusNotFound {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, ok, http.MethodPost, "/codex/asks/missing/decision", `{`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
	if rec := doJSON(t, ok, http.MethodPost, "/codex/sessions/"+id+"/commands", `{"name":"stop"}`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code, rec.Body.String())
	}

	w := &plainWriter{hdr: make(http.Header)}
	req := httptest.NewRequest(http.MethodGet, "/codex/sessions/"+id+"/events", nil)
	ok.ServeHTTP(w, req)
	if w.code != http.StatusInternalServerError {
		t.Fatalf("streaming unsupported code=%d", w.code)
	}
}

type plainWriter struct {
	hdr  http.Header
	code int
	buf  strings.Builder
}

func (p *plainWriter) Header() http.Header         { return p.hdr }
func (p *plainWriter) Write(b []byte) (int, error) { return p.buf.WriteString(string(b)) }
func (p *plainWriter) WriteHeader(c int)           { p.code = c }
