package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"codedock/internal/errors"
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

func overBudgetIndex() string {
	return strings.Repeat("line\n", IndexMaxLines) + "overflow"
}

func TestByteLenAndClipIndex(t *testing.T) {
	if ByteLen("你好") != len("你好") {
		t.Fatalf("ByteLen=%d", ByteLen("你好"))
	}
	if IndexOverBudget("short") {
		t.Fatal("short should not be over budget")
	}
	long := overBudgetIndex()
	if !IndexOverBudget(long) {
		t.Fatal("expected over budget")
	}
	clipped := ClipIndex(long)
	if IndexOverBudget(clipped) {
		t.Fatal("clip should be within budget")
	}
	if indexLineCount(clipped) > IndexMaxLines {
		t.Fatalf("clipped lines=%d", indexLineCount(clipped))
	}
}

func TestTextMemoryCRUD(t *testing.T) {
	q, ctx := testQueries(t)
	item, err := Upsert(ctx, q, TextMemory{Scope: ScopeUser, ScopeID: "u1", Name: NameIndex, Content: "# prefs\n"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != KindIndex || item.ByteLen == 0 || item.OverBudget {
		t.Fatalf("upsert index: %+v", item)
	}
	got, err := Get(ctx, q, TextMemoryKey{Scope: ScopeUser, ScopeID: "u1", Name: NameIndex})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "# prefs\n" {
		t.Fatalf("get content %q", got.Content)
	}
	topic, err := Upsert(ctx, q, TextMemory{Scope: ScopeUser, ScopeID: "u1", Name: "debugging", Content: "details"})
	if err != nil {
		t.Fatal(err)
	}
	if topic.Kind != KindTopic {
		t.Fatalf("kind %s", topic.Kind)
	}
	listed, err := List(ctx, q, ScopeUser, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("list %d", len(listed))
	}
	if err := Delete(ctx, q, TextMemoryKey{Scope: ScopeUser, ScopeID: "u1", Name: "debugging"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(ctx, q, TextMemoryKey{Scope: ScopeUser, ScopeID: "u1", Name: "debugging"}); !errors.IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
	if err := DeleteByScope(ctx, q, ScopeUser, "u1"); err != nil {
		t.Fatal(err)
	}
	listed, err = List(ctx, q, ScopeUser, "u1")
	if err != nil || len(listed) != 0 {
		t.Fatalf("cleared list %v %v", listed, err)
	}
}

func TestUpsertIndexOverBudgetStillWrites(t *testing.T) {
	q, ctx := testQueries(t)
	item, err := Upsert(ctx, q, TextMemory{Scope: ScopeWorkspace, ScopeID: "default", Name: NameIndex, Content: overBudgetIndex()})
	if err != nil {
		t.Fatal(err)
	}
	if !item.OverBudget {
		t.Fatal("expected over budget")
	}
	got, err := Get(ctx, q, TextMemoryKey{Scope: ScopeWorkspace, ScopeID: "default", Name: NameIndex})
	if err != nil || !got.OverBudget {
		t.Fatalf("stored %+v err=%v", got, err)
	}
}

func TestIndexAndSearchMessages(t *testing.T) {
	q, ctx := testQueries(t)
	if err := IndexMessage(ctx, q, ContextMessage{
		ID:          "m1",
		WorkspaceID: "ws1",
		SessionID:   "s1",
		RunID:       "r1",
		Role:        "user",
		Content:     "remember the docking checklist",
	}); err != nil {
		t.Fatal(err)
	}
	if err := IndexMessage(ctx, q, ContextMessage{
		ID:          "m2",
		WorkspaceID: "ws2",
		SessionID:   "s2",
		Role:        "user",
		Content:     "remember the docking checklist",
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := SearchMessages(ctx, q, Search{WorkspaceID: "ws1", Query: "docking"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Message.ID != "m1" {
		t.Fatalf("hits %+v", hits)
	}
	if _, err := SearchMessages(ctx, q, Search{WorkspaceID: "ws1"}); !errors.IsInvalid(err) {
		t.Fatalf("expected invalid query, got %v", err)
	}
}
