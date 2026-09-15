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

const redactSecret = "super-secret-value-not-a-shape"
const redactAWS = "AKIAIOSFODNN7EXAMPLE"

// TestLoopRedactPlugin 对照脱敏示例：read .env 和用户粘贴的 key 落库都没有原文。
func TestLoopRedactPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("redact plugin")
	}
	pluginDir := t.TempDir()
	buildRedactPlugin(t, filepath.Join(pluginDir, "redact", "redact"))

	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, ".env"), []byte("API_KEY="+redactSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newFixture(t)
	host, err := pluginhost.Load(context.Background(), pluginhost.Options{
		Dir:      pluginDir,
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

	rec := f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{
		UserID: "u1", TenantID: "t1", WorkspaceID: ws,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create session %d %s", rec.Code, rec.Body.String())
	}
	var created handler.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	sessionID := created.Session.ID

	readID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "read env",
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{
				{ToolCalls: []pkgagent.FakeToolCall{{
					Name:      "read",
					Arguments: json.RawMessage(`{"path":".env"}`),
				}}},
				{Text: "done"},
			},
		}),
	})
	f.waitRun(t, readID, pkgagent.RunCompleted)
	msgs := listMessages(t, f, sessionID, "")
	var toolBody string
	for _, msg := range msgs.Messages {
		if msg.Role == pkgagent.RoleTool {
			toolBody = string(msg.Content)
		}
	}
	if toolBody == "" {
		t.Fatalf("missing tool message: %+v", msgs.Messages)
	}
	if strings.Contains(toolBody, redactSecret) {
		t.Fatalf("secret leaked in tool message: %s", toolBody)
	}
	if !strings.Contains(toolBody, "API_KEY=") || !strings.Contains(toolBody, "«REDACTED:assignment:") {
		t.Fatalf("want keyed placeholder: %s", toolBody)
	}

	pasteID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "use " + redactAWS,
		Mode:    pkgagent.WorkAgent,
		Config: withFake(pkgagent.DefaultYoloConfig(pkgagent.ModelConfig{}), pkgagent.FakeOptions{
			Turns: []pkgagent.FakeTurn{{Text: "ok"}},
		}),
	})
	f.waitRun(t, pasteID, pkgagent.RunCompleted)
	pasted := listMessages(t, f, sessionID, "")
	var userText string
	for _, msg := range pasted.Messages {
		if msg.Role == pkgagent.RoleUser {
			text := pkgagent.DecodeText(msg.Content)
			if strings.Contains(text, "use ") || strings.Contains(text, "REDACTED") {
				userText = text
			}
		}
	}
	if userText == "" || strings.Contains(userText, redactAWS) || !strings.Contains(userText, "«REDACTED:aws_ak:") {
		t.Fatalf("user paste %q all=%+v", userText, pasted.Messages)
	}
}

func buildRedactPlugin(t *testing.T, dest string) {
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
	cmd.Dir = filepath.Join(filepath.Dir(root), "example", "redact")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build redact: %v\n%s", err, out)
	}
}
