package handler

import (
	"bytes"
	"context"
	"codedock/internal/config"
	cderr "codedock/internal/errors"
	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
	"codedock/pkg/git"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestFillBatchDecisions 校验整单同一裁决时会补齐未列出的 tool_call。
func TestFillBatchDecisions(t *testing.T) {
	calls := []pkgagent.ApprovalToolCall{{ID: "c0"}, {ID: "c1"}}
	got := fillBatchDecisions([]ToolDecision{{ToolCallID: "c1", Status: pkgagent.ApprovalApproved}}, calls)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	byID := map[string]pkgagent.ApprovalStatus{}
	for _, item := range got {
		byID[item.ToolCallID] = item.Status
	}
	if byID["c0"] != pkgagent.ApprovalApproved || byID["c1"] != pkgagent.ApprovalApproved {
		t.Fatalf("got=%+v", got)
	}
}

// TestNormalizeDecisionsCoversBatch 校验只点一条时也能整单通过。
func TestNormalizeDecisionsCoversBatch(t *testing.T) {
	calls := []pkgagent.ApprovalToolCall{{ID: "c0"}, {ID: "c1"}}
	got, err := normalizeDecisions(DecideApprovalRequest{
		Decisions: []ToolDecision{{ToolCallID: "c1", Status: pkgagent.ApprovalDenied}},
	}, calls)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
}

func TestNormalizeDecisionsMore(t *testing.T) {
	calls := []pkgagent.ApprovalToolCall{{ID: "a"}, {ID: "b"}}
	got, err := normalizeDecisions(DecideApprovalRequest{Status: pkgagent.ApprovalApproved, Reason: "ok"}, calls)
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := normalizeDecisions(DecideApprovalRequest{}, calls); err == nil {
		t.Fatal("empty")
	}
	if _, err := normalizeDecisions(DecideApprovalRequest{
		Decisions: []ToolDecision{{ToolCallID: "a", Status: "weird"}, {ToolCallID: "b", Status: pkgagent.ApprovalDenied}},
	}, calls); err == nil {
		t.Fatal("invalid")
	}
	if _, err := normalizeDecisions(DecideApprovalRequest{
		Decisions: []ToolDecision{{ToolCallID: "z", Status: pkgagent.ApprovalApproved}, {ToolCallID: "a", Status: pkgagent.ApprovalDenied}},
	}, calls); err == nil {
		t.Fatal("unknown")
	}
	if _, err := normalizeDecisions(DecideApprovalRequest{
		Decisions: []ToolDecision{{ToolCallID: "a", Status: pkgagent.ApprovalApproved}, {ToolCallID: "a", Status: pkgagent.ApprovalApproved}},
	}, calls); err == nil {
		t.Fatal("dup")
	}
	mixed := fillBatchDecisions([]ToolDecision{
		{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
		{ToolCallID: "b", Status: pkgagent.ApprovalDenied},
	}, calls)
	if len(mixed) != 2 {
		t.Fatal(mixed)
	}
	gotFill := fillBatchDecisions([]ToolDecision{
		{ToolCallID: "", Status: pkgagent.ApprovalApproved},
		{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
		{ToolCallID: "a", Status: pkgagent.ApprovalApproved},
	}, calls)
	if len(gotFill) != 2 {
		t.Fatal(gotFill)
	}
}

func TestGitHelpers(t *testing.T) {
	if mapGitErr(nil) != nil {
		t.Fatal("nil")
	}
	if !cderr.IsInvalid(mapGitErr(git.ErrNotRepo)) || !cderr.IsInvalid(mapGitErr(git.ErrCurrentBranch)) {
		t.Fatal("invalid")
	}
	if !cderr.IsConflict(mapGitErr(git.ErrConflict)) || !cderr.IsConflict(mapGitErr(git.ErrDirty)) || !cderr.IsConflict(mapGitErr(git.ErrIntegrating)) {
		t.Fatal("conflict")
	}
	if !cderr.IsNotFound(mapGitErr(cderr.NotFound("x"))) {
		t.Fatal("passthrough")
	}
	if !cderr.IsInvalid(mapGitErr(errors.New("plain"))) {
		t.Fatal("generic mapped")
	}

	if !sameCheckout(".", ".") || sameCheckout("a", "b") {
		t.Fatal("sameCheckout")
	}
	if _, err := checkoutRelPath("/tmp", "../x"); err == nil {
		t.Fatal("escape")
	}
	if _, err := checkoutRelPath("/tmp", "ok.txt"); err != nil {
		t.Fatal(err)
	}

	api := &API{cfg: config.Config{}}
	if root, err := api.gitRoot(nil); err != nil || root == "" {
		t.Fatal(root, err)
	}
	api.cfg.GitRepo = t.TempDir()
	if root, err := api.gitRoot(nil); err != nil || root == "" {
		t.Fatal(root, err)
	}
	_ = api.gitModel()

	if _, ok := snapshotByID(nil, "x"); ok {
		t.Fatal("missing snap")
	}
	items := []AgentSnapshot{{ID: "s1", Checkout: git.Checkout{Path: "/a"}}}
	if got, ok := snapshotByID(items, "s1"); !ok || got.ID != "s1" {
		t.Fatal(got)
	}
	if latestSnapshot(items, "/nope").ID != "" {
		t.Fatal("latest miss")
	}
	if latestSnapshot(items, "").ID != "s1" {
		t.Fatal("latest empty checkout")
	}
	if !hasUntrackedFiles(git.SiteState{Files: []git.FileStatus{{WorktreeStatus: "?"}}}) {
		t.Fatal("untracked")
	}
	if hasUntrackedFiles(git.SiteState{Files: []git.FileStatus{{WorktreeStatus: "M"}}}) {
		t.Fatal("tracked")
	}

	buttons := undoButtons(git.SiteState{
		Empty: false, Head: "h1", Upstream: "origin/main", Ahead: 0,
		Files:       []git.FileStatus{{Path: "a.txt", WorktreeStatus: "M"}, {Path: "n.txt", WorktreeStatus: "?"}},
		Integrating: "merge",
	}, git.Graph{Nodes: []git.GraphNode{{Commit: git.Commit{ID: "h1", Parents: []string{"p"}}}}}, AgentSnapshot{ID: "snap", HasUntracked: true})
	if len(buttons) < 4 {
		t.Fatalf("%+v", buttons)
	}
	_ = pkgagent.WorkAgent
}

func TestMapAndAPIHelpers(t *testing.T) {
	if wrapHandlerDB(nil) != nil {
		t.Fatal("nil")
	}
	if !cderr.IsNotFound(wrapHandlerDB(sql.ErrNoRows)) {
		t.Fatal("norows")
	}
	if wrapHandlerDB(errors.New("x")).Error() != "x" {
		t.Fatal("passthrough")
	}
	if nullString("").Valid || !nullString("a").Valid {
		t.Fatal("nullString")
	}
	if deref(nil) != "" {
		t.Fatal("deref nil")
	}
	s := "x"
	if deref(&s) != "x" {
		t.Fatal("deref")
	}
	if string(rawJSON("")) != "null" || string(rawJSON(`{}`)) != "{}" {
		t.Fatal("rawJSON")
	}
	if !parseTime("nope").IsZero() {
		t.Fatal("parse")
	}
	_ = mapUsage(sqlite.UsageRecord{ID: "u1", RawProviderUsage: sql.NullString{}, CreatedAt: time.Now().Format(time.RFC3339)})
	_ = mapApproval(sqlite.Approval{ToolCallID: "c1"})
	_ = mapApproval(sqlite.Approval{ToolCalls: `[{"id":"c2"}]`})
	_ = mapMessage(sqlite.Message{Attachments: sql.NullString{String: `[{"id":"a"}]`, Valid: true}, ToolCalls: sql.NullString{String: `[{"id":"c"}]`, Valid: true}})
	_ = mapRun(sqlite.Run{Config: `{"approval":"manual"}`, StopReason: sql.NullString{String: "completed", Valid: true}, StartedAt: sql.NullString{String: time.Now().Format(time.RFC3339), Valid: true}})
	if ptrTime(sql.NullString{}) != nil {
		t.Fatal("empty time")
	}
	if ptrTime(sql.NullString{String: "bad", Valid: true}) != nil {
		t.Fatal("bad time")
	}

	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusNoContent, nil)
	writeError(rec, cderr.NotFound("n"))
	writeError(rec, cderr.Conflict("c"))
	writeError(rec, cderr.Invalid("i"))
	writeError(rec, cderr.Unauthorized("u"))
	writeError(rec, cderr.Unavailable("v"))
	writeError(rec, errors.New("e"))
	gone := httptest.NewRecorder()
	writeError(gone, context.Canceled)
	if gone.Code != http.StatusOK || gone.Body.Len() != 0 {
		t.Fatalf("canceled wrote %d %s", gone.Code, gone.Body.String())
	}
	secretRun := mapRun(sqlite.Run{Config: `{"model":{"options":{"api_key":"sk-secret","base_url":"https://api.deepseek.com"}}}`})
	if strings.Contains(string(secretRun.Config.Model.Options), "sk-secret") {
		t.Fatal("run config leaked api_key")
	}
	secretEv := mapEvent(sqlite.AgentEvent{Payload: `{"config":{"model":{"options":{"api_key":"sk-secret"}}}}`})
	if strings.Contains(string(secretEv.Payload), "sk-secret") {
		t.Fatal("event leaked api_key")
	}
	if !bytes.Contains(redactJSONSecrets(json.RawMessage(`{"api_key":"x"}`)), []byte(`"***"`)) {
		t.Fatal("redact")
	}
	writeSSE(rec, pkgagent.AgentEvent{Seq: 1, Type: pkgagent.EventRunCreated})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{"))
	if err := decodeJSON(req, &map[string]any{}); err == nil {
		t.Fatal("bad json")
	}
	empty := httptest.NewRequest(http.MethodPost, "/", nil)
	empty.Body = nil
	if err := decodeJSON(empty, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	after := httptest.NewRequest(http.MethodGet, "/x", nil)
	after.Header.Set("Last-Event-ID", "7")
	if parseAfterSeq(after) != 7 {
		t.Fatal(parseAfterSeq(after))
	}

	var nilAPI *API
	_ = nilAPI.logger()
	api := &API{bus: events.New(), log: nil}
	_ = api.logger()
	api.publishEvent(pkgagent.AgentEvent{})
	api.publishEvent(pkgagent.AgentEvent{EventID: "e1", SessionID: "s", Type: pkgagent.EventRunCreated})
	if api.q(httptest.NewRequest(http.MethodGet, "/", nil).Context()) != nil {
		t.Fatal("nil queries")
	}
	if api.runNeedsRecover(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "x") {
		t.Fatal("no runtime")
	}
	Health(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
}
