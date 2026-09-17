package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestRequestIDRoundTrip(t *testing.T) {
	cases := []RequestID{IntID(7), StringID("req-1"), ParseRequestID("42"), ParseRequestID("abc")}
	for _, id := range cases {
		raw, err := json.Marshal(id)
		if err != nil {
			t.Fatal(err)
		}
		var out RequestID
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if out.Key() != id.Key() || out.String() != id.String() {
			t.Fatalf("%s -> %s key=%s want %s", raw, out.String(), out.Key(), id.Key())
		}
	}
	var unset RequestID
	raw, _ := json.Marshal(unset)
	if string(raw) != "null" {
		t.Fatalf("unset = %s", raw)
	}
	if err := json.Unmarshal([]byte("true"), &unset); err == nil {
		t.Fatal("expected error")
	}
}

func TestClassify(t *testing.T) {
	id := IntID(1)
	if Classify(Envelope{ID: &id, Method: "initialize"}) != KindRequest {
		t.Fatal("request")
	}
	if Classify(Envelope{Method: "initialized"}) != KindNotification {
		t.Fatal("notification")
	}
	if Classify(Envelope{ID: &id, Result: json.RawMessage(`{}`)}) != KindResponse {
		t.Fatal("response")
	}
	if Classify(Envelope{}) != KindUnknown {
		t.Fatal("unknown")
	}
}

func TestJSONLSkipEmptyAndOversize(t *testing.T) {
	pr, pw := io.Pipe()
	tr := NewJSONL(pr, io.Discard, pw, 16)
	go func() {
		_, _ = pw.Write([]byte("\n\n{\"ok\":true}\n" + strings.Repeat("x", 32) + "\n"))
		_ = pw.Close()
	}()
	ctx := context.Background()
	frame, err := tr.Read(ctx)
	if err != nil || string(frame) != `{"ok":true}` {
		t.Fatalf("frame=%q err=%v", frame, err)
	}
	if _, err := tr.Read(ctx); err == nil {
		t.Fatal("expected oversize")
	}
}

func TestJSONLTruncated(t *testing.T) {
	pr, pw := io.Pipe()
	tr := NewJSONL(pr, io.Discard, pw, 0)
	go func() {
		_, _ = pw.Write([]byte(`{"partial":`))
		_ = pw.Close()
	}()
	if _, err := tr.Read(context.Background()); err == nil {
		t.Fatal("expected truncated")
	}
}

func TestJSONLWriteCancelled(t *testing.T) {
	tr := NewJSONL(bytes.NewReader(nil), io.Discard, nil, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tr.Write(ctx, []byte(`{}`)); err == nil {
		t.Fatal("expected cancel")
	}
}

func TestRPCErrorError(t *testing.T) {
	plain := RPCError{Code: 1}
	withMsg := RPCError{Code: 1, Message: "nope"}
	if plain.Error() == "" || withMsg.Error() == "" {
		t.Fatal("error string")
	}
}
