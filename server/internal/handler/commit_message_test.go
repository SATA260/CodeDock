package handler

import (
	"context"
	"strings"
	"testing"
	"time"

	"codedock/internal/config"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/git"
)

func TestClassifyPatchAndGeneratedPath(t *testing.T) {
	if got := classifyPatch(git.DiffFile{Path: "a.png", Binary: true, Kind: "added"}); got != "binary" {
		t.Fatalf("binary: %s", got)
	}
	if got := classifyPatch(git.DiffFile{Path: "pnpm-lock.yaml", Patch: "huge", Kind: "modified"}); got != "generated" {
		t.Fatalf("generated: %s", got)
	}
	if got := classifyPatch(git.DiffFile{Path: "a.go", Kind: "modified"}); got != "empty" {
		t.Fatalf("empty: %s", got)
	}
	if got := classifyPatch(git.DiffFile{Path: "a.go", Kind: "modified", Patch: "@@\n+x\n"}); got != "text" {
		t.Fatalf("text: %s", got)
	}
	if !isGeneratedPath("vendor/go.sum") || !isGeneratedPath("Cargo.lock") {
		t.Fatal("generated path")
	}
	if isGeneratedPath("server/pkg/git/repo.go") {
		t.Fatal("source is not generated")
	}
}

func TestFileInventoryKeepsEveryPath(t *testing.T) {
	inv := fileInventory([]git.DiffFile{
		{Path: "a.go", Kind: "modified", Patch: "@@\n+x\n"},
		{Path: "logo.png", Kind: "added", Binary: true},
		{Path: "go.sum", Kind: "modified", Patch: "+h1"},
	})
	if !strings.Contains(inv, "modified a.go") || !strings.Contains(inv, "binary added logo.png") || !strings.Contains(inv, "generated modified go.sum") {
		t.Fatalf("inventory: %s", inv)
	}
}

func TestClipPatchAndFillBudget(t *testing.T) {
	small := git.DiffFile{Path: "a.go", Kind: "modified", Patch: "@@ -1 +1 @@\n-old\n+new\n"}
	if got := fillPatchBudget([]git.DiffFile{small}, 8000); got != strings.TrimSpace(small.Patch) {
		t.Fatalf("small patch should fit: %q", got)
	}

	long := strings.Repeat("x", 40)
	clipped := clipPatch(long, 2)
	if !strings.Contains(clipped, "... truncated") {
		t.Fatalf("clip: %q", clipped)
	}
	if strings.Contains(clipPatch("abcd", 8), "truncated") {
		t.Fatal("short patch should not clip")
	}

	files := []git.DiffFile{
		{Path: "z.go", Kind: "modified", Patch: strings.Repeat("z", 20)},
		{Path: "a.go", Kind: "modified", Patch: strings.Repeat("a", 20)},
		{Path: "go.sum", Kind: "modified", Patch: strings.Repeat("s", 20)},
	}
	packed := fillPatchBudget(files, 5)
	if strings.Contains(packed, "s") {
		t.Fatalf("generated should not pack: %q", packed)
	}
	if !strings.Contains(packed, "a") {
		t.Fatalf("path order should prefer a.go: %q", packed)
	}
	if strings.Count(packed, "z") > 0 && strings.Count(packed, "a") == 0 {
		t.Fatalf("budget should fill a.go first: %q", packed)
	}
}

func TestParseDraftAndFallback(t *testing.T) {
	draft := parseDraft("feat(notes): add demo\n\n- Added the generate fixture.\n- Updated the large payload for budget tests.")
	if draft.Title != "feat(notes): add demo" || !strings.Contains(draft.Body, "- Added the generate fixture.") {
		t.Fatalf("draft: %+v", draft)
	}
	files := []git.DiffFile{{Path: "a.go", Kind: "modified"}, {Path: "b.go", Kind: "added"}}
	got := fallbackDraft(files)
	if !strings.HasPrefix(got, "chore: update staged files") || !strings.Contains(got, "- Added b.go") || !strings.Contains(got, "- Updated a.go") {
		t.Fatalf("fallback: %q", got)
	}
}

func TestGenerateDraftDeadlineFallsBackFast(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_REPO", dir)
	t.Setenv("LLM_PROVIDER", "fake")
	t.Setenv("LLM_MODEL", "fake")
	api := New(nil, nil, nil, nil, pkgagent.RunConfigSnapshot{}, config.Load(), nil)
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := []git.DiffFile{{Path: "a.txt", Kind: "added", Patch: "@@\n+hello\n"}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	start := time.Now()
	draft := api.generateDraft(ctx, repo, files)
	if time.Since(start) > 2*time.Second {
		t.Fatalf("deadline fallback took %s", time.Since(start))
	}
	if draft.Title == "" {
		t.Fatal("empty fallback title")
	}
}

func TestDraftBudgetStaysSmall(t *testing.T) {
	if draftStreamTimeout > 8*time.Second || draftRequestTimeout > 9*time.Second {
		t.Fatalf("timeouts %s %s", draftStreamTimeout, draftRequestTimeout)
	}
	if patchBudgetTokens > 1500 || perFilePatchTokens > 300 || draftMaxOutputTokens > 320 {
		t.Fatalf("budgets %d %d %d", patchBudgetTokens, perFilePatchTokens, draftMaxOutputTokens)
	}
}

func TestReadDraftTextKeepsDeltas(t *testing.T) {
	opts := `{"turns":[{"text":"feat(notes): add demo\n\nAdd the fixture.\nKeep tests green."}]}`
	stream, err := pkgagent.Stream(context.Background(), pkgagent.Chat{
		Model: pkgagent.ModelConfig{Provider: "fake", Options: []byte(opts)},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	got := readDraftText(stream)
	if !strings.HasPrefix(got, "feat(notes): add demo") || !strings.Contains(got, "Keep tests green.") {
		t.Fatalf("text: %q", got)
	}
}

func TestResolvePrompt(t *testing.T) {
	if resolvePrompt(promptStore{Selected: promptIDCustom, Custom: "写短标题"}) != "写短标题" {
		t.Fatal("custom")
	}
	if resolvePrompt(promptStore{Selected: promptIDCustom}) != conventionalCommitPrompt {
		t.Fatal("empty custom falls back")
	}
	got := resolvePrompt(promptStore{Selected: promptIDConventional})
	if !strings.Contains(got, "type(scope)") || !strings.Contains(got, `- "`) {
		t.Fatal("conventional")
	}
	if resolvePrompt(promptStore{Selected: "standard"}) != conventionalCommitPrompt {
		t.Fatal("old standard maps to conventional")
	}
}
