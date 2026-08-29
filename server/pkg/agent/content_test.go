package agent

import "testing"

func TestDecodeTextEmptyObject(t *testing.T) {
	if got := DecodeText(EncodeText("")); got != "" {
		t.Fatalf("empty text = %q", got)
	}
	if got := DecodeText([]byte(`{"text":""}`)); got != "" {
		t.Fatalf("empty json object = %q", got)
	}
	if got := DecodeText(EncodeText("hello")); got != "hello" {
		t.Fatalf("hello = %q", got)
	}
}
