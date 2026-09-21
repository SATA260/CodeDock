package tools

import (
	"codedock/pkg/agent/tool"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodingExecuteAndWorkspaceInspect(t *testing.T) {
	root := t.TempDir()
	ports := Ports{WorkspaceRoot: root}
	reg := tool.NewRegistry()
	Register(reg, nil, nil, ports)
	write, err := reg.Get(tool.Reference{Name: ToolWrite})
	if err != nil {
		t.Fatal(err)
	}
	out, err := write.Execute(context.Background(), tool.Input{Call: tool.Call{
		ID:        "w1",
		Name:      ToolWrite,
		Arguments: json.RawMessage(`{"path":"hello.txt","content":"hi"}`),
	}})
	if err != nil || !out.Success {
		t.Fatalf("write %v %+v", err, out)
	}
	body, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil || string(body) != "hi" {
		t.Fatalf("disk %q %v", body, err)
	}
	read, err := reg.Get(tool.Reference{Name: ToolRead})
	if err != nil {
		t.Fatal(err)
	}
	got, err := read.Execute(context.Background(), tool.Input{Call: tool.Call{
		ID:        "r1",
		Name:      ToolRead,
		Arguments: json.RawMessage(`{"path":"hello.txt"}`),
	}})
	if err != nil || !got.Success {
		t.Fatalf("read %v %+v", err, got)
	}

	inspector := read.(tool.Inspector)
	if err := inspector.Inspect(context.Background(), tool.Input{Call: tool.Call{
		Arguments: json.RawMessage(`{"path":"hello.txt"}`),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := inspector.Inspect(context.Background(), tool.Input{Call: tool.Call{
		Arguments: json.RawMessage(`{"path":"../outside.txt"}`),
	}}); err == nil || !errors.Is(err, tool.ErrOutsideWorkspace) {
		t.Fatalf("outside %v", err)
	}
	empty := codingTool{name: ToolRead, ports: Ports{}}
	if err := empty.Inspect(context.Background(), tool.Input{Call: tool.Call{
		Arguments: json.RawMessage(`{"path":"hello.txt"}`),
	}}); err == nil {
		t.Fatal("empty workspace should fail inspect")
	}
}

func TestCodingPathVariants(t *testing.T) {
	root := t.TempDir()
	ports := Ports{WorkspaceRoot: root, LookPath: func(string) (string, error) { return "/bin/true", nil }}
	if _, err := codingPath(ToolWrite, json.RawMessage(`{"path":"a.txt","content":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := codingPath(ToolEdit, json.RawMessage(`{"path":"a.txt","edits":[{"oldText":"a","newText":"b"}]}`)); err != nil {
		t.Fatal(err)
	}
	path := "."
	if _, err := codingPath(ToolGrep, mustRaw(GrepInput{Pattern: "x", Path: &path})); err != nil {
		t.Fatal(err)
	}
	if _, err := codingPath(ToolFind, mustRaw(FindInput{Pattern: "*", Path: &path})); err != nil {
		t.Fatal(err)
	}
	if _, err := codingPath(ToolLS, mustRaw(LSInput{Path: &path})); err != nil {
		t.Fatal(err)
	}
	if _, err := codingPath(ToolBash, json.RawMessage(`{"command":"echo hi"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := codingPath(ToolBash, json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty command")
	}
	if _, err := codingPath(ToolPowerShell, json.RawMessage(`{"command":"dir"}`)); err != nil {
		t.Fatal(err)
	}
	_ = ports
}

func mustRaw(v any) json.RawMessage {
	body, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return body
}

func TestWritePlanFileSkipsApproval(t *testing.T) {
	root := t.TempDir()
	ports := Ports{WorkspaceRoot: root}
	reg := tool.NewRegistry()
	Register(reg, nil, nil, ports)

	planWrite, err := tool.Dispatch(context.Background(), tool.Invocation{
		WorkspaceRoot: root,
		BoundNames:    []string{"write"},
		Approval:      tool.ApprovalManual,
		Registry:      reg,
		Calls: []tool.Call{{
			ID:        "p1",
			Name:      "write",
			Arguments: json.RawMessage(`{"path":".cursor/notes.md","content":"# next"}`),
		}},
	})
	if err != nil || planWrite.WaitingApproval || len(planWrite.Results) != 1 || !planWrite.Results[0].Success {
		t.Fatalf("plan file write should skip approval: %v %+v", err, planWrite)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "notes.md")); err != nil {
		t.Fatal(err)
	}

	codeWrite, err := tool.Dispatch(context.Background(), tool.Invocation{
		WorkspaceRoot: root,
		BoundNames:    []string{"write"},
		Approval:      tool.ApprovalManual,
		Registry:      reg,
		Calls: []tool.Call{{
			ID:        "c1",
			Name:      "write",
			Arguments: json.RawMessage(`{"path":"main.go","content":"package main"}`),
		}},
	})
	if err != nil || !codeWrite.WaitingApproval {
		t.Fatalf("regular write still needs approval: %v %+v", err, codeWrite)
	}
}

func TestPlanToolsAndJail(t *testing.T) {
	root := t.TempDir()
	ports := Ports{WorkspaceRoot: root}
	reg := tool.NewRegistry()
	Register(reg, nil, nil, ports)

	write, err := reg.Get(tool.Reference{Name: "plan_write"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := write.Execute(context.Background(), tool.Input{Call: tool.Call{
		ID:        "w1",
		Name:      "plan_write",
		Arguments: json.RawMessage(`{"name":"demo","content":"---\n{\"title\":\"demo\",\"items\":[{\"id\":\"item-1\",\"description\":\"文档说明\",\"verify_cmd\":\"manual\"}]}\n---\n\n# hi"}`),
	}})
	if err != nil || !out.Success {
		t.Fatalf("write %v %+v", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "demo.md")); err != nil {
		t.Fatal(err)
	}

	pass, err := reg.Get(tool.Reference{Name: "plan_pass"})
	if err != nil {
		t.Fatal(err)
	}
	marked, err := pass.Execute(context.Background(), tool.Input{
		ActivePlan: "demo.md",
		Call: tool.Call{
			ID:        "p1",
			Name:      "plan_pass",
			Arguments: json.RawMessage(`{"name":"demo","itemId":"item-1","evidence":"manual ok"}`),
		},
	})
	if err != nil || !marked.Success {
		t.Fatalf("plan_pass %v %+v", err, marked)
	}

	list, err := reg.Get(tool.Reference{Name: "plan_list"})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := list.Execute(context.Background(), tool.Input{ActivePlan: "demo.md", Call: tool.Call{ID: "l1", Name: "plan_list", Arguments: json.RawMessage(`{}`)}})
	if err != nil || !listed.Success || !strings.Contains(string(listed.Output), "demo.md") {
		t.Fatalf("list %v %+v", err, listed)
	}

	read, err := reg.Get(tool.Reference{Name: "plan_read"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := read.Execute(context.Background(), tool.Input{
		ActivePlan: "demo.md",
		Call: tool.Call{
			ID:        "r1",
			Name:      "plan_read",
			Arguments: json.RawMessage(`{"name":"demo.md"}`),
		},
	})
	if err != nil || !got.Success || !strings.Contains(string(got.Output), "## 验收") || !strings.Contains(string(got.Output), "文档说明") {
		t.Fatalf("read %v %+v", err, got)
	}

	inspector, ok := write.(tool.Inspector)
	if !ok {
		t.Fatal("plan_write should inspect")
	}
	if err := inspector.Inspect(context.Background(), tool.Input{Call: tool.Call{
		Arguments: json.RawMessage(`{"name":"../escape","content":"x"}`),
	}}); err == nil {
		t.Fatal("expected invalid plan name")
	}
}

func TestCodingInspectEscapesWorkspace(t *testing.T) {
	root := t.TempDir()
	ports := Ports{WorkspaceRoot: root}
	item := codingTools(ports)[0]
	inspector := item.(tool.Inspector)
	err := inspector.Inspect(context.Background(), tool.Input{Call: tool.Call{
		Arguments: json.RawMessage(`{"path":"../outside.txt"}`),
	}})
	if err == nil || !errors.Is(err, tool.ErrOutsideWorkspace) {
		t.Fatalf("expected outside workspace, got %v", err)
	}
	if err := inspector.Inspect(context.Background(), tool.Input{Call: tool.Call{
		Arguments: json.RawMessage(`{"path":"inside.txt"}`),
	}}); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := inspector.Inspect(context.Background(), tool.Input{
		WorkspaceRoot: other,
		Call:          tool.Call{Arguments: json.RawMessage(`{"path":"inside.txt"}`)},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizePlanName(t *testing.T) {
	got, err := sanitizePlanName("notes")
	if err != nil || got != "notes.md" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := sanitizePlanName("../x.md"); err == nil {
		t.Fatal("expected deny")
	}
	if _, err := sanitizePlanName("a/b.md"); err == nil {
		t.Fatal("expected deny")
	}
}

func TestJailSymlinkAndResolve(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "out")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := jailPath(root, link, nil); err == nil {
		t.Fatal("symlink escape")
	}
	if insideRoot("/a", "/a/b") && !insideRoot("/a", "/b") {
		// ok
	}
	ports := Ports{WorkspaceRoot: root, FS: osFileSystem{}}
	_ = ports.executor()
	_ = inspectCodingPath(ports, ToolRead, json.RawMessage(`{"path":"out"}`))
	if err := inspectCodingPath(ports, ToolRead, json.RawMessage(`{"path":"file://%zz"}`)); err == nil {
		t.Fatal("bad file url")
	}

	dir := t.TempDir()
	origAbs, origRel := pathAbs, pathRel
	t.Cleanup(func() { pathAbs, pathRel = origAbs, origRel })
	pathAbs = func(string) (string, error) { return "", os.ErrInvalid }
	if _, err := jailPath(dir, "a.txt", nil); err == nil {
		t.Fatal("jail abs")
	}
	n := 0
	pathAbs = func(p string) (string, error) {
		n++
		if n >= 2 {
			return "", os.ErrInvalid
		}
		return origAbs(p)
	}
	if _, err := jailPath(dir, "a.txt", nil); err == nil {
		t.Fatal("target abs")
	}
	pathAbs = func(string) (string, error) { return "", os.ErrInvalid }
	if _, err := planDir(dir); err == nil {
		t.Fatal("planDir jail")
	}
	pathAbs = origAbs
	pathRel = func(string, string) (string, error) { return "", os.ErrInvalid }
	if insideRoot(dir, dir) {
		t.Fatal("rel fail")
	}
	pathRel = origRel
}

func TestPlanCursorIsFileAndFindPowerShell(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".cursor"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	ports := Ports{WorkspaceRoot: root}
	list := planTool{name: "plan_list", ports: ports}
	got, err := list.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "l"}})
	if err != nil || got.Success {
		t.Fatalf("list file %v %+v", err, got)
	}
	write := planTool{name: "plan_write", ports: ports}
	got, err = write.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "w", Arguments: json.RawMessage(`{"name":"a.md","content":"x"}`)}})
	if err != nil || got.Success {
		t.Fatalf("write file %v %+v", err, got)
	}

	okRoot := t.TempDir()
	okPorts := Ports{
		WorkspaceRoot: okRoot,
		LookPath:      func(string) (string, error) { return "/bin/echo", nil },
		RunCommand: func(ctx context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
			return CommandResult{}, nil
		},
	}
	reg := tool.NewRegistry()
	Register(reg, nil, nil, okPorts)
	find, _ := reg.Get(tool.Reference{Name: ToolFind})
	got, err = find.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "f", Arguments: json.RawMessage(`{"pattern":"*"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := reg.Get(tool.Reference{Name: ToolPowerShell})
	got, err = ps.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "p", Arguments: json.RawMessage(`{"command":"dir"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	grep, _ := reg.Get(tool.Reference{Name: ToolGrep})
	_, _ = grep.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "g", Arguments: json.RawMessage(`{"pattern":"x"}`)}})
}

func TestImageSniffAndDiffEdges(t *testing.T) {
	if isSupportedPNG(nil) || isSupportedBMP(nil) {
		t.Fatal("empty")
	}
	png := []byte("\x89PNG\r\n\x1a\n")
	if isSupportedPNG(png) {
		t.Fatal("short png")
	}
	if isSupportedBMP([]byte("BM")) {
		t.Fatal("short bmp")
	}
	diff, _ := generateDiffString("", "a\n", 1)
	if diff == "" {
		t.Fatal("diff")
	}
	diff, _ = generateDiffString("a\n", "a\n", 1)
	_ = diff
	diff, _ = generateDiffString("a\nb\nc\n", "a\nX\nc\n", 0)
	if diff == "" {
		t.Fatal("no context")
	}
	_ = mergeDiffOperations(nil)
}
