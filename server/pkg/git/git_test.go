package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func writeRepoFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initRepo(t *testing.T) (Repo, Checkout) {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.name", "tester")
	gitRun(t, dir, "config", "user.email", "tester@example.com")
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo, Checkout{Path: repo.Path}
}

func commitFile(t *testing.T, repo Repo, co Checkout, name, body, message string) Commit {
	t.Helper()
	writeRepoFile(t, co.Path, name, body)
	if err := Stage(repo, co, []string{name}); err != nil {
		t.Fatal(err)
	}
	commit, err := CreateCommit(repo, co, message)
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func TestOpenAndStatusNotRepo(t *testing.T) {
	repo, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state, err := Status(repo, Checkout{Path: repo.Path})
	if err != nil {
		t.Fatal(err)
	}
	if state.IsRepo {
		t.Fatal("expected non-repo")
	}
	if state.Path == "" {
		t.Fatal("path")
	}
}

func TestStatusListsNestedUntrackedFiles(t *testing.T) {
	repo, co := initRepo(t)
	writeRepoFile(t, co.Path, "pkg/foo/a.txt", "x")
	writeRepoFile(t, co.Path, "pkg/foo/b.txt", "y")
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 2 {
		t.Fatalf("nested untracked: %+v", state.Files)
	}
	got := map[string]bool{}
	for _, file := range state.Files {
		got[file.Path] = file.WorktreeStatus == "?"
	}
	if !got["pkg/foo/a.txt"] || !got["pkg/foo/b.txt"] {
		t.Fatalf("expected files under pkg/foo: %+v", state.Files)
	}
}

func TestStatusEmptyAndFirstCommit(t *testing.T) {
	repo, co := initRepo(t)
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if !state.IsRepo || !state.Empty || state.Branch != "main" || state.Detached {
		t.Fatalf("empty status: %+v", state)
	}
	writeRepoFile(t, co.Path, "a.txt", "hello")
	state, err = Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 1 || state.Files[0].Path != "a.txt" || state.Files[0].WorktreeStatus != "?" {
		t.Fatalf("untracked: %+v", state.Files)
	}
	if err := Stage(repo, co, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	state, err = Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 1 || state.Files[0].StagedStatus != "A" {
		t.Fatalf("staged: %+v", state.Files)
	}
	commit, err := CreateCommit(repo, co, "add a")
	if err != nil {
		t.Fatal(err)
	}
	if commit.ID == "" || commit.Title != "add a" || commit.Author == "" {
		t.Fatalf("commit: %+v", commit)
	}
	state, err = Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if state.Empty || state.Head != commit.ID || len(state.Files) != 0 {
		t.Fatalf("after commit: %+v", state)
	}
}

func TestStageUnstageDiffRename(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "old.txt", "v1", "first")
	writeRepoFile(t, co.Path, "old.txt", "v2")
	files, err := Diff(repo, co, "worktree")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Kind != "modified" || files[0].Patch == "" {
		t.Fatalf("worktree diff: %+v", files)
	}
	if err := Stage(repo, co, []string{"old.txt"}); err != nil {
		t.Fatal(err)
	}
	staged, err := Diff(repo, co, "staged")
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 1 || staged[0].Kind != "modified" {
		t.Fatalf("staged diff: %+v", staged)
	}
	if err := Unstage(repo, co, []string{"old.txt"}); err != nil {
		t.Fatal(err)
	}
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 1 || letterDirty(state.Files[0].StagedStatus) {
		t.Fatalf("unstage: %+v", state.Files)
	}
	if err := RestorePath(repo, co, "old.txt"); err != nil {
		t.Fatal(err)
	}
	gitRun(t, co.Path, "mv", "old.txt", "new.txt")
	state, err = Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range state.Files {
		if file.Path == "new.txt" && file.OrigPath == "old.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rename missing orig: %+v", state.Files)
	}
}

func TestBranchesAndGraph(t *testing.T) {
	repo, co := initRepo(t)
	first := commitFile(t, repo, co, "a.txt", "1", "first")
	if err := CreateBranch(repo, co, "feature", first.ID); err != nil {
		t.Fatal(err)
	}
	if err := SwitchBranch(repo, co, "feature"); err != nil {
		t.Fatal(err)
	}
	second := commitFile(t, repo, co, "b.txt", "2", "second")
	branches, err := ListBranches(repo)
	if err != nil {
		t.Fatal(err)
	}
	var feature, main Branch
	for _, b := range branches {
		if b.Name == "feature" {
			feature = b
		}
		if b.Name == "main" {
			main = b
		}
	}
	if !feature.IsCurrent || feature.Head != second.ID || main.IsCurrent {
		t.Fatalf("branches: %+v", branches)
	}
	graph, err := LogGraph(repo, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) < 2 || len(graph.Edges) < 1 {
		t.Fatalf("graph: %+v", graph)
	}
	if err := SwitchBranch(repo, co, "main"); err != nil {
		t.Fatal(err)
	}
	if err := CreateBranch(repo, co, "extra", ""); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(repo, "extra"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(repo, "main"); err != ErrCurrentBranch {
		t.Fatalf("delete current: %v", err)
	}
}

func TestSwitchDirtyAndWorktreeBusy(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "1", "first")
	if err := CreateBranch(repo, co, "feature", ""); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, co.Path, "a.txt", "dirty")
	if err := SwitchBranch(repo, co, "feature"); err != ErrDirty {
		t.Fatalf("dirty: %v", err)
	}
	if err := RestorePath(repo, co, "a.txt"); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(repo.Path), "wt")
	tree, err := AddWorktree(repo, other, "feature", "")
	if err != nil {
		t.Fatal(err)
	}
	if tree.Branch != "feature" || tree.Path == "" {
		t.Fatalf("worktree: %+v", tree)
	}
	if err := SwitchBranch(repo, co, "feature"); err == nil {
		t.Fatal("expected busy branch")
	}
	trees, err := ListWorktrees(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(trees) != 2 {
		t.Fatalf("worktrees: %+v", trees)
	}
}

func TestCreateBranchUsesCheckoutHEAD(t *testing.T) {
	repo, co := initRepo(t)
	mainCommit := commitFile(t, repo, co, "a.txt", "1", "first")
	if err := CreateBranch(repo, co, "feature", ""); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(repo.Path), "wt")
	if _, err := AddWorktree(repo, other, "feature", ""); err != nil {
		t.Fatal(err)
	}
	wt := Checkout{Path: other}
	featureCommit := commitFile(t, repo, wt, "a.txt", "2", "on feature")
	if err := CreateBranch(repo, wt, "from-wt", ""); err != nil {
		t.Fatal(err)
	}
	branches, err := ListBranches(repo)
	if err != nil {
		t.Fatal(err)
	}
	var fromWT Branch
	for _, b := range branches {
		if b.Name == "from-wt" {
			fromWT = b
		}
	}
	if fromWT.Head != featureCommit.ID {
		t.Fatalf("from-wt head %s want checkout %s (not main %s)", fromWT.Head, featureCommit.ID, mainCommit.ID)
	}
}

func TestLogListsCurrentBranchNewestFirst(t *testing.T) {
	repo, co := initRepo(t)
	first := commitFile(t, repo, co, "a.txt", "1", "first")
	second := commitFile(t, repo, co, "a.txt", "2", "second")
	commits, err := Log(repo, co, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].ID != second.ID || commits[1].ID != first.ID {
		t.Fatalf("log: %+v", commits)
	}
	if commits[0].Title != "second" || commits[0].Author == "" || commits[0].Date == "" {
		t.Fatalf("fields: %+v", commits[0])
	}
}

func TestDiscardRestoresTrackedAndDeletesUntracked(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "base", "first")
	writeRepoFile(t, co.Path, "a.txt", "dirty")
	writeRepoFile(t, co.Path, "pkg/foo/new.txt", "untracked")
	if err := Discard(repo, co, []string{"a.txt", "pkg/foo/new.txt"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(co.Path, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "base" {
		t.Fatalf("tracked restore: %q", body)
	}
	if _, err := os.Stat(filepath.Join(co.Path, "pkg/foo/new.txt")); !os.IsNotExist(err) {
		t.Fatalf("untracked still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(co.Path, "pkg")); !os.IsNotExist(err) {
		t.Fatalf("empty untracked dirs remain: %v", err)
	}
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 0 {
		t.Fatalf("status after discard: %+v", state.Files)
	}
}

func TestDiscardKeepsStaged(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "base", "first")
	writeRepoFile(t, co.Path, "a.txt", "staged")
	if err := Stage(repo, co, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, co.Path, "a.txt", "worktree")
	if err := Discard(repo, co, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(co.Path, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "staged" {
		t.Fatalf("worktree should match index: %q", body)
	}
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 1 || state.Files[0].StagedStatus != "M" || letterDirty(state.Files[0].WorktreeStatus) {
		t.Fatalf("keep staged: %+v", state.Files)
	}
}

func TestResetAndRevert(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "1", "first")
	second := commitFile(t, repo, co, "a.txt", "2", "second")
	if err := Reset(repo, co, "HEAD~1", "soft"); err != nil {
		t.Fatal(err)
	}
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if state.Head == second.ID {
		t.Fatal("soft reset kept same head")
	}
	if _, err := CreateCommit(repo, co, "second again"); err != nil {
		t.Fatal(err)
	}
	head, err := readCommit(co.Path, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := Revert(repo, co, head)
	if err != nil {
		t.Fatal(err)
	}
	if reverted.ID == "" || reverted.ID == head.ID {
		t.Fatalf("revert: %+v", reverted)
	}
}

func TestCaptureRestoreWork(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "base", "first")
	writeRepoFile(t, co.Path, "a.txt", "changed")
	if err := Stage(repo, co, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, co.Path, "a.txt", "changed more")
	oid, err := CaptureWork(repo, co, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if oid == "" {
		t.Fatal("expected stash oid")
	}
	after, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Files) == 0 {
		t.Fatal("capture must not move work")
	}
	head := after.Head
	writeRepoFile(t, co.Path, "a.txt", "lost")
	if err := RestoreWork(repo, co, oid, head); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(co.Path, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "changed more" {
		t.Fatalf("restored %q", body)
	}
}

func TestMergeConflictReadContinueAbort(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "base\n", "base")
	if err := CreateBranch(repo, co, "other", ""); err != nil {
		t.Fatal(err)
	}
	if err := SwitchBranch(repo, co, "other"); err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, co, "a.txt", "theirs\n", "theirs")
	if err := SwitchBranch(repo, co, "main"); err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, co, "a.txt", "ours\n", "ours")
	if _, err := runGit(co.Path, "merge", "other"); err == nil {
		t.Fatal("expected merge conflict")
	}
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if state.Integrating != "merge" || !hasUnmerged(state) {
		t.Fatalf("integrating: %+v", state)
	}
	item, err := ReadConflict(repo, co, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != "both_modified" || !strings.Contains(item.Ours, "ours") || !strings.Contains(item.Theirs, "theirs") {
		t.Fatalf("conflict: %+v", item)
	}
	ours, theirs, err := ConflictNames(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if ours == "" || theirs == "" {
		t.Fatalf("names %q %q", ours, theirs)
	}
	if err := AbortIntegrate(repo, co); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(co.Path, "merge", "other"); err == nil {
		t.Fatal("expected merge conflict again")
	}
	writeRepoFile(t, co.Path, "a.txt", "resolved\n")
	if err := Stage(repo, co, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := ContinueIntegrate(repo, co); err != nil {
		t.Fatal(err)
	}
	state, err = Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if state.Integrating != "" || hasUnmerged(state) {
		t.Fatalf("after continue: %+v", state)
	}
}

func TestPushPullRemote(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "1", "first")
	if err := Push(context.Background(), repo, co); err == nil {
		t.Fatal("push without remote")
	}
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare", "-b", "main")
	gitRun(t, co.Path, "remote", "add", "origin", bare)
	remotes, err := ListRemotes(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 1 || remotes[0].Name != "origin" || remotes[0].FetchURL != bare {
		t.Fatalf("remotes: %+v", remotes)
	}
	if err := Push(context.Background(), repo, co); err == nil {
		t.Fatal("push without upstream")
	}
	gitRun(t, co.Path, "push", "-u", "origin", "main")
	state, err := Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if state.Upstream != "origin/main" {
		t.Fatalf("upstream: %+v", state)
	}
	commitFile(t, repo, co, "a.txt", "2", "second")
	state, err = Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if state.Ahead != 1 {
		t.Fatalf("ahead: %+v", state)
	}
	if err := Push(context.Background(), repo, co); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	otherDir := filepath.Join(parent, "clone")
	gitRun(t, parent, "clone", bare, otherDir)
	gitRun(t, otherDir, "config", "user.name", "tester")
	gitRun(t, otherDir, "config", "user.email", "tester@example.com")
	other, err := Open(otherDir)
	if err != nil {
		t.Fatal(err)
	}
	otherCo := Checkout{Path: other.Path}
	commitFile(t, other, otherCo, "a.txt", "3", "third")
	if err := Push(context.Background(), other, otherCo); err != nil {
		t.Fatal(err)
	}
	if err := Pull(context.Background(), repo, co); err != nil {
		t.Fatal(err)
	}
	state, err = Status(repo, co)
	if err != nil {
		t.Fatal(err)
	}
	if state.Behind != 0 || state.Ahead != 0 {
		t.Fatalf("after pull: %+v", state)
	}
}

func TestMutationsRequireRepo(t *testing.T) {
	repo, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	co := Checkout{Path: repo.Path}
	if err := Stage(repo, co, []string{"a.txt"}); err != ErrNotRepo {
		t.Fatalf("stage: %v", err)
	}
}

func TestAddWorktreeRejectsEscape(t *testing.T) {
	repo, _ := initRepo(t)
	escape := filepath.Join(filepath.Dir(filepath.Dir(repo.Path)), "codedock-wt-escape")
	if _, err := AddWorktree(repo, escape, "feature", ""); err == nil {
		t.Fatal("expected path outside parent to fail")
	}
}

func TestRestoreWorkRejectsMissingSnapshot(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "base", "first")
	writeRepoFile(t, co.Path, "a.txt", "dirty")
	before, err := os.ReadFile(filepath.Join(co.Path, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := RestoreWork(repo, co, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "HEAD"); err == nil {
		t.Fatal("expected missing snapshot to fail")
	}
	after, err := os.ReadFile(filepath.Join(co.Path, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("worktree changed after failed restore: %q", after)
	}
}

func TestUntrackedDiffSkipsLargeFile(t *testing.T) {
	repo, co := initRepo(t)
	commitFile(t, repo, co, "a.txt", "1", "first")
	big := filepath.Join(co.Path, "huge.bin")
	if err := os.WriteFile(big, bytes.Repeat([]byte("x"), maxUntrackedPatchBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Diff(repo, co, "worktree")
	if err != nil {
		t.Fatal(err)
	}
	var found DiffFile
	for _, file := range files {
		if file.Path == "huge.bin" {
			found = file
		}
	}
	if !found.Binary || found.Patch != "" {
		t.Fatalf("large untracked should skip patch: %+v", found)
	}
}
