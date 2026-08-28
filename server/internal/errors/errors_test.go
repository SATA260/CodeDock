package errors

import "testing"

// TestWrapAndIs 校验包装错误能被 Is / IsNotFound 识别。
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
