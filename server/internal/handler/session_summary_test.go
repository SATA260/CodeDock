package handler

import (
	"strings"
	"testing"
)

func TestFirstUserSummary(t *testing.T) {
	if got := firstUserSummary("  hello world  "); got != "hello world" {
		t.Fatalf("got %q", got)
	}
	if got := firstUserSummary("first line\nsecond"); got != "first line" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("你", 240)
	if got := firstUserSummary(long); got != strings.Repeat("你", 200) {
		t.Fatalf("should clip to 200 runes, len=%d", len([]rune(got)))
	}
}
