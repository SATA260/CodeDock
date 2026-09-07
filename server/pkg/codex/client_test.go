package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func scriptedClient(t *testing.T, handle func(server Transport, env Envelope) *Envelope) *Client {
	t.Helper()
	local, remote := PipePair()
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
	})
	go func() {
		ctx := context.Background()
		for {
			frame, err := remote.Read(ctx)
			if err != nil {
				return
			}
			var env Envelope
			if json.Unmarshal(frame, &env) != nil {
				return
			}
			resp := handle(remote, env)
			if resp == nil {
				continue
			}
			if resp.ID == nil && env.ID != nil {
				resp.ID = env.ID
			}
			body, err := json.Marshal(resp)
			if err != nil {
				return
			}
			if err := remote.Write(ctx, body); err != nil {
				return
			}
		}
	}()
	return NewClient(local)
}

func TestClientHandshakeAndCall(t *testing.T) {
	client := scriptedClient(t, func(_ Transport, env Envelope) *Envelope {
		switch env.Method {
		case MethodInitialize:
			return &Envelope{Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}
		case MethodInitialized:
			return nil
		case MethodAccountRead:
			return &Envelope{Result: json.RawMessage(`{"requiresOpenaiAuth":false,"account":{"type":"apiKey"}}`)}
		default:
			return &Envelope{Error: &RPCError{Code: CodeMethodNotFound, Message: env.Method}}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := client.Handshake(ctx, DefaultClientInfo())
	if err != nil {
		t.Fatal(err)
	}
	if got.CodexHome != "/tmp" || got.PlatformOS != "macos" {
		t.Fatalf("%+v", got)
	}
	account, err := client.AccountRead(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !Authorized(account) {
		t.Fatal("authorized")
	}
}

func TestClientOutOfOrderAndNotification(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	client := scriptedClient(t, func(server Transport, env Envelope) *Envelope {
		mu.Lock()
		seen = append(seen, env.Method)
		mu.Unlock()
		switch env.Method {
		case "slow":
			note, _ := json.Marshal(Envelope{Method: MethodAgentMessageDelta, Params: json.RawMessage(`{"threadId":"t","turnId":"u","itemId":"i","delta":"hi"}`)})
			_ = server.Write(context.Background(), note)
			time.Sleep(30 * time.Millisecond)
			return &Envelope{Result: json.RawMessage(`{"n":1}`)}
		case "fast":
			return &Envelope{Result: json.RawMessage(`{"n":2}`)}
		default:
			return &Envelope{Result: json.RawMessage(`{}`)}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var slow, fast map[string]int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = client.Call(ctx, "slow", map[string]any{}, &slow)
	}()
	time.Sleep(10 * time.Millisecond)
	go func() {
		defer wg.Done()
		_ = client.Call(ctx, "fast", map[string]any{}, &fast)
	}()
	select {
	case msg := <-client.Incoming():
		if msg.Method != MethodAgentMessageDelta {
			t.Fatalf("method %s", msg.Method)
		}
	case <-time.After(time.Second):
		t.Fatal("missing notification")
	}
	wg.Wait()
	if slow["n"] != 1 || fast["n"] != 2 {
		t.Fatalf("slow=%v fast=%v", slow, fast)
	}
}

func TestClientCancelDropsLateResponse(t *testing.T) {
	started := make(chan struct{})
	client := scriptedClient(t, func(_ Transport, env Envelope) *Envelope {
		if env.Method == "hang" {
			close(started)
			time.Sleep(80 * time.Millisecond)
			return &Envelope{Result: json.RawMessage(`{"ok":true}`)}
		}
		return &Envelope{Result: json.RawMessage(`{}`)}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	err := client.Call(ctx, "hang", map[string]any{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	time.Sleep(120 * time.Millisecond)
}

func TestClientBadJSONEnds(t *testing.T) {
	local, remote := PipePair()
	t.Cleanup(func() { _ = local.Close(); _ = remote.Close() })
	client := NewClient(local)
	_ = remote.Write(context.Background(), []byte(`{not json`))
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("client did not stop")
	}
	if client.Err() == nil {
		t.Fatal("expected error")
	}
}

func TestClientServerRequestAndReply(t *testing.T) {
	client := scriptedClient(t, func(server Transport, env Envelope) *Envelope {
		if env.Method != MethodInitialize {
			return &Envelope{Result: json.RawMessage(`{}`)}
		}
		id := StringID("ask-1")
		req, _ := json.Marshal(Envelope{
			ID:     &id,
			Method: MethodItemCommandApproval,
			Params: json.RawMessage(`{"threadId":"th","turnId":"tu","command":"ls"}`),
		})
		_ = server.Write(context.Background(), req)
		return &Envelope{Result: json.RawMessage(`{"codexHome":"/","platformFamily":"unix","platformOs":"linux","userAgent":"x"}`)}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Handshake(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-client.Incoming():
		if msg.Kind != KindRequest || msg.Method != MethodItemCommandApproval {
			t.Fatalf("%+v", msg)
		}
		if err := client.Reply(ctx, msg.ID, map[string]any{"decision": "accept"}); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing server request")
	}
}

func TestClientUnknownMethodReplyError(t *testing.T) {
	client := scriptedClient(t, func(_ Transport, env Envelope) *Envelope {
		return &Envelope{Result: json.RawMessage(`{}`)}
	})
	ctx := context.Background()
	if err := client.ReplyError(ctx, StringID("x"), CodeMethodNotFound, "unsupported"); err != nil {
		t.Fatal(err)
	}
}

func TestClientCloseUnblocksCall(t *testing.T) {
	local, remote := PipePair()
	client := NewClient(local)
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = local.Close()
		_ = remote.Close()
	}()
	err := client.Call(context.Background(), "noop", map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, io.EOF) && err.Error() == "" {
		t.Fatalf("err=%v", err)
	}
}

func TestCallRawNilResult(t *testing.T) {
	client := scriptedClient(t, func(_ Transport, env Envelope) *Envelope {
		return &Envelope{Result: json.RawMessage(`null`)}
	})
	if err := client.Call(context.Background(), "x", nil, nil); err != nil {
		t.Fatal(err)
	}
}
