package pluginhost

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
	sdk "codedock/pkg/plugin"
)

// jsonGet 读信封袋子里的一个键。
func jsonGet(raw json.RawMessage, key string) json.RawMessage {
	return sdk.DecodePluginContext(raw).Get(key)
}

// TestHostDispatchMethodsAndEmit 覆盖链式改写、换向、方法执行和 Emit 拒绝同名口。
func TestHostDispatchMethodsAndEmit(t *testing.T) {
	if testing.Short() {
		t.Skip("plugin host integration")
	}
	dir := t.TempDir()
	buildExample(t, "hello", filepath.Join(dir, "hello", "hello"))
	buildPlugin(t, "codedock/internal/pluginhost/testdata/probe", filepath.Join(dir, "probe", "probe"))

	reg := tool.NewRegistry()
	host, err := Load(context.Background(), Options{
		Dir:      dir,
		Timeout:  3 * time.Second,
		Registry: reg,
		Model:    pkgagent.ModelConfig{Provider: "fake", Model: "fake"},
		Log:      nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	ev, err := host.Dispatch(context.Background(), seam.Envelope{
		Type:    seam.TypeInput,
		Payload: pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "hello"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload pkgagent.InputPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if ev.Type != seam.TypeInput || payload.Content != "[hello] hello +probe" {
		t.Fatalf("ev=%+v payload=%+v", ev, payload)
	}
	if len(ev.Seen) != 2 || ev.Seen[0] != "hello" || ev.Seen[1] != "probe" {
		t.Fatalf("seen=%v", ev.Seen)
	}

	handled, err := host.Dispatch(context.Background(), seam.Envelope{
		Type:    seam.TypeInput,
		Payload: pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "/skip later"}),
	})
	if err != nil || handled.Type != seam.TypeInputHandled {
		t.Fatalf("handled=%+v err=%v", handled, err)
	}

	denied, err := host.Dispatch(context.Background(), seam.Envelope{
		Type: seam.TypePreExecute,
		Payload: pkgagent.MarshalPayload(tool.PreExecutePayload{
			Call: tool.Call{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{"x":"forbidden"}`)},
		}),
	})
	if err != nil || denied.Type != seam.TypeToolsDenied {
		t.Fatalf("denied=%+v err=%v", denied, err)
	}

	names := host.MethodNames()
	if len(names) != 1 || names[0] != "hello" {
		t.Fatalf("methods=%v", names)
	}
	item, err := reg.Get(tool.Reference{Name: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := item.Execute(context.Background(), tool.Input{
		Call: tool.Call{ID: "c1", Name: "hello", Arguments: json.RawMessage(`{"text":"hi"}`)},
	})
	if err != nil || !result.Success || string(result.Output) != `{"text":"hi"}` {
		t.Fatalf("hello result=%+v err=%v", result, err)
	}

	if err := host.Emit(context.Background(), seam.Envelope{Type: seam.TypeInput}); err == nil {
		t.Fatal("expected emit of seam type to fail")
	}
}

// TestHostProbeHandled 确认 probe 能把 input 换成 handled。
func TestHostProbeHandled(t *testing.T) {
	if testing.Short() {
		t.Skip("plugin host integration")
	}
	dir := t.TempDir()
	buildPlugin(t, "codedock/internal/pluginhost/testdata/probe", filepath.Join(dir, "probe", "probe"))
	host, err := Load(context.Background(), Options{
		Dir:     dir,
		Timeout: 3 * time.Second,
		Model:   pkgagent.ModelConfig{Provider: "fake", Model: "fake"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	handled, err := host.Dispatch(context.Background(), seam.Envelope{
		Type:    seam.TypeInput,
		Payload: pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "handle-me"}),
	})
	if err != nil || handled.Type != seam.TypeInputHandled {
		t.Fatalf("handled=%+v err=%v", handled, err)
	}
}

// TestHostTimeout 确认插件拖延超过 RPC 超时会失败。
func TestHostTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("plugin host integration")
	}
	dir := t.TempDir()
	buildPlugin(t, "codedock/internal/pluginhost/testdata/probe", filepath.Join(dir, "probe", "probe"))
	host, err := Load(context.Background(), Options{
		Dir:     dir,
		Timeout: 200 * time.Millisecond,
		Model:   pkgagent.ModelConfig{Provider: "fake", Model: "fake"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	_, err = host.Dispatch(context.Background(), seam.Envelope{
		Type:    seam.TypeInput,
		Payload: pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "stall:now"}),
	})
	if err == nil {
		t.Fatal("expected timeout")
	}
}

// TestHelloTemplate 跑一遍 hello 示例的改正文、跳过、隐藏提示、否决和方法。
func TestHelloTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("plugin host integration")
	}
	dir := t.TempDir()
	buildExample(t, "hello", filepath.Join(dir, "hello", "hello"))
	reg := tool.NewRegistry()
	host, err := Load(context.Background(), Options{
		Dir:      dir,
		Timeout:  3 * time.Second,
		Registry: reg,
		Model:    pkgagent.ModelConfig{Provider: "fake", Model: "fake"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	rewritten, err := host.Dispatch(context.Background(), seam.Envelope{
		Type:      seam.TypeInput,
		SessionID: "sess-hello",
		Payload:   pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "world"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var input pkgagent.InputPayload
	if err := json.Unmarshal(rewritten.Payload, &input); err != nil {
		t.Fatal(err)
	}
	if rewritten.Type != seam.TypeInput || input.Content != "[hello] world" || len(rewritten.Seen) != 1 || rewritten.Seen[0] != "hello" {
		t.Fatalf("rewrite=%+v payload=%+v", rewritten, input)
	}
	if string(jsonGet(rewritten.Context, "hello.marked")) != "true" {
		t.Fatalf("input context=%s", rewritten.Context)
	}

	skipped, err := host.Dispatch(context.Background(), seam.Envelope{
		Type:    seam.TypeInput,
		Payload: pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "/skip later"}),
	})
	if err != nil || skipped.Type != seam.TypeInputHandled {
		t.Fatalf("skip=%+v err=%v", skipped, err)
	}

	pre, err := host.Dispatch(context.Background(), seam.Envelope{
		Type:      seam.TypePreStep,
		SessionID: "sess-hello",
		RunID:     "run-hello",
		Payload:   pkgagent.MarshalPayload(pkgagent.PreStepPayload{SystemPrompt: "base"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var step pkgagent.PreStepPayload
	if err := json.Unmarshal(pre.Payload, &step); err != nil {
		t.Fatal(err)
	}
	if len(step.Hidden) != 1 || pkgagent.DecodeText(step.Hidden[0].Content) == "" {
		t.Fatalf("pre-step=%+v", step)
	}
	if string(jsonGet(pre.Context, "hello.marked")) != "true" || string(jsonGet(pre.Context, "hello.seen_at_pre_step")) != "true" {
		t.Fatalf("pre-step context=%s", pre.Context)
	}

	denied, err := host.Dispatch(context.Background(), seam.Envelope{
		Type: seam.TypePreExecute,
		Payload: pkgagent.MarshalPayload(tool.PreExecutePayload{
			Call: tool.Call{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{"x":"forbidden"}`)},
		}),
	})
	if err != nil || denied.Type != seam.TypeToolsDenied {
		t.Fatalf("denied=%+v err=%v", denied, err)
	}

	item, err := reg.Get(tool.Reference{Name: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := item.Execute(context.Background(), tool.Input{
		Call: tool.Call{ID: "c1", Name: "hello", Arguments: json.RawMessage(`{"text":"hi"}`)},
	})
	if err != nil || !result.Success || string(result.Output) != `{"text":"hi"}` {
		t.Fatalf("hello method=%+v err=%v", result, err)
	}
}

// TestLoadEmptyDir 确认未设插件目录时不拉进程。
func TestLoadEmptyDir(t *testing.T) {
	host, err := Load(context.Background(), Options{Dir: ""})
	if err != nil || host != nil {
		t.Fatalf("empty dir host=%v err=%v", host, err)
	}
}

// buildExample 编译仓根 example/<name> 到 dest。
func buildExample(t *testing.T, name, dest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", dest, ".")
	cmd.Dir = filepath.Join(repoRoot(t), "example", name)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build example/%s: %v\n%s", name, err, out)
	}
}

// buildPlugin 用 server 模块路径编译一个测试插件到 dest。
func buildPlugin(t *testing.T, pkg, dest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", dest, pkg)
	cmd.Dir = moduleRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
}

// repoRoot 返回仓根（server 的上一级）。
func repoRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(moduleRoot(t))
	if _, err := os.Stat(filepath.Join(root, "example")); err != nil {
		t.Fatalf("example/ not found at %s", root)
	}
	return root
}

// moduleRoot 从当前工作目录向上找到 server/go.mod。
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found")
	return ""
}
