package codex

import (
	"context"
	"testing"

	pkg "codedock/pkg/codex"
)

func TestDedupeSessionsKeepsNewest(t *testing.T) {
	got := dedupeSessions([]pkg.Session{
		{ID: "a", Preview: "old", UpdatedAt: 1},
		{ID: "b", Preview: "only", UpdatedAt: 2},
		{ID: "a", Preview: "new", UpdatedAt: 3},
		{ID: "a", Preview: "mid", UpdatedAt: 2},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].Preview != "new" {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].ID != "b" {
		t.Fatalf("second %+v", got[1])
	}
}

func TestCreateSessionFirstTurnSkipsResume(t *testing.T) {
	fake := NewFakeHandler()
	rt := loopRT(t, fake)
	session, err := rt.CreateSession(context.Background(), pkg.Settings{Cwd: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := rt.StartTurn(context.Background(), session.ID, "hello", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status == pkg.TurnFailed {
		t.Fatalf("turn %+v", turn)
	}
}

func TestGetSessionResumesStoredThread(t *testing.T) {
	fake := NewFakeHandler()
	fake.SeedStored("hist-1", "stored")
	rt := loopRT(t, fake)
	got, progress, err := rt.GetSession(context.Background(), "hist-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "hist-1" {
		t.Fatalf("session %+v", got)
	}
	if len(progress) == 0 {
		t.Fatal("expected history after resume")
	}
	if !fake.isLoaded("hist-1") {
		t.Fatal("thread should be resumed")
	}
}

func TestStartTurnSendsConfiguredModel(t *testing.T) {
	fake := NewFakeHandler()
	fake.SeedStored("hist-model", "stored")
	var gotModel, gotEffort string
	fake.OnRequest = func(env pkg.Envelope) {
		if env.Method != pkg.MethodTurnStart {
			return
		}
		gotModel = jsonField(env.Params, "model")
		gotEffort = jsonField(env.Params, "effort")
	}
	rt := loopRT(t, fake)
	if _, err := rt.StartTurn(context.Background(), "hist-model", "continue", pkg.Input{}, pkg.InputStart); err != nil {
		t.Fatal(err)
	}
	if gotModel != "gpt-5.6" {
		t.Fatalf("model=%q effort=%q", gotModel, gotEffort)
	}
	if gotEffort != "medium" {
		t.Fatalf("effort=%q", gotEffort)
	}
}

func TestStartTurnResumesStoredThread(t *testing.T) {
	fake := NewFakeHandler()
	fake.SeedStored("hist-2", "stored")
	rt := loopRT(t, fake)
	turn, err := rt.StartTurn(context.Background(), "hist-2", "continue", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status == pkg.TurnFailed {
		t.Fatalf("turn %+v", turn)
	}
	if !fake.isLoaded("hist-2") {
		t.Fatal("thread should be resumed")
	}
}
