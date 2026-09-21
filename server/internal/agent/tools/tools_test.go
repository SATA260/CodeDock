package tools

import (
	"codedock/internal/agent/memory"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testQueries(t *testing.T) (*sqlite.Queries, context.Context) {
	t.Helper()
	ctx := context.Background()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	client, err := db.Open(ctx, db.Config{Engine: db.EngineSQLite, DSN: fmt.Sprintf("file:%s?mode=memory&cache=shared", name)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := db.Migrate(ctx, client.DB()); err != nil {
		t.Fatal(err)
	}
	return db.SQLiteQueries(client), ctx
}

func TestPing(t *testing.T) {
	item := Ping()
	var schema map[string]any
	if err := json.Unmarshal(item.Definition().OutputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	props, _ := schema["properties"].(map[string]any)
	ok, _ := props["ok"].(map[string]any)
	if ok["type"] != "boolean" {
		t.Fatalf("output schema %+v", schema)
	}
	out, err := item.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "c1", Name: "ping"}})
	if err != nil || !out.Success || string(out.Output) != `{"ok":true}` {
		t.Fatalf("ping %v %+v", err, out)
	}
}

func TestMemoryReadSchema(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(ReadTool(nil).Definition().ParametersSchema, &schema); err != nil {
		t.Fatal(err)
	}
	required, _ := schema["required"].([]any)
	got := map[string]bool{}
	for _, item := range required {
		name, _ := item.(string)
		got[name] = true
	}
	if !got["scope"] || !got["name"] {
		t.Fatalf("required %v", required)
	}
}

func TestRegisterWithoutQueries(t *testing.T) {
	reg := tool.NewRegistry()
	Register(reg, nil, nil, Ports{})
	if _, err := reg.Get(tool.Reference{Name: "ping"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get(tool.Reference{Name: "memory_read"}); err == nil {
		t.Fatal("memory tools should stay unregistered when queries are nil")
	}
}

func TestMemoryTools(t *testing.T) {
	q, ctx := testQueries(t)
	if _, err := q.InsertSession(ctx, sqlite.InsertSessionParams{
		ID:          "sess1",
		TenantID:    "t1",
		UserID:      "u1",
		AgentID:     "default",
		WorkspaceID: "ws1",
		Status:      "active",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	var queued memory.TextMemoryKey
	write := WriteTool(q, func(key memory.TextMemoryKey) { queued = key })
	read := ReadTool(q)
	search := SearchTool(q)
	out, err := write.Execute(ctx, tool.Input{SessionID: "sess1", Call: tool.Call{
		ID:        "c1",
		Name:      "memory_write",
		Arguments: []byte(`{"scope":"user","name":"index","content":"prefer gofmt"}`),
	}})
	if err != nil || !out.Success {
		t.Fatalf("write %v %+v", err, out)
	}
	out, err = read.Execute(ctx, tool.Input{SessionID: "sess1", Call: tool.Call{
		ID:        "c2",
		Name:      "memory_read",
		Arguments: []byte(`{"scope":"user","name":"index"}`),
	}})
	if err != nil || !out.Success || !strings.Contains(string(out.Output), "gofmt") {
		t.Fatalf("read %v %s", err, out.Output)
	}
	if err := memory.IndexMessage(ctx, q, memory.ContextMessage{ID: "m1", WorkspaceID: "ws1", SessionID: "sess1", Role: "user", Content: "gofmt on save"}); err != nil {
		t.Fatal(err)
	}
	out, err = search.Execute(ctx, tool.Input{SessionID: "sess1", Call: tool.Call{
		ID:        "c3",
		Name:      "memory_search",
		Arguments: []byte(`{"query":"gofmt"}`),
	}})
	if err != nil || !out.Success {
		t.Fatalf("search %v %+v", err, out)
	}
	over, err := write.Execute(ctx, tool.Input{SessionID: "sess1", Call: tool.Call{
		ID:        "c4",
		Name:      "memory_write",
		Arguments: []byte(fmt.Sprintf(`{"scope":"workspace","name":"index","content":%q}`, strings.Repeat("line\n", memory.IndexMaxLines)+"overflow")),
	}})
	if err != nil || !over.Success {
		t.Fatalf("over write %v %+v", err, over)
	}
	if queued.Scope != memory.ScopeWorkspace || queued.ScopeID != "ws1" {
		t.Fatalf("queued %+v", queued)
	}
}

func TestMemoryReadMissingIsResult(t *testing.T) {
	q, ctx := testQueries(t)
	if _, err := q.InsertSession(ctx, sqlite.InsertSessionParams{
		ID:          "sess-miss",
		TenantID:    "t1",
		UserID:      "u1",
		AgentID:     "default",
		WorkspaceID: "ws1",
		Status:      "active",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	out, err := ReadTool(q).Execute(ctx, tool.Input{SessionID: "sess-miss", Call: tool.Call{
		ID:        "c1",
		Name:      "memory_read",
		Arguments: []byte(`{"scope":"user","name":"nope"}`),
	}})
	if err != nil {
		t.Fatalf("missing memory must be a result, not error: %v", err)
	}
	if out.Success || out.Error == "" {
		t.Fatalf("out=%+v", out)
	}
}

func TestToolNames(t *testing.T) {
	t.Parallel()
	want := []string{"read", "bash", "powershell", "edit", "write", "grep", "find", "ls"}
	if strings.Join(ToolNames, ",") != strings.Join(want, ",") {
		t.Fatalf("ToolNames = %v, want %v", ToolNames, want)
	}
}

func TestToolResultJSONEnvelope(t *testing.T) {
	t.Parallel()
	result := textResult("ok", nil)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `{"content":[{"type":"text","text":"ok"}]}`; got != want {
		t.Fatalf("encoded result = %s, want %s", got, want)
	}
	emptyEncoded, err := json.Marshal(textResult("", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(emptyEncoded), `{"content":[{"type":"text","text":""}]}`; got != want {
		t.Fatalf("encoded empty result = %s, want %s", got, want)
	}

	by := "bytes"
	result.Details = &ResultDetails{
		Truncation: &TruncationResult{
			Content:     "part",
			Truncated:   true,
			TruncatedBy: &by,
			TotalLines:  2,
			TotalBytes:  10,
			OutputLines: 1,
			OutputBytes: 4,
			MaxLines:    DefaultMaxLines,
			MaxBytes:    DefaultMaxBytes,
		},
	}
	encoded, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"details"`,
		`"truncation"`,
		`"truncatedBy":"bytes"`,
		`"lastLinePartial":false`,
		`"firstLineExceedsLimit":false`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("encoded result %s does not contain %s", encoded, field)
		}
	}
}

func TestExecuteRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()
	_, err := NewExecutor().Execute(context.Background(), ToolRead, t.TempDir(), json.RawMessage(`{}`))
	if err == nil || err.Error() != "missing required property: path" {
		t.Fatalf("error = %v", err)
	}

	_, err = NewExecutor().Execute(context.Background(), "unknown", t.TempDir(), json.RawMessage(`{}`))
	if err == nil || err.Error() != "unknown tool name: unknown" {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteRejectsRequiredNullFields(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	cwd := t.TempDir()
	for _, testCase := range []struct {
		name string
		tool string
		raw  string
	}{
		{name: "required string null", tool: ToolWrite, raw: `{"path":"a","content":null}`},
		{
			name: "edit replacement null",
			tool: ToolEdit,
			raw:  `{"path":"a","edits":[{"oldText":"x","newText":null}]}`,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := executor.Execute(
				context.Background(),
				testCase.tool,
				cwd,
				json.RawMessage(testCase.raw),
			); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDecodeAllowsOptionalNullAndAdditionalPropertiesLikeTypeBox(t *testing.T) {
	t.Parallel()
	var input ReadInput
	if err := decodeToolInput(
		json.RawMessage(`{"path":"a","offset":null,"extra":true}`),
		&input,
		"path",
	); err != nil {
		t.Fatal(err)
	}
	if input.Offset != nil {
		t.Fatalf("offset = %v, want nil", input.Offset)
	}
}

func TestDecodeEditCompatibilityInputs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{
			name: "array",
			raw:  `{"path":"a","edits":[{"oldText":"x","newText":"y"}]}`,
			want: 1,
		},
		{
			name: "single object",
			raw:  `{"path":"a","edits":{"oldText":"x","newText":"y"}}`,
			want: 1,
		},
		{
			name: "stringified array",
			raw:  `{"path":"a","edits":"[{\"oldText\":\"x\",\"newText\":\"y\"}]"}`,
			want: 1,
		},
		{
			name: "legacy top level",
			raw:  `{"path":"a","oldText":"x","newText":"y"}`,
			want: 1,
		},
		{
			name: "legacy appended",
			raw:  `{"path":"a","edits":[{"oldText":"x","newText":"y"}],"oldText":"z","newText":"w"}`,
			want: 2,
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			input, err := decodeEditInput(json.RawMessage(testCase.raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(input.Edits) != testCase.want {
				t.Fatalf("len(edits) = %d, want %d", len(input.Edits), testCase.want)
			}
		})
	}
}

func TestRegisterNilAndWithQueries(t *testing.T) {
	Register(nil, nil, nil, Ports{})
	q, _ := testQueries(t)
	reg := tool.NewRegistry()
	Register(reg, q, nil, Ports{WorkspaceRoot: t.TempDir()})
	if _, err := reg.Get(tool.Reference{Name: "memory_read"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get(tool.Reference{Name: "read"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get(tool.Reference{Name: "plan_list"}); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterOverBudgetType(t *testing.T) {
	q, _ := testQueries(t)
	reg := tool.NewRegistry()
	Register(reg, q, nil, Ports{})
	if _, err := reg.Get(tool.Reference{Name: "memory_write"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get(tool.Reference{Name: "read"}); err != nil {
		t.Fatal(err)
	}
	item, err := reg.Get(tool.Reference{Name: "read"})
	if err != nil {
		t.Fatal(err)
	}
	if err := item.(tool.Inspector).Inspect(context.Background(), tool.Input{Call: tool.Call{
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
	}}); err == nil {
		t.Fatal("inspect without workspace should fail")
	}
}

func TestCodingExecuteAllAndErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	ports := Ports{
		WorkspaceRoot: root,
		LookPath:      func(file string) (string, error) { return "/bin/echo", nil },
		RunCommand: func(ctx context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
			if onStdout != nil {
				onStdout([]byte("ok"))
			}
			return CommandResult{ExitCode: 0}, nil
		},
	}
	reg := tool.NewRegistry()
	Register(reg, nil, nil, ports)

	read, _ := reg.Get(tool.Reference{Name: ToolRead})
	got, err := read.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "r", Name: ToolRead, Arguments: json.RawMessage(`{"path":"hello.txt"}`)}})
	if err != nil || !got.Success {
		t.Fatalf("read %v %+v", err, got)
	}
	ls, _ := reg.Get(tool.Reference{Name: ToolLS})
	got, err = ls.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "l", Name: ToolLS, Arguments: json.RawMessage(`{}`)}})
	if err != nil || !got.Success {
		t.Fatalf("ls %v %+v", err, got)
	}
	write, _ := reg.Get(tool.Reference{Name: ToolWrite})
	got, err = write.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "w", Name: ToolWrite, Arguments: json.RawMessage(`{"path":"n.txt","content":"x"}`)}})
	if err != nil || !got.Success {
		t.Fatalf("write %v %+v", err, got)
	}
	edit, _ := reg.Get(tool.Reference{Name: ToolEdit})
	got, err = edit.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "e", Name: ToolEdit, Arguments: json.RawMessage(`{"path":"n.txt","edits":[{"oldText":"x","newText":"y"}]}`)}})
	if err != nil || !got.Success {
		t.Fatalf("edit %v %+v", err, got)
	}
	bash, _ := reg.Get(tool.Reference{Name: ToolBash})
	got, err = bash.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "b", Name: ToolBash, Arguments: json.RawMessage(`{"command":"echo hi"}`)}})
	if err != nil || !got.Success {
		t.Fatalf("bash %v %+v", err, got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = write.Execute(ctx, tool.Input{Call: tool.Call{ID: "c", Name: ToolWrite, Arguments: json.RawMessage(`{"path":"z.txt","content":"1"}`)}})
	if err == nil || got.Success {
		t.Fatalf("cancel %v %+v", err, got)
	}

	failPorts := Ports{
		WorkspaceRoot: root,
		LookPath:      func(string) (string, error) { return "", os.ErrNotExist },
	}
	item := codingTool{name: ToolGrep, effect: tool.EffectAllow, schema: schemaOf[GrepInput](), ports: failPorts}
	got, err = item.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "g", Name: ToolGrep, Arguments: json.RawMessage(`{"pattern":"x"}`)}})
	if err != nil || got.Success {
		t.Fatalf("grep missing %v %+v", err, got)
	}

	if _, err := codingPath(ToolRead, json.RawMessage(`{`)); err == nil {
		t.Fatal("bad json")
	}
	if _, err := codingPath(ToolWrite, json.RawMessage(`{`)); err == nil {
		t.Fatal("bad write json")
	}
	if _, err := codingPath(ToolEdit, json.RawMessage(`{`)); err == nil {
		t.Fatal("bad edit json")
	}
	if _, err := codingPath(ToolGrep, json.RawMessage(`{`)); err == nil {
		t.Fatal("bad grep")
	}
	if _, err := codingPath(ToolFind, json.RawMessage(`{`)); err == nil {
		t.Fatal("bad find")
	}
	if _, err := codingPath(ToolLS, json.RawMessage(`{`)); err == nil {
		t.Fatal("bad ls")
	}
	if _, err := codingPath(ToolBash, json.RawMessage(`{`)); err == nil {
		t.Fatal("bad bash")
	}
	if path, err := codingPath("unknown", nil); err != nil || path != "" {
		t.Fatal(path, err)
	}
	if path, err := codingPath(ToolGrep, json.RawMessage(`{"pattern":"x"}`)); err != nil || path != "." {
		t.Fatal(path, err)
	}
	if path, err := codingPath(ToolFind, json.RawMessage(`{"pattern":"*"}`)); err != nil || path != "." {
		t.Fatal(path, err)
	}
	if path, err := codingPath(ToolLS, json.RawMessage(`{}`)); err != nil || path != "." {
		t.Fatal(path, err)
	}
	cancelPorts := Ports{
		WorkspaceRoot: root,
		LookPath:      func(string) (string, error) { return "/bin/echo", nil },
		RunCommand: func(ctx context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
			return CommandResult{}, context.Canceled
		},
	}
	bashTool := codingTool{name: ToolBash, effect: tool.EffectAsk, schema: schemaOf[ShellInput](), ports: cancelPorts}
	got, err = bashTool.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "d", Name: ToolBash, Arguments: json.RawMessage(`{"command":"echo"}`)}})
	if err == nil || got.Success {
		t.Fatalf("canceled exec %v %+v", err, got)
	}
	if len(nonzeroJSON(nil)) == 0 {
		t.Fatal("nonzero")
	}
	if err := inspectCodingPath(ports, ToolBash, json.RawMessage(`{"command":"echo"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestJailAndPlanErrors(t *testing.T) {
	if _, err := jailPath("", "x", nil); err == nil {
		t.Fatal("empty root")
	}
	if !insideRoot("/a", "/a") {
		t.Fatal("same")
	}

	root := t.TempDir()
	if _, err := jailPath(root, filepath.Join(root, "..", "nope"), osFileSystem{}); err == nil || !errors.Is(err, tool.ErrOutsideWorkspace) {
		t.Fatalf("outside %v", err)
	}
	if _, err := jailPath(root, filepath.Join(root, "inside.txt"), osFileSystem{}); err != nil {
		t.Fatal(err)
	}

	ports := Ports{WorkspaceRoot: root}
	unknown := planTool{name: "plan_other", ports: ports}
	got, err := unknown.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "u", Name: "plan_other"}})
	if err != nil || got.Success {
		t.Fatalf("unknown %v %+v", err, got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = planTool{name: "plan_list", ports: ports}.Execute(ctx, tool.Input{Call: tool.Call{ID: "c"}})
	if err == nil || got.Success {
		t.Fatalf("cancel %v %+v", err, got)
	}
	empty := planTool{name: "plan_list", ports: Ports{}}
	got, err = empty.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "e"}})
	if err != nil || got.Success {
		t.Fatalf("empty root %v %+v", err, got)
	}
	if err := (planTool{name: "plan_list"}).Inspect(context.Background(), tool.Input{}); err != nil {
		t.Fatal(err)
	}
	if _, err := planNameFromArgs("plan_read", json.RawMessage(`{`)); err == nil {
		t.Fatal("bad name json")
	}
	if _, err := planNameFromArgs("plan_write", json.RawMessage(`{`)); err == nil {
		t.Fatal("bad write json")
	}
	if _, err := sanitizePlanName(""); err == nil {
		t.Fatal("empty name")
	}
	if _, err := sanitizePlanName(`a\b.md`); err == nil {
		t.Fatal("backslash")
	}
	read := planTool{name: "plan_read", ports: ports}
	got, err = read.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "r", Arguments: json.RawMessage(`{"name":"missing.md"}`)}})
	if err != nil || got.Success {
		t.Fatalf("missing %v %+v", err, got)
	}
	write := planTool{name: "plan_write", ports: ports}
	got, err = write.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "w", Arguments: json.RawMessage(`{`)}})
	if err != nil || got.Success {
		t.Fatalf("bad write exec %v %+v", err, got)
	}
	got, err = write.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "w2", Arguments: json.RawMessage(`{"name":"../x","content":"a"}`)}})
	if err != nil || got.Success {
		t.Fatalf("bad name exec %v %+v", err, got)
	}
	dir, err := os.MkdirTemp(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cursor", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "keep.md"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := planTool{name: "plan_list", ports: Ports{WorkspaceRoot: dir}}
	got, err = list.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "l"}})
	if err != nil || !got.Success {
		t.Fatalf("list %v %+v", err, got)
	}
	if strings.Contains(string(got.Output), "keep.md") {
		t.Fatalf("unbound list leaked sibling: %s", got.Output)
	}
	if _, err := planDir(""); err == nil {
		t.Fatal("planDir empty")
	}
	if _, err := okToolResult("id", "n", make(chan int)); err != nil {
		t.Fatal(err)
	}
	if _, err := planNameFromArgs("plan_write", json.RawMessage(`{"name":"a.md","content":1}`)); err == nil {
		t.Fatal("write type")
	}
	badRead := planTool{name: "plan_read", ports: ports}
	got, err = badRead.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "br", Arguments: json.RawMessage(`{"name":"../x"}`)}})
	if err != nil || got.Success {
		t.Fatalf("bad read name %v %+v", err, got)
	}
}

func TestPlanSessionIsolation(t *testing.T) {
	root := t.TempDir()
	ports := Ports{WorkspaceRoot: root}
	other := "---\n{\"title\":\"other\",\"items\":[{\"id\":\"x\",\"description\":\"别的任务\",\"verify_cmd\":\"manual\"}]}\n---\n# other\n"
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "other.md"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	list := planTool{name: "plan_list", ports: ports}
	got, err := list.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "l"}})
	if err != nil || !got.Success || strings.Contains(string(got.Output), "other.md") {
		t.Fatalf("unbound list %v %+v", err, got)
	}
	listed, err := list.Execute(context.Background(), tool.Input{AllowListAll: true, Call: tool.Call{ID: "all"}})
	if err != nil || !listed.Success || !strings.Contains(string(listed.Output), "other.md") {
		t.Fatalf("catalog list %v %+v", err, listed)
	}
	read := planTool{name: "plan_read", ports: ports}
	got, err = read.Execute(context.Background(), tool.Input{Call: tool.Call{ID: "r", Arguments: json.RawMessage(`{"name":"other.md"}`)}})
	if err != nil || got.Success {
		t.Fatalf("foreign read should fail %v %+v", err, got)
	}
	got, err = read.Execute(context.Background(), tool.Input{MentionedPlans: []string{"other.md"}, Call: tool.Call{ID: "r2", Arguments: json.RawMessage(`{"name":"other.md"}`)}})
	if err != nil || !got.Success {
		t.Fatalf("mentioned read %v %+v", err, got)
	}
	write := planTool{name: "plan_write", ports: ports}
	got, err = write.Execute(context.Background(), tool.Input{Call: tool.Call{
		ID:        "w",
		Arguments: json.RawMessage(`{"name":"other.md","content":"x"}`),
	}})
	if err != nil || got.Success {
		t.Fatalf("overwrite sibling %v %+v", err, got)
	}
	fresh, err := write.Execute(context.Background(), tool.Input{Call: tool.Call{
		ID:        "w2",
		Arguments: json.RawMessage(`{"name":"mine.md","content":"---\n{\"title\":\"mine\",\"items\":[{\"id\":\"a\",\"description\":\"文档说明\",\"verify_cmd\":\"manual\"}]}\n---\n# mine"}`),
	}})
	if err != nil || !fresh.Success {
		t.Fatalf("new plan %v %+v", err, fresh)
	}
	plain, err := write.Execute(context.Background(), tool.Input{Call: tool.Call{
		ID:        "w3",
		Arguments: json.RawMessage(`{"name":"plain.md","title":"plain","content":"# 正文","items":[{"id":"V1","description":"文档说明","verify_cmd":"manual"}]}`),
	}})
	if err != nil || !plain.Success || !strings.Contains(string(plain.Output), "V1") || !strings.Contains(string(plain.Output), "文档说明") {
		t.Fatalf("items param %v %+v", err, plain)
	}
	reader := codingTool{name: ToolRead, ports: ports}
	got, err = reader.Execute(context.Background(), tool.Input{Call: tool.Call{
		ID:        "cr",
		Arguments: json.RawMessage(`{"path":".cursor/other.md"}`),
	}})
	if err != nil || got.Success {
		t.Fatalf("coding read sibling %v %+v", err, got)
	}
}

func TestMemoryAndPingErrors(t *testing.T) {
	q, ctx := testQueries(t)
	if _, err := q.InsertSession(ctx, sqlite.InsertSessionParams{
		ID: "sess-err", TenantID: "t", UserID: "u", AgentID: "default", WorkspaceID: "", Status: "active",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	got, err := Ping().Execute(canceled, tool.Input{Call: tool.Call{ID: "p"}})
	if err == nil || got.Success {
		t.Fatalf("ping cancel %v %+v", err, got)
	}
	got, err = Ping().Execute(ctx, tool.Input{Call: tool.Call{ID: "p2", Arguments: json.RawMessage(`{`)}})
	if err != nil || got.Success {
		t.Fatalf("ping json %v %+v", err, got)
	}

	read := ReadTool(q)
	got, err = read.Execute(canceled, tool.Input{Call: tool.Call{ID: "r"}})
	if err == nil || got.Success {
		t.Fatalf("read cancel %v %+v", err, got)
	}
	got, err = read.Execute(ctx, tool.Input{Call: tool.Call{ID: "r2"}})
	if err != nil || got.Success {
		t.Fatalf("read args %v %+v", err, got)
	}
	got, err = read.Execute(ctx, tool.Input{Call: tool.Call{ID: "r3", Arguments: json.RawMessage(`{"scope":"nope","name":"index"}`)}, SessionID: "sess-err"})
	if err != nil || got.Success {
		t.Fatalf("bad scope %v %+v", err, got)
	}
	write := WriteTool(q, nil)
	got, err = write.Execute(canceled, tool.Input{Call: tool.Call{ID: "w"}})
	if err == nil || got.Success {
		t.Fatalf("write cancel %v %+v", err, got)
	}
	got, err = write.Execute(ctx, tool.Input{Call: tool.Call{ID: "w2", Arguments: json.RawMessage(`{`)}})
	if err != nil || got.Success {
		t.Fatalf("write json %v %+v", err, got)
	}
	search := SearchTool(q)
	got, err = search.Execute(canceled, tool.Input{Call: tool.Call{ID: "s"}})
	if err == nil || got.Success {
		t.Fatalf("search cancel %v %+v", err, got)
	}
	got, err = search.Execute(ctx, tool.Input{Call: tool.Call{ID: "s2"}})
	if err != nil || got.Success {
		t.Fatalf("search args %v %+v", err, got)
	}
	got, err = search.Execute(ctx, tool.Input{SessionID: "missing", Call: tool.Call{ID: "s3", Arguments: json.RawMessage(`{"query":"x"}`)}})
	if err != nil || got.Success {
		t.Fatalf("search session %v %+v", err, got)
	}
	got, err = search.Execute(ctx, tool.Input{SessionID: "sess-err", Call: tool.Call{ID: "s4", Arguments: json.RawMessage(`{"query":"x"}`)}})
	if err != nil || !got.Success {
		t.Fatalf("search empty ws %v %+v", err, got)
	}
	if _, err := scopeIDFromSession(ctx, nil, "s", "user"); err == nil {
		t.Fatal("nil q")
	}
	if _, err := scopeIDFromSession(ctx, q, "", "user"); err == nil {
		t.Fatal("empty session")
	}
	if wrapSessionErr(nil) != nil {
		t.Fatal("nil wrap")
	}
	got, err = write.Execute(ctx, tool.Input{SessionID: "sess-err", Call: tool.Call{ID: "w3", Arguments: json.RawMessage(`{"scope":"workspace","name":"index","content":"x"}`)}})
	if err != nil || !got.Success {
		t.Fatalf("write empty workspace %v %+v", err, got)
	}
	got, err = write.Execute(ctx, tool.Input{SessionID: "missing", Call: tool.Call{ID: "w4", Arguments: json.RawMessage(`{"scope":"user","name":"index","content":"x"}`)}})
	if err != nil || got.Success {
		t.Fatalf("write missing session %v %+v", err, got)
	}
	got, err = write.Execute(ctx, tool.Input{SessionID: "sess-err", Call: tool.Call{ID: "w5", Arguments: json.RawMessage(`{"scope":"nope","name":"index","content":"x"}`)}})
	if err != nil || got.Success {
		t.Fatalf("write bad scope %v %+v", err, got)
	}
	if _, err := okResult(tool.Input{Call: tool.Call{ID: "x"}}, "n", make(chan int)); err != nil {
		t.Fatal(err)
	}
}
