package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveProbeAndTurn(t *testing.T) {
	unsetForTest(t, "CLAUDE_CONFIG_DIR")
	unsetForTest(t, "CLAUDE_BIN")
	bin, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not installed")
	}
	auth := exec.Command(bin, "auth", "status")
	if out, err := auth.CombinedOutput(); err != nil {
		t.Skipf("claude not authorized: %v %s", err, out)
	}
	resetRuntime()
	t.Cleanup(resetRuntime)
	t.Setenv("CLAUDE_BIN", bin)
	repo := t.TempDir()
	t.Setenv("GIT_REPO", repo)
	t.Cleanup(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		_ = os.RemoveAll(filepath.Join(home, ".claude", "projects", sanitizeProject(repo)))
	})
	status, err := Probe()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || !status.Authorized {
		t.Fatalf("%+v", status)
	}
	sess, err := Create("live")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(sess.ID, Settings{PermissionMode: "dontAsk"}); err != nil {
		t.Fatal(err)
	}
	turnID, err := Start(sess.ID, "Reply with the single word pong.", Input{Text: "Reply with the single word pong.", Mentions: emptyStrings(), Images: emptyStrings()}, InputModeStart)
	if err != nil {
		t.Fatal(err)
	}
	if turnID == "" {
		t.Fatal("turn id")
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		got, err := Get(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ActiveTurnID == "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	got, err := Get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClaudeSessionID == "" {
		t.Fatal("missing claude session")
	}
	items, err := Hydrate(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if items == nil {
		t.Fatal("items")
	}
}

func unsetForTest(t *testing.T, key string) {
	t.Helper()
	orig, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, orig)
			return
		}
		_ = os.Unsetenv(key)
	})
}
