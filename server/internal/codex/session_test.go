package codex

import (
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
