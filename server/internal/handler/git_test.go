package handler_test

import (
	"bytes"
	"codedock/internal/config"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true", "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-b", "main")
	gitCmd(t, dir, "config", "user.name", "tester")
	gitCmd(t, dir, "config", "user.email", "tester@example.com")
	return dir
}

func newGitAPI(t *testing.T, repo string) *handler.API {
	t.Helper()
	t.Setenv("GIT_REPO", repo)
	t.Setenv("LLM_PROVIDER", "fake")
	t.Setenv("LLM_MODEL", "fake")
	return handler.New(nil, nil, nil, nil, pkgagent.RunConfigSnapshot{}, config.Load(), nil)
}

func gitRouter(api *handler.API) http.Handler {
	r := chi.NewRouter()
	r.Get("/git/status", api.GitStatus)
	r.Get("/git/diff", api.GitDiff)
	r.Get("/git/graph", api.GitGraph)
	r.Get("/git/log", api.GitLog)
	r.Post("/git/stage", api.GitStage)
	r.Post("/git/unstage", api.GitUnstage)
	r.Post("/git/discard", api.GitDiscard)
	r.Post("/git/commit", api.GitCommit)
	r.Post("/git/reset", api.GitReset)
	r.Post("/git/revert", api.GitRevert)
	r.Post("/git/push", api.GitPush)
	r.Post("/git/pull", api.GitPull)
	r.Get("/git/remotes", api.GitListRemotes)
	r.Get("/git/worktrees", api.GitListWorktrees)
	r.Post("/git/worktrees", api.GitAddWorktree)
	r.Get("/git/branches", api.GitListBranches)
	r.Post("/git/branches", api.GitCreateBranch)
	r.Post("/git/branches/switch", api.GitSwitchBranch)
	r.Delete("/git/branches", api.GitDeleteBranch)
	r.Get("/git/conflict", api.GitGetConflict)
	r.Post("/git/conflict/write", api.GitWriteConflict)
	r.Post("/git/conflict/continue", api.GitContinueConflict)
	r.Post("/git/conflict/abort", api.GitAbortConflict)
	r.Get("/git/commit-message/prompt", api.GitGetPrompt)
	r.Put("/git/commit-message/prompt", api.GitSetPrompt)
	r.Post("/git/commit-message/generate", api.GitGenerateMessage)
	r.Post("/git/stash", api.GitCreateSnapshot)
	r.Get("/git/stash/latest", api.GitLatestSnapshot)
	r.Post("/git/stash/restore", api.GitRestoreSnapshot)
	r.Get("/git/undo", api.GitListUndo)
	r.Post("/git/undo", api.GitClickUndo)
	return r
}

func doGit(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func decodeGit[T any](t *testing.T, rec *httptest.ResponseRecorder, dest *T) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}

func TestGitStatusUsesSessionWorkspace(t *testing.T) {
	f := newFixture(t)
	ws := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(ws, "sess.txt"), []byte("from-session"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{UserID: "u1", WorkspaceID: ws})
	if rec.Code != http.StatusOK {
		t.Fatalf("create session %d %s", rec.Code, rec.Body.String())
	}
	var created handler.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	r := gitRouter(f.api)
	got := doGit(t, r, http.MethodGet, "/git/status?session_id="+created.Session.ID, "")
	if got.Code != http.StatusOK {
		t.Fatalf("status %d %s", got.Code, got.Body.String())
	}
	var state struct {
		Path   string `json:"path"`
		IsRepo bool   `json:"is_repo"`
	}
	decodeGit(t, got, &state)
	want, _ := filepath.Abs(ws)
	if state.Path != ws && state.Path != want {
		t.Fatalf("path %q want %q", state.Path, want)
	}
	if !state.IsRepo {
		t.Fatal("expected session workspace to be the git repo")
	}
	missing := doGit(t, r, http.MethodGet, "/git/status?session_id=missing", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing session %d %s", missing.Code, missing.Body.String())
	}
}

func TestGitSkeletonRoutes(t *testing.T) {
	t.Setenv("GIT_REPO", t.TempDir())
	api := handler.New(nil, nil, nil, nil, pkgagent.RunConfigSnapshot{}, config.Load(), nil)
	r := gitRouter(api)
	for _, path := range []string{"/git/status", "/git/branches", "/git/undo", "/git/conflict"} {
		rec := doGit(t, r, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestGitStatusCommitBranchUndo(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	api := newGitAPI(t, dir)
	r := gitRouter(api)

	rec := doGit(t, r, http.MethodGet, "/git/status", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var status map[string]any
	decodeGit(t, rec, &status)
	if status["is_repo"] != true || status["empty"] != true {
		t.Fatalf("status: %s", rec.Body.String())
	}

	rec = doGit(t, r, http.MethodPost, "/git/stage", `{"paths":["a.txt"]}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = doGit(t, r, http.MethodPost, "/git/commit-message/generate", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var draft handler.MessageDraft
	decodeGit(t, rec, &draft)
	if draft.Title == "" {
		t.Fatal("draft title")
	}
	rec = doGit(t, r, http.MethodPost, "/git/commit", `{"message":"add a","paths":["a.txt"]}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}

	rec = doGit(t, r, http.MethodGet, "/git/branches", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var view handler.BranchView
	decodeGit(t, rec, &view)
	if view.Current != "main" || len(view.Locals) != 1 {
		t.Fatalf("branches: %+v", view)
	}

	rec = doGit(t, r, http.MethodPost, "/git/branches", `{"name":"feature"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = doGit(t, r, http.MethodPost, "/git/branches/switch", `{"name":"feature"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = doGit(t, r, http.MethodPost, "/git/commit", `{"message":"add b","paths":["b.txt"]}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}

	rec = doGit(t, r, http.MethodGet, "/git/undo", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var undo struct {
		Buttons []handler.UndoButton `json:"buttons"`
	}
	decodeGit(t, rec, &undo)
	foundLast := false
	for _, b := range undo.Buttons {
		if b.ID == "last_commit" {
			foundLast = true
		}
	}
	if !foundLast {
		t.Fatalf("undo buttons: %+v", undo.Buttons)
	}
	rec = doGit(t, r, http.MethodPost, "/git/undo", `{"id":"last_commit"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}

	rec = doGit(t, r, http.MethodPost, "/git/reset", `{"target":"HEAD","mode":"hard"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reset without confirm: %d %s", rec.Code, rec.Body.String())
	}
	rec = doGit(t, r, http.MethodPost, "/git/reset", `{"target":"HEAD","mode":"hard","confirm":true}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
}

func TestGitConflictAbortAndPromptSnapshot(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "base")
	gitCmd(t, dir, "checkout", "-b", "other")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "theirs")
	gitCmd(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "ours")
	cmd := exec.Command("git", "merge", "other")
	cmd.Dir = dir
	_ = cmd.Run()

	api := newGitAPI(t, dir)
	r := gitRouter(api)
	rec := doGit(t, r, http.MethodGet, "/git/conflict", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var sess handler.ConflictSession
	decodeGit(t, rec, &sess)
	if sess.Kind != "merge" || len(sess.Items) != 1 || sess.Items[0].Kind != "both_modified" {
		t.Fatalf("conflict: %+v", sess)
	}
	rec = doGit(t, r, http.MethodPost, "/git/conflict/write", `{"path":"a.txt","result":"resolved\n"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = doGit(t, r, http.MethodPost, "/git/conflict/continue", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}

	rec = doGit(t, r, http.MethodPut, "/git/commit-message/prompt", `{"selected":"custom","custom":"写短标题"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = doGit(t, r, http.MethodGet, "/git/commit-message/prompt", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var prompt handler.PromptConfig
	decodeGit(t, rec, &prompt)
	if prompt.Selected != "custom" || prompt.Custom != "写短标题" || prompt.SystemPrompt != "写短标题" || len(prompt.Presets) != 2 {
		t.Fatalf("prompt: %+v", prompt)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("snap\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = doGit(t, r, http.MethodPost, "/git/stash", `{"agent_run":"run-1"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var snap handler.AgentSnapshot
	decodeGit(t, rec, &snap)
	if snap.ID == "" || snap.StashOID == "" || snap.AgentRun != "run-1" {
		t.Fatalf("snap: %+v", snap)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("lost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = doGit(t, r, http.MethodGet, "/git/stash/latest", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = doGit(t, r, http.MethodPost, "/git/stash/restore", `{"id":"`+snap.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "snap\n" {
		t.Fatalf("restored %q", body)
	}
}

func TestGitLog(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "add a")
	api := newGitAPI(t, dir)
	r := gitRouter(api)
	rec := doGit(t, r, http.MethodGet, "/git/log?limit=10", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var body struct {
		Commits []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"commits"`
	}
	decodeGit(t, rec, &body)
	if len(body.Commits) != 1 || body.Commits[0].Title != "add a" || body.Commits[0].ID == "" {
		t.Fatalf("log: %+v", body.Commits)
	}
}

func TestGitDiscard(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	api := newGitAPI(t, dir)
	r := gitRouter(api)
	rec := doGit(t, r, http.MethodPost, "/git/discard", `{"paths":["a.txt","pkg/new.txt"]}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "base" {
		t.Fatalf("tracked: %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "pkg", "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("untracked: %v", err)
	}
}

func TestGitInvalidCheckout(t *testing.T) {
	dir := initGitRepo(t)
	api := newGitAPI(t, dir)
	r := gitRouter(api)
	rec := doGit(t, r, http.MethodGet, "/git/status?checkout=/tmp/not-a-worktree", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
}

func TestGitUndoRejectsOtherCheckoutSnapshot(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "base")
	gitCmd(t, dir, "branch", "feature")
	wt := filepath.Join(t.TempDir(), "wt")
	gitCmd(t, dir, "worktree", "add", wt, "feature")

	api := newGitAPI(t, dir)
	r := gitRouter(api)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("main-change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := doGit(t, r, http.MethodPost, "/git/stash", `{"agent_run":"run-2"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var snap handler.AgentSnapshot
	decodeGit(t, rec, &snap)
	if snap.ID == "" {
		t.Fatal("snapshot id")
	}
	body := `{"id":"agent_stash:` + snap.ID + `","checkout":` + jsonQuote(wt) + `}`
	rec = doGit(t, r, http.MethodPost, "/git/undo", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("undo other checkout: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGitCommitMessagePromptPresets(t *testing.T) {
	dir := initGitRepo(t)
	api := newGitAPI(t, dir)
	r := gitRouter(api)

	rec := doGit(t, r, http.MethodGet, "/git/commit-message/prompt", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var prompt handler.PromptConfig
	decodeGit(t, rec, &prompt)
	if prompt.Selected != "conventional" || prompt.SystemPrompt == "" || len(prompt.Presets) != 2 {
		t.Fatalf("default prompt: %+v", prompt)
	}

	rec = doGit(t, r, http.MethodPut, "/git/commit-message/prompt", `{"selected":"nope","custom":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid id: %d %s", rec.Code, rec.Body.String())
	}

	rec = doGit(t, r, http.MethodPut, "/git/commit-message/prompt", `{"selected":"conventional","custom":"keep me"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	decodeGit(t, rec, &prompt)
	if prompt.Selected != "conventional" || prompt.Custom != "keep me" || !strings.Contains(prompt.SystemPrompt, "type(scope)") || !strings.Contains(prompt.SystemPrompt, `- "`) {
		t.Fatalf("conventional: %+v", prompt)
	}

	if err := os.MkdirAll(filepath.Join(dir, ".git", "codedock"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "codedock", "commit-message.json"), []byte(`{"selected":"standard","custom":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = doGit(t, r, http.MethodGet, "/git/commit-message/prompt", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	decodeGit(t, rec, &prompt)
	if prompt.Selected != "conventional" || !strings.Contains(prompt.SystemPrompt, "type(scope)") {
		t.Fatalf("legacy standard: %+v", prompt)
	}

	if err := os.MkdirAll(filepath.Join(dir, ".git", "codedock"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, ".git", "codedock", "commit-message-prompt")
	if err := os.WriteFile(legacy, []byte("旧提示词"), 0o644); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, ".git", "codedock", "commit-message.json")
	if err := os.Remove(jsonPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	rec = doGit(t, r, http.MethodGet, "/git/commit-message/prompt", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	decodeGit(t, rec, &prompt)
	if prompt.Selected != "custom" || prompt.Custom != "旧提示词" || prompt.SystemPrompt != "旧提示词" {
		t.Fatalf("migrate: %+v", prompt)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = doGit(t, r, http.MethodPost, "/git/commit-message/generate", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("nothing staged: %d %s", rec.Code, rec.Body.String())
	}
	rec = doGit(t, r, http.MethodPost, "/git/stage", `{"paths":["a.txt"]}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = doGit(t, r, http.MethodPost, "/git/commit-message/generate", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var draft handler.MessageDraft
	decodeGit(t, rec, &draft)
	if draft.Title == "" {
		t.Fatal("draft title")
	}
}

func TestGitPullConflictAndRemoteBranches(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "base")
	bare := t.TempDir()
	gitCmd(t, bare, "init", "--bare", "-b", "main")
	gitCmd(t, dir, "remote", "add", "origin", bare)
	gitCmd(t, dir, "push", "-u", "origin", "main")

	parent := t.TempDir()
	other := filepath.Join(parent, "clone")
	gitCmd(t, parent, "clone", "-b", "main", bare, other)
	gitCmd(t, other, "config", "user.name", "tester")
	gitCmd(t, other, "config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(other, "a.txt"), []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, other, "add", "a.txt")
	gitCmd(t, other, "commit", "-m", "theirs")
	gitCmd(t, other, "push", "origin", "HEAD:main")

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "ours")

	api := newGitAPI(t, dir)
	r := gitRouter(api)
	if rec := doGit(t, r, http.MethodGet, "/git/branches", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodPost, "/git/pull", `{}`); rec.Code != http.StatusConflict && rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("pull %d %s", rec.Code, rec.Body.String())
	}
}

func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

func TestGitRemainingRoutes(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "add a")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	api := newGitAPI(t, dir)
	r := gitRouter(api)

	if rec := doGit(t, r, http.MethodGet, "/git/diff?scope=worktree", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodGet, "/git/diff", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodGet, "/git/graph", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodGet, "/git/log?limit=bad", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit %d", rec.Code)
	}
	if rec := doGit(t, r, http.MethodPost, "/git/stage", `{"paths":["a.txt"]}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodPost, "/git/unstage", `{"paths":["a.txt"]}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodGet, "/git/remotes", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodGet, "/git/worktrees", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	wt := filepath.Join(t.TempDir(), "wt")
	if rec := doGit(t, r, http.MethodPost, "/git/worktrees", `{"path":`+jsonQuote(wt)+`,"new_branch":"wt-branch"}`); rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest && rec.Code != http.StatusConflict {
		t.Fatalf("worktree %d %s", rec.Code, rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodPost, "/git/push", `{}`); rec.Code == http.StatusOK {
		t.Fatal("push without remote should fail")
	}
	if rec := doGit(t, r, http.MethodPost, "/git/pull", `{}`); rec.Code == http.StatusOK {
		t.Fatal("pull without remote should fail")
	}
	if rec := doGit(t, r, http.MethodPost, "/git/revert", `{"id":"missing"}`); rec.Code == http.StatusOK {
		t.Fatal("revert missing")
	}
	if rec := doGit(t, r, http.MethodPost, "/git/reset", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("reset target %d", rec.Code)
	}
	if rec := doGit(t, r, http.MethodPost, "/git/reset", `{"target":"HEAD","mode":"weird"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("reset mode %d", rec.Code)
	}
	gitCmd(t, dir, "branch", "to-delete")
	if rec := doGit(t, r, http.MethodDelete, "/git/branches", `{"name":"to-delete"}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodPost, "/git/conflict/abort", `{}`); rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest && rec.Code != http.StatusConflict {
		t.Fatalf("abort %d %s", rec.Code, rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodPost, "/git/conflict/write", `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad write %d", rec.Code)
	}
	if rec := doGit(t, r, http.MethodGet, "/git/diff?checkout=/tmp/not-a-worktree", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("diff checkout %d", rec.Code)
	}

	_ = handler.MessageDraft{}
}

func TestGitMapErrAndRoot(t *testing.T) {
	t.Setenv("GIT_REPO", "")
	api := newGitAPI(t, t.TempDir())
	r := gitRouter(api)
	rec := doGit(t, r, http.MethodGet, "/git/status", "")
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestGitAllErrorBranches(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "add a")
	api := newGitAPI(t, dir)
	r := gitRouter(api)

	posts := []string{
		"/git/stage", "/git/unstage", "/git/discard", "/git/commit", "/git/reset",
		"/git/revert", "/git/push", "/git/pull", "/git/worktrees", "/git/branches",
		"/git/branches/switch", "/git/conflict/write", "/git/conflict/continue",
		"/git/conflict/abort", "/git/stash", "/git/stash/restore", "/git/undo",
	}
	for _, path := range posts {
		if rec := doGit(t, r, http.MethodPost, path, `{`); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s bad json %d %s", path, rec.Code, rec.Body.String())
		}
		method := http.MethodPost
		if path == "/git/branches" && false {
			method = http.MethodDelete
		}
		if rec := doGit(t, r, method, path, `{"checkout":"/tmp/not-a-worktree"}`); rec.Code != http.StatusBadRequest && rec.Code != http.StatusConflict {
			t.Fatalf("%s bad checkout %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if rec := doGit(t, r, http.MethodDelete, "/git/branches", `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete branch json %d", rec.Code)
	}
	if rec := doGit(t, r, http.MethodDelete, "/git/branches", `{"checkout":"/tmp/not-a-worktree","name":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete branch checkout %d", rec.Code)
	}
	for _, path := range []string{"/git/status", "/git/diff", "/git/graph", "/git/log", "/git/remotes", "/git/worktrees", "/git/branches", "/git/conflict", "/git/stash/latest", "/git/undo", "/git/commit-message/prompt"} {
		if rec := doGit(t, r, http.MethodGet, path+"?checkout=/tmp/not-a-worktree", ""); rec.Code != http.StatusBadRequest && rec.Code != http.StatusOK {
			t.Fatalf("GET %s %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if rec := doGit(t, r, http.MethodPut, "/git/commit-message/prompt", `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("set prompt json %d", rec.Code)
	}
	if rec := doGit(t, r, http.MethodPost, "/git/commit-message/generate", `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("generate json %d", rec.Code)
	}
}

func TestGitWriteConflictEscape(t *testing.T) {
	dir := initGitRepo(t)
	api := newGitAPI(t, dir)
	r := gitRouter(api)
	if rec := doGit(t, r, http.MethodPost, "/git/conflict/write", `{"path":"../escape","result":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("escape %d %s", rec.Code, rec.Body.String())
	}
}

func TestGitPushPullRevertUndoExtras(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "first")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "second")

	bare := t.TempDir()
	gitCmd(t, bare, "init", "--bare", "-b", "main")
	gitCmd(t, dir, "remote", "add", "origin", bare)
	gitCmd(t, dir, "push", "-u", "origin", "main")

	api := newGitAPI(t, dir)
	r := gitRouter(api)
	if rec := doGit(t, r, http.MethodGet, "/git/remotes", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodPost, "/git/push", `{}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := doGit(t, r, http.MethodPost, "/git/pull", `{}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}

	var logBody struct {
		Commits []struct {
			ID string `json:"id"`
		} `json:"commits"`
	}
	rec := doGit(t, r, http.MethodGet, "/git/log?limit=2", "")
	decodeGit(t, rec, &logBody)
	if len(logBody.Commits) == 0 {
		t.Fatal("log")
	}
	if rec = doGit(t, r, http.MethodPost, "/git/revert", `{"id":"`+logBody.Commits[0].ID+`"}`); rec.Code != http.StatusOK && rec.Code != http.StatusConflict && rec.Code != http.StatusBadRequest {
		t.Fatalf("revert %d %s", rec.Code, rec.Body.String())
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec = doGit(t, r, http.MethodPost, "/git/undo", `{"id":"uncommitted"}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec = doGit(t, r, http.MethodPost, "/git/undo", `{"id":"path:a.txt"}`); rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("path undo %d %s", rec.Code, rec.Body.String())
	}
	if rec = doGit(t, r, http.MethodPost, "/git/undo", `{"id":"unknown"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown undo %d", rec.Code)
	}
	if rec = doGit(t, r, http.MethodPost, "/git/undo", `{"id":"agent_stash:missing"}`); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("missing snap %d %s", rec.Code, rec.Body.String())
	}
	if rec = doGit(t, r, http.MethodGet, "/git/stash/latest", ""); rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Fatalf("latest %d %s", rec.Code, rec.Body.String())
	}
	if rec = doGit(t, r, http.MethodPost, "/git/stash/restore", `{"id":"missing"}`); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("restore %d %s", rec.Code, rec.Body.String())
	}

	gd := filepath.Join(dir, ".git", "codedock")
	if err := os.MkdirAll(gd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gd, "snapshots.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec = doGit(t, r, http.MethodGet, "/git/undo", ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
}
