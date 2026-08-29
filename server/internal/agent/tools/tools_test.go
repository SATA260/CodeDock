package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"codedock/internal/agent/memory"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
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
