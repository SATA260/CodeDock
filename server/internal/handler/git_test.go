package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"codedock/internal/config"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
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

	rec = doGit(t, r, http.MethodPut, "/git/commit-message/prompt", `{"system_prompt":"写短标题"}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = doGit(t, r, http.MethodGet, "/git/commit-message/prompt", "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var prompt handler.PromptConfig
	decodeGit(t, rec, &prompt)
	if prompt.SystemPrompt != "写短标题" {
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

func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
