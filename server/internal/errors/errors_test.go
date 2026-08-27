package errors

import "testing"

func TestWrapAndIs(t *testing.T) {
	err := NotFound("session %s", "abc")
	if !IsNotFound(err) {
		t.Fatal("expected NotFound")
	}
	if !Is(err, ErrNotFound) {
		t.Fatal("expected Is ErrNotFound")
	}
	if IsConflict(err) {
		t.Fatal("did not expect Conflict")
	}
}
