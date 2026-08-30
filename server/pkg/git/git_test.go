package git

import "testing"

func TestSkeletonCompiles(t *testing.T) {
	repo, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	co := Checkout{Path: repo.Path}
	if _, err := Status(repo, co); err != nil {
		t.Fatal(err)
	}
	if _, err := Diff(repo, co, "staged"); err != nil {
		t.Fatal(err)
	}
	if _, err := LogGraph(repo, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := ListRemotes(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := ListWorktrees(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := AddWorktree(repo, t.TempDir(), "", "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := ListBranches(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureWork(repo, co, "note"); err != nil {
		t.Fatal(err)
	}
}
