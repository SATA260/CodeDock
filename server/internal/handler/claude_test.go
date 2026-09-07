package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"codedock/internal/config"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

func newClaudeAPI(t *testing.T) *handler.API {
	t.Helper()
	return handler.New(nil, nil, nil, nil, pkgagent.RunConfigSnapshot{}, config.Load(), nil)
}

func claudeRouter(api *handler.API) http.Handler {
	r := chi.NewRouter()
	r.Get("/claude/status", api.ClaudeProbe)
	r.Get("/claude/models", api.ClaudeListModels)
	r.Get("/claude/modes", api.ClaudeListModes)
	r.Get("/claude/commands", api.ClaudeListCommands)
	r.Get("/claude/sessions", api.ClaudeListSessions)
	r.Post("/claude/sessions", api.ClaudeCreateSession)
	r.Get("/claude/sessions/{session_id}", api.ClaudeGetSession)
	r.Patch("/claude/sessions/{session_id}", api.ClaudeRenameSession)
	r.Post("/claude/sessions/{session_id}/archive", api.ClaudeArchiveSession)
	r.Post("/claude/sessions/{session_id}/fork", api.ClaudeForkSession)
	r.Get("/claude/sessions/{session_id}/settings", api.ClaudeEffective)
	r.Post("/claude/sessions/{session_id}/settings", api.ClaudeApplySettings)
	r.Post("/claude/sessions/{session_id}/commands", api.ClaudeInvoke)
	r.Post("/claude/sessions/{session_id}/mentions", api.ClaudeMention)
	r.Post("/claude/sessions/{session_id}/images", api.ClaudeAttachImage)
	r.Post("/claude/sessions/{session_id}/turns", api.ClaudeStartTurn)
	r.Get("/claude/sessions/{session_id}/transcript", api.ClaudeHydrate)
	r.Post("/claude/turns/{turn_id}/cancel", api.ClaudeCancelTurn)
	r.Post("/claude/turns/{turn_id}/continue", api.ClaudeContinueTurn)
	r.Post("/claude/approvals/{approval_id}/decision", api.ClaudeDecide)
	r.Post("/claude/asks/{request_id}/reject-unknown", api.ClaudeRejectUnknown)
	return r
}

func doClaude(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s status %d body %s", method, path, rec.Code, rec.Body.String())
	}
	return rec
}

func decodeClaude(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		t.Fatalf("decode %T: %v body %s", dest, err, rec.Body.String())
	}
}

func assertJSONKeys(t *testing.T, v any, want ...string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%T keys %v want %v json %s", v, got, want, raw)
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("%T missing %q in %s", v, key, raw)
		}
	}
}

func TestClaudeHTTPContracts(t *testing.T) {
	assertJSONKeys(t, handler.ClaudeCreateSessionRequest{}, "user_id")
	assertJSONKeys(t, handler.ClaudeRenameSessionRequest{}, "title")
	assertJSONKeys(t, handler.ClaudeInvokeRequest{}, "name", "args")
	assertJSONKeys(t, handler.ClaudePathRequest{}, "path")
	assertJSONKeys(t, handler.ClaudeInput{}, "text", "mentions", "images")
	assertJSONKeys(t, handler.ClaudeStartTurnRequest{}, "content", "input", "mode")
	assertJSONKeys(t, handler.ClaudeApplySettingsRequest{}, "model", "effort", "permission_mode", "cwd", "overridden")
	assertJSONKeys(t, handler.ClaudeDecideRequest{}, "approved", "scope", "choice", "values")
	assertJSONKeys(t, handler.ClaudeRejectUnknownRequest{}, "session_id", "turn_id")

	assertJSONKeys(t, handler.ClaudeStatusResponse{}, "available", "authorized", "version", "hint")
	assertJSONKeys(t, handler.ClaudeModel{}, "id", "efforts", "default_effort", "hidden", "is_default")
	assertJSONKeys(t, handler.ClaudeListModelsResponse{}, "models")
	assertJSONKeys(t, handler.ClaudeMode{}, "id", "kind")
	assertJSONKeys(t, handler.ClaudeListModesResponse{}, "modes")
	assertJSONKeys(t, handler.ClaudeCommand{}, "name", "action", "hint")
	assertJSONKeys(t, handler.ClaudeListCommandsResponse{}, "commands")
	assertJSONKeys(t, handler.ClaudeSession{}, "id", "claude_session_id", "title", "active_turn_id", "archived")
	assertJSONKeys(t, handler.ClaudeSessionResponse{}, "session")
	assertJSONKeys(t, handler.ClaudeListSessionsResponse{}, "sessions")
	assertJSONKeys(t, handler.ClaudeSettingsResponse{}, "model", "effort", "permission_mode", "cwd", "overridden")
	assertJSONKeys(t, handler.ClaudeInvokeResponse{}, "hint")
	assertJSONKeys(t, handler.ClaudeStartTurnResponse{}, "turn_id")
	assertJSONKeys(t, handler.ClaudeProgress{}, "kind", "text", "command", "paths", "diff")
	assertJSONKeys(t, handler.ClaudeTranscriptResponse{}, "items")
	assertJSONKeys(t, handler.ClaudeOKResponse{}, "ok")
}

func TestClaudeSkeletonHTTP(t *testing.T) {
	router := claudeRouter(newClaudeAPI(t))

	var status handler.ClaudeStatusResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/status", ""), &status)

	var models handler.ClaudeListModelsResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/models", ""), &models)
	if models.Models == nil {
		t.Fatal("models must be []")
	}

	var modes handler.ClaudeListModesResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/modes", ""), &modes)
	if modes.Modes == nil {
		t.Fatal("modes must be []")
	}

	var commands handler.ClaudeListCommandsResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/commands", ""), &commands)
	if commands.Commands == nil {
		t.Fatal("commands must be []")
	}

	var sessions handler.ClaudeListSessionsResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/sessions", ""), &sessions)
	if sessions.Sessions == nil {
		t.Fatal("sessions must be []")
	}

	var created handler.ClaudeSessionResponse
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions", `{"user_id":"local"}`), &created)

	var got handler.ClaudeSessionResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/sessions/s1", ""), &got)

	var ok handler.ClaudeOKResponse
	decodeClaude(t, doClaude(t, router, http.MethodPatch, "/claude/sessions/s1", `{"title":"t"}`), &ok)
	if !ok.OK {
		t.Fatal("ok")
	}
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions/s1/archive", ""), &ok)
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions/s1/fork", ""), &created)

	var settings handler.ClaudeSettingsResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/sessions/s1/settings", ""), &settings)
	if settings.Overridden == nil {
		t.Fatal("overridden must be []")
	}
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions/s1/settings", `{"model":"opus","effort":"","permission_mode":"","cwd":"","overridden":[]}`), &settings)

	var invoked handler.ClaudeInvokeResponse
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions/s1/commands", `{"name":"mcp","args":""}`), &invoked)

	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions/s1/mentions", `{"path":"a.go"}`), &ok)
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions/s1/images", `{"path":"a.png"}`), &ok)

	var started handler.ClaudeStartTurnResponse
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/sessions/s1/turns", `{"content":"hi","input":{"text":"hi","mentions":[],"images":[]},"mode":"start"}`), &started)

	var transcript handler.ClaudeTranscriptResponse
	decodeClaude(t, doClaude(t, router, http.MethodGet, "/claude/sessions/s1/transcript", ""), &transcript)
	if transcript.Items == nil {
		t.Fatal("items must be []")
	}

	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/turns/t1/cancel", ""), &ok)
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/turns/t1/continue", ""), &ok)
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/approvals/a1/decision", `{"approved":true,"scope":"once","choice":"","values":[]}`), &ok)
	decodeClaude(t, doClaude(t, router, http.MethodPost, "/claude/asks/r1/reject-unknown", `{"session_id":"s1","turn_id":"t1"}`), &ok)
}
