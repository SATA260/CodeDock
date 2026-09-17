package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codedock/internal/handler"
	"codedock/internal/pluginhost"
	pkgagent "codedock/pkg/agent"
)

// TestLoopHelloPlugin 对照示例：/skip 不建 Run，world 改正文并注入 Hidden，hello 方法可执行。
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
		Queries:  f.queries,
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
		Mode:    pkgagent.WorkAgent,
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
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "ok"}},
		}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	msgs := listMessages(t, f, sessionID, "")
	var userText string
	for _, msg := range msgs.Messages {
		if msg.Role == pkgagent.RoleUser {
			userText = pkgagent.DecodeText(msg.Content)
			break
		}
	}
	if userText != "[hello] world" {
		t.Fatalf("user message=%q all=%+v", userText, msgs.Messages)
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
	bound := false
	for _, name := range hist.Run.Config.Profile.Tools.Names {
		if name == "hello" {
			bound = true
		}
	}
	if !bound {
		t.Fatalf("hello should be executable: names=%v", hist.Run.Config.Profile.Tools.Names)
	}
}

// TestLoopHelloPluginMethodsAndDeny 确认 hello 能被模型调用，forbidden 参数会被否决。
func TestLoopHelloPluginMethodsAndDeny(t *testing.T) {
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
		Queries:  f.queries,
		Model:    pkgagent.ModelConfig{Provider: "fake", Model: "fake"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	f.runtime.SetDispatcher(host)

	sessionID := f.createSession(t)
	skip := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{
		Content: "/skip notice",
		Mode:    pkgagent.WorkAgent,
	})
	if skip.Code != http.StatusOK {
		t.Fatalf("skip %d %s", skip.Code, skip.Body.String())
	}
	notices := listMessages(t, f, sessionID, "")
	sawNotice := false
	for _, msg := range notices.Messages {
		if msg.Role == pkgagent.RoleSystem && pkgagent.DecodeText(msg.Content) == "hello 已跳过本次对话。" {
			sawNotice = true
		}
	}
	if !sawNotice {
		t.Fatalf("missing skip notice: %+v", notices.Messages)
	}

	callID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "call hello",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{
				{ToolCalls: []pkgagent.FakeToolCall{{
					Name:      "hello",
					Arguments: json.RawMessage(`{"text":"hi"}`),
				}}},
				{Text: "hello-done"},
			},
		}),
	})
	f.waitRun(t, callID, pkgagent.RunCompleted)
	called := listMessages(t, f, sessionID, "")
	var sawHello bool
	for _, msg := range called.Messages {
		if msg.Role == pkgagent.RoleTool && strings.Contains(string(msg.Content), `"text":"hi"`) {
			sawHello = true
		}
	}
	if !sawHello {
		t.Fatalf("hello method should execute: %+v", called.Messages)
	}

	denyID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "deny ping",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{
				{ToolCalls: []pkgagent.FakeToolCall{{
					Name:      "ping",
					Arguments: json.RawMessage(`{"x":"forbidden"}`),
				}}},
				{Text: "after-deny"},
			},
		}),
	})
	f.waitRun(t, denyID, pkgagent.RunCompleted)
	denied := listMessages(t, f, sessionID, "")
	var sawDenied bool
	for _, msg := range denied.Messages {
		if msg.Role == pkgagent.RoleTool && (strings.Contains(pkgagent.DecodeText(msg.Content), "denied by plugin") || strings.Contains(string(msg.Content), "denied by plugin")) {
			sawDenied = true
		}
	}
	if !sawDenied {
		t.Fatalf("forbidden ping should be denied: %+v", denied.Messages)
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
	cmd.Dir = filepath.Join(root, "internal", "pluginhost", "testdata", "hello")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build hello: %v\n%s", err, out)
	}
}
