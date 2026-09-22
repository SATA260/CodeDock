package board

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codedock/internal/util"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

// testQueries 打开一块迁移过的内存库。
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

func TestWorkCheckoutPlacement(t *testing.T) {
	q, ctx := testQueries(t)
	work, err := CreateWork(ctx, q, "t1", "u1", "修登录")
	if err != nil {
		t.Fatal(err)
	}
	info, err := GetInfo(ctx, q, work.ID, "")
	if err != nil || info.WorkID != work.ID {
		t.Fatalf("blank work info: %+v %v", info, err)
	}
	dir := t.TempDir()
	co, err := AttachCheckout(ctx, q, work.ID, dir, CheckoutFolder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PutInfo(ctx, q, work.ID, co.Path, "这个目录写 API"); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(ctx, q, Placement{Engine: EngineNative, SessionID: "s-dir", WorkID: work.ID, Checkout: co.Path}); err != nil {
		t.Fatal(err)
	}
	talk, err := AttachExisting(ctx, q, EngineNative, "s-talk", work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if talk.Checkout != "" {
		t.Fatalf("talk checkout %q", talk.Checkout)
	}
	if _, err := Bind(ctx, q, Placement{Engine: EngineNative, SessionID: "s-talk", WorkID: work.ID, Checkout: co.Path}); err == nil {
		t.Fatal("talk session must not bind a directory later")
	}
	if err := DetachCheckout(ctx, q, work.ID, co.Path); err != nil {
		t.Fatal(err)
	}
	place, err := GetPlacement(ctx, q, EngineNative, "s-dir")
	if err != nil {
		t.Fatal(err)
	}
	if place.Checkout != "" {
		t.Fatalf("detached session checkout %q", place.Checkout)
	}
	if HasCheckout(ctx, q, work.ID, dir) {
		t.Fatal("detached path should not stay attached")
	}
	if err := DeleteWork(ctx, q, work.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetPlacement(ctx, q, EngineNative, "s-talk"); err == nil {
		t.Fatal("delete work should unbind placements")
	}
}

func TestStartTalkAndInDir(t *testing.T) {
	q, ctx := testQueries(t)
	work, err := CreateWork(ctx, q, "t1", "u1", "卡")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := AttachCheckout(ctx, q, work.ID, dir, CheckoutPrimary); err != nil {
		t.Fatal(err)
	}
	svc := New(q, Ports{
		StartNative: func(_ context.Context, spec StartSpec) (Started, error) {
			id := "talk"
			if spec.WorkspaceID != "" {
				id = "dir"
			}
			return Started{ID: id, Summary: spec.WorkspaceID, UpdatedAt: util.FormatTime(util.Now())}, nil
		},
	})
	talk, started, err := svc.StartTalk(ctx, work.ID, StartSpec{Engine: EngineNative, UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if started.ID != "talk" || talk.Checkout != "" {
		t.Fatalf("talk %+v %+v", talk, started)
	}
	inDir, started, err := svc.StartInDir(ctx, work.ID, dir, StartSpec{Engine: EngineNative, UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if started.ID != "dir" || inDir.Checkout == "" {
		t.Fatalf("dir %+v %+v", inDir, started)
	}
	if _, _, err := svc.StartInDir(ctx, work.ID, filepath.Join(os.TempDir(), "missing-codedock-dir"), StartSpec{Engine: EngineNative}); err == nil {
		t.Fatal("missing checkout must fail")
	}
}

func TestAttachCheckoutRequiresExistingDir(t *testing.T) {
	q, ctx := testQueries(t)
	work, err := CreateWork(ctx, q, "t1", "u1", "卡")
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(os.TempDir(), "codedock-missing-checkout-"+t.Name())
	if _, err := AttachCheckout(ctx, q, work.ID, missing, CheckoutFolder); err == nil {
		t.Fatal("missing directory must not attach")
	}
}

func TestBoardCardsAndPacket(t *testing.T) {
	q, ctx := testQueries(t)
	work, err := CreateWork(ctx, q, "t1", "u1", "卡")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PutInfo(ctx, q, work.ID, "", "卡片说明"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	co, err := AttachCheckout(ctx, q, work.ID, dir, CheckoutFolder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(ctx, q, Placement{Engine: EngineNative, SessionID: "s1", WorkID: work.ID, Checkout: co.Path}); err != nil {
		t.Fatal(err)
	}
	if _, err := PutIssue(ctx, q, IssueSnap{Engine: EngineNative, SessionID: "s1", Repo: "acme/app", Number: 15, Title: "看板"}); err != nil {
		t.Fatal(err)
	}
	svc := New(q, Ports{
		InspectDir: func(path string) DirView {
			return DirView{Path: path, Branch: "main", Dirty: true, SharedWriters: []string{"/other"}}
		},
		ListEngine: func(_ context.Context, engine Engine) ([]SessionMeta, error) {
			if engine != EngineClaude {
				return nil, nil
			}
			return []SessionMeta{{ID: "c1", Summary: "claude 闲聊", UpdatedAt: "z"}}, nil
		},
	})
	view, err := svc.Cards(ctx, "t1", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Cards) != 1 || len(view.Cards[0].Sessions) != 1 {
		t.Fatalf("cards=%+v", view)
	}
	if view.Cards[0].Dirs[0].Branch != "main" || !view.Cards[0].Dirs[0].Dirty {
		t.Fatalf("dir %+v", view.Cards[0].Dirs[0])
	}
	if len(view.Ungrouped) != 1 || view.Ungrouped[0].SessionID != "c1" {
		t.Fatalf("ungrouped %+v", view.Ungrouped)
	}
	pkt, err := BuildPacket(ctx, q, EngineNative, "s1")
	if err != nil || pkt.Text == "" || !strings.Contains(pkt.Text, "卡片说明") || !strings.Contains(pkt.Text, "#15") {
		t.Fatalf("packet %+v %v", pkt, err)
	}
	if got := PrefixContent(pkt.Text, "继续"); !strings.Contains(got, "只读") || !strings.Contains(got, "继续") {
		t.Fatalf("prefix %q", got)
	}
}

func TestReplaceLinks(t *testing.T) {
	q, ctx := testQueries(t)
	first, err := ReplaceLinks(ctx, q, EngineNative, "s1", &IssueSnap{
		Engine: EngineNative, SessionID: "s1", Repo: "acme/app", Number: 15, Title: "旧", URL: "https://github.com/acme/app/issues/15",
	}, []PullSnap{{
		Engine: EngineNative, SessionID: "s1", Repo: "acme/app", Number: 3, Title: "旧 PR",
	}})
	if err != nil || first.Issue == nil || len(first.Pulls) != 1 {
		t.Fatalf("first %+v %v", first, err)
	}
	next, err := ReplaceLinks(ctx, q, EngineNative, "s1", nil, []PullSnap{{
		Engine: EngineNative, SessionID: "s1", Repo: "acme/app", Number: 8, Title: "新 PR", URL: "https://github.com/acme/app/pull/8",
	}})
	if err != nil || next.Issue != nil || len(next.Pulls) != 1 || next.Pulls[0].Number != 8 {
		t.Fatalf("replaced %+v %v", next, err)
	}
}

func TestParseEngine(t *testing.T) {
	eng, err := ParseEngine("agent")
	if err != nil || eng != EngineNative || PublicEngine(eng) != "agent" {
		t.Fatalf("agent %s %v", eng, err)
	}
	if _, err := ParseEngine("nope"); err == nil {
		t.Fatal("expected invalid engine")
	}
}
