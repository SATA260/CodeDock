package agent

import (
	"context"
	"testing"

	"codedock/internal/agent/memory"
	"codedock/pkg/db"
)

func TestLoadMemoryIndexesUsesUserAndWork(t *testing.T) {
	ctx := context.Background()
	client, err := db.Open(ctx, db.Config{Engine: db.EngineSQLite, DSN: "file:memindex?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := db.Migrate(ctx, client.DB()); err != nil {
		t.Fatal(err)
	}
	q := db.SQLiteQueries(client)
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{Scope: memory.ScopeUser, ScopeID: "u1", Name: memory.NameIndex, Content: "user-dir"}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{Scope: memory.ScopeWorkspace, ScopeID: "ws1", Name: memory.NameIndex, Content: "ws-dir"}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Upsert(ctx, q, memory.TextMemory{Scope: memory.ScopeWork, ScopeID: "work-1", Name: memory.NameIndex, Content: "work-dir"}); err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{queries: q}
	got := rt.loadMemoryIndexes(ctx, "u1", "work-1")
	if len(got) != 2 || got[0] != "user-dir" || got[1] != "work-dir" {
		t.Fatalf("indexes=%v", got)
	}
}
