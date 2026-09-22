package github

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestViewIssueAndPull(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "gh")
	if runtime.GOOS == "windows" {
		bin += ".bat"
		if err := os.WriteFile(bin, []byte("@echo {\"number\":15,\"title\":\"bug\",\"url\":\"https://x\",\"body\":\"repro\",\"state\":\"OPEN\"}\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		script := "#!/bin/sh\nprintf '%s\\n' '{\"number\":15,\"title\":\"bug\",\"url\":\"https://x\",\"body\":\"repro\",\"state\":\"OPEN\"}'\n"
		if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	issue, err := ViewIssue(context.Background(), "acme/app", 15)
	if err != nil {
		t.Fatal(err)
	}
	if issue.Number != 15 || issue.Title != "bug" || issue.Repo != "acme/app" {
		t.Fatalf("issue=%+v", issue)
	}
	pull, err := ViewPull(context.Background(), "acme/app", 15)
	if err != nil || pull.Title != "bug" {
		t.Fatalf("pull=%+v %v", pull, err)
	}
	if _, err := ViewIssue(context.Background(), "acme/app", 0); err == nil {
		t.Fatal("zero number")
	}
}

func TestFormatRef(t *testing.T) {
	repo, number, err := FormatRef("acme/app#15")
	if err != nil || repo != "acme/app" || number != 15 {
		t.Fatalf("ref %s #%d %v", repo, number, err)
	}
	if _, _, err := FormatRef(""); err == nil {
		t.Fatal("empty ref")
	}
}

func TestParseLink(t *testing.T) {
	repo, number, kind, err := ParseLink("https://github.com/acme/app/issues/15")
	if err != nil || repo != "acme/app" || number != 15 || kind != "issue" {
		t.Fatalf("issue %s #%d %s %v", repo, number, kind, err)
	}
	repo, number, kind, err = ParseLink("https://github.com/acme/app/pull/8/files")
	if err != nil || repo != "acme/app" || number != 8 || kind != "pull" {
		t.Fatalf("pull %s #%d %s %v", repo, number, kind, err)
	}
	if _, _, _, err := ParseLink("https://example.com/acme/app/issues/1"); err == nil {
		t.Fatal("non-github")
	}
}

func TestIssueJSONShape(t *testing.T) {
	raw, err := json.Marshal(Issue{Repo: "a/b", Number: 1, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("empty")
	}
}
