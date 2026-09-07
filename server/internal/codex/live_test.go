package codex

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pkg "codedock/pkg/codex"
)

func TestLiveCodexAppServer(t *testing.T) {
	path, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("local Codex CLI is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rt := New(Options{Bin: path})
	defer rt.Close()

	status, err := rt.Probe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available {
		t.Fatalf("codex found at %s but Probe says unavailable: %+v", path, status)
	}
	if status.Version == "" {
		t.Fatal("empty version")
	}
	t.Logf("codex version=%s authorized=%v hint=%s", status.Version, status.Authorized, status.Hint)

	models, err := rt.ListModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("models=%d", len(models))

	modes, err := rt.ListModes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) == 0 {
		t.Fatal("expected plan modes")
	}
	t.Logf("modes=%d", len(modes))

	if !status.Authorized {
		t.Log("not authorized; skip thread lifecycle")
		return
	}

	dir := t.TempDir()
	session, err := rt.CreateSession(ctx, pkg.Settings{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if session.ID == "" {
		t.Fatal("empty thread id")
	}
	t.Logf("thread=%s", session.ID)
	if err := rt.Rename(ctx, session.ID, "codedock-live-"+filepath.Base(dir)); err != nil {
		t.Fatal(err)
	}
	got, _, err := rt.GetSession(ctx, session.ID)
	if err != nil || got.ID != session.ID {
		t.Fatal(got, err)
	}
	if err := rt.Archive(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ListSessions(ctx, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Effective(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
}
