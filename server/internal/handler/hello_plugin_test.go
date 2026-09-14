package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"codedock/internal/handler"
	"codedock/internal/pluginhost"
	pkgagent "codedock/pkg/agent"
)

func TestLoopHelloPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("hello plugin")
	}
	dir := t.TempDir()
	buildHelloPlugin(t, filepath.Join(dir, "hello", "hello"))

	f := newFixture(t)
	host, err := pluginhost.Load(context.Background(), pluginhost.Options{
		Dir:      dir,
		Timeout:  3 * time.Second,
		Registry: f.runtime.Tools(),
		Model:    pkgagent.ModelConfig{Provider: "fake", Model: "fake"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	f.runtime.SetDispatcher(host)

	sessionID := f.createSession(t)
	rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{
		Content: "/skip this",
		Mode:    pkgagent.ModeAutoApprove,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("skip %d %s", rec.Code, rec.Body.String())
	}
	var skipped handler.StartRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &skipped); err != nil {
		t.Fatal(err)
	}
	if !skipped.Handled || skipped.RunID != "" {
		t.Fatalf("skip resp=%+v", skipped)
	}

	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "world",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "ok"}},
		}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	msgs := listMessages(t, f, sessionID, "")
	if len(msgs.Messages) == 0 || pkgagent.DecodeText(msgs.Messages[0].Content) != "[hello] world" {
		t.Fatalf("user message=%+v", msgs.Messages)
	}
	_, hist, err := f.runtime.LoadAgentState(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Hidden) != 1 || pkgagent.DecodeText(hist.Hidden[0].Content) == "" {
		t.Fatalf("overlay hidden=%+v", hist.Hidden)
	}
	found := false
	for _, def := range hist.Tools {
		if def.Name == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hello method not visible: %+v", hist.Tools)
	}
}

func buildHelloPlugin(t *testing.T, dest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := dir
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("go.mod not found")
		}
		root = parent
	}
	cmd := exec.Command("go", "build", "-o", dest, ".")
	cmd.Dir = filepath.Join(filepath.Dir(root), "example", "hello")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build hello: %v\n%s", err, out)
	}
}
