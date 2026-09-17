package codex

import (
	"context"
	"os"
	"testing"
	"time"

	pkg "codedock/pkg/codex"
)

func TestRuntimeProbeAndCatalog(t *testing.T) {
	fake := NewFakeHandler()
	rt := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.149.0\n", nil },
		Start:    LoopbackStarter(fake.Handle),
	})
	defer rt.Close()
	ctx := context.Background()
	status, err := rt.Probe(ctx)
	if err != nil || !status.Available || !status.Authorized || status.Version != "0.149.0" {
		t.Fatalf("%+v %v", status, err)
	}
	models, err := rt.ListModels(ctx)
	if err != nil || len(models) == 0 {
		t.Fatal(models, err)
	}
	modes, err := rt.ListModes(ctx)
	if err != nil || len(modes) < 3 {
		t.Fatal(modes, err)
	}
	if len(rt.Commands()) == 0 {
		t.Fatal("commands")
	}
}

func TestRuntimeSessionTurnQueueAsk(t *testing.T) {
	fake := NewFakeHandler()
	rt := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.149.0", nil },
		Start:    LoopbackStarter(fake.Handle),
	})
	defer rt.Close()
	ctx := context.Background()
	session, err := rt.CreateSession(ctx, pkg.Settings{Cwd: "/tmp", Model: "gpt-5.6"})
	if err != nil || session.ID == "" {
		t.Fatal(session, err)
	}
	if _, err := rt.ApplySettings(ctx, session.ID, pkg.Settings{Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Mention(ctx, session.ID, "a.go"); err != nil {
		t.Fatal(err)
	}
	if err := rt.AttachImage(ctx, session.ID, "x.png"); err != nil {
		t.Fatal(err)
	}
	got, progress, err := rt.GetSession(ctx, session.ID)
	if err != nil || got.ID != session.ID {
		t.Fatal(got, progress, err)
	}
	page, err := rt.ListSessions(ctx, false, "")
	if err != nil || len(page.Sessions) == 0 {
		t.Fatal(page, err)
	}

	fake.SendAsk = true
	turn, err := rt.StartTurn(ctx, session.ID, "do it", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := rt.StartTurn(ctx, session.ID, "next", pkg.Input{}, pkg.InputStart)
	if err != nil || queued.Status != pkg.TurnQueued {
		t.Fatal(queued, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var asks []pkg.ApprovalAsk
	for time.Now().Before(deadline) {
		asks = rt.PendingAsks(session.ID)
		if len(asks) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(asks) == 0 {
		t.Fatal("expected ask")
	}
	if err := rt.Decide(ctx, asks[0].ID, pkg.AskAnswer{Approved: true, Scope: pkg.ScopeOnce}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Decide(ctx, asks[0].ID, pkg.AskAnswer{Approved: true}); err == nil {
		t.Fatal("late decide")
	}
	if err := rt.Rename(ctx, session.ID, "Title"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Fork(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Compact(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Review(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Invoke(ctx, session.ID, "mcp", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Invoke(ctx, session.ID, "model", "gpt-5.6"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Archive(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.StartTurn(ctx, session.ID, "no", pkg.Input{}, pkg.InputStart); err == nil {
		t.Fatal("archived")
	}
	events, _ := rt.Events(session.ID, 0)
	if len(events) == 0 {
		t.Fatal("events")
	}
	ch, unsub := rt.Subscribe(session.ID)
	unsub()
	select {
	case <-ch:
	default:
	}
	_ = turn
}

func TestRuntimeUnauthorizedAndMissing(t *testing.T) {
	fake := NewFakeHandler()
	fake.Authorized = false
	rt := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.1", nil },
		Start:    LoopbackStarter(fake.Handle),
	})
	defer rt.Close()
	if _, err := rt.CreateSession(context.Background(), pkg.Settings{}); err == nil {
		t.Fatal("expected unauthorized")
	}
	rt2 := New(Options{
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	})
	status, err := rt2.Probe(context.Background())
	if err != nil || status.Available {
		t.Fatal(status, err)
	}
}

func TestParseVersion(t *testing.T) {
	if ParseVersion("WARNING: x\ncodex-cli 0.149.0\n") != "0.149.0" {
		t.Fatal(ParseVersion("WARNING: x\ncodex-cli 0.149.0\n"))
	}
	if ParseVersion("codex 1.2.3") != "1.2.3" {
		t.Fatal(ParseVersion("codex 1.2.3"))
	}
	if ParseVersion("\n0.1\n") != "0.1" {
		t.Fatal(ParseVersion("0.1"))
	}
	if ParseVersion(" \n") != "" {
		t.Fatal("empty")
	}
	Drain(nil)
}

func TestRuntimeCancelQueuedAndUnknown(t *testing.T) {
	fake := NewFakeHandler()
	fake.SendAsk = true
	fake.Unknown = true
	rt := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.149.0", nil },
		Start:    LoopbackStarter(fake.Handle),
	})
	defer rt.Close()
	ctx := context.Background()
	session, err := rt.CreateSession(ctx, pkg.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := rt.StartTurn(ctx, session.ID, "one", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rt.StartTurn(ctx, session.ID, "two", pkg.Input{}, pkg.InputQueue)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.CancelTurn(ctx, session.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	_ = rt.CancelTurn(ctx, session.ID, first.ID)
}

func TestRuntimeInterruptAfterStart(t *testing.T) {
	fake := NewFakeHandler()
	fake.SendAsk = true
	rt := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.149.0", nil },
		Start:    LoopbackStarter(fake.Handle),
	})
	defer rt.Close()
	ctx := context.Background()
	session, err := rt.CreateSession(ctx, pkg.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := rt.StartTurn(ctx, session.ID, "one", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		st := rt.state(session.ID)
		st.mu.Lock()
		ready := st.active != nil && st.active.CodexID != ""
		st.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := rt.CancelTurn(ctx, session.ID, turn.ID); err != nil {
		t.Fatal(err)
	}
}

func loopRT(t *testing.T, fake *FakeHandler) *Runtime {
	t.Helper()
	if fake == nil {
		fake = NewFakeHandler()
	}
	rt := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.149.0", nil },
		Start:    LoopbackStarter(fake.Handle),
	})
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

func TestForkTitleSequence(t *testing.T) {
	rt := loopRT(t, nil)
	ctx := context.Background()
	session, err := rt.CreateSession(ctx, pkg.Settings{Cwd: "/tmp", Model: "gpt-5.6"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Rename(ctx, session.ID, "ping"); err != nil {
		t.Fatal(err)
	}
	first, err := rt.Fork(ctx, session.ID)
	if err != nil || first.Title != "ping (1)" {
		t.Fatalf("first %+v %v", first, err)
	}
	second, err := rt.Fork(ctx, session.ID)
	if err != nil || second.Title != "ping (2)" {
		t.Fatalf("second %+v %v", second, err)
	}
	third, err := rt.Fork(ctx, first.ID)
	if err != nil || third.Title != "ping (3)" {
		t.Fatalf("third %+v %v", third, err)
	}
	if nextForkTitle("ping", []string{"ping"}) != "ping (1)" || nextForkTitle("", nil) != "fork (1)" {
		t.Fatal("nextForkTitle")
	}
}

func TestRuntimeInvokeExpireRingAndFailTurn(t *testing.T) {
	fake := NewFakeHandler()
	rt := loopRT(t, fake)
	ctx := context.Background()
	session, err := rt.CreateSession(ctx, pkg.Settings{Model: "gpt-5.6"})
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []struct{ name, args string }{
		{"effort", "high"},
		{"plan", ""},
		{"permissions", "read-only"},
		{"approval", "never"},
		{"mention", "a.go"},
		{"image", "a.png"},
		{"rename", "Title"},
		{"mcp", ""},
		{"skills", ""},
	} {
		if _, err := rt.Invoke(ctx, session.ID, cmd.name, cmd.args); err != nil {
			t.Fatalf("%s: %v", cmd.name, err)
		}
	}
	if _, err := rt.Invoke(ctx, session.ID, "nope", ""); err == nil {
		t.Fatal("unknown command")
	}
	if _, err := rt.Invoke(ctx, session.ID, "fork", ""); err != nil {
		t.Fatal(err)
	}
	if err := rt.Mention(ctx, session.ID, ""); err == nil {
		t.Fatal("empty mention")
	}
	if err := rt.AttachImage(ctx, session.ID, ""); err == nil {
		t.Fatal("empty image")
	}
	if _, err := rt.StartTurn(ctx, "", "x", pkg.Input{}, pkg.InputStart); err == nil {
		t.Fatal("empty session")
	}
	if err := rt.CancelTurn(ctx, session.ID, ""); err == nil {
		t.Fatal("empty turn")
	}
	if err := rt.CancelTurn(ctx, session.ID, "missing"); err == nil {
		t.Fatal("missing turn")
	}
	if err := rt.Expire(ctx, "missing"); err == nil {
		t.Fatal("missing ask")
	}

	fake.FailTurn = true
	if _, err := rt.StartTurn(ctx, session.ID, "fail", pkg.Input{Text: "x"}, pkg.InputStart); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, _ := rt.Events(session.ID, 0)
		for _, ev := range events {
			if ev.Type == pkg.EventTurnFailed {
				goto failed
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected failed turn")
failed:
	fake.FailTurn = false
	fake.SendAsk = true
	turn, err := rt.StartTurn(ctx, session.ID, "ask", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	var askID string
	askDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(askDeadline) {
		asks := rt.PendingAsks(session.ID)
		if len(asks) > 0 {
			askID = asks[0].ID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if askID == "" {
		t.Fatal("expected ask")
	}
	if err := rt.Expire(ctx, askID); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Invoke(ctx, session.ID, "stop", ""); err != nil && err.Error() == "" {
		t.Fatal(err)
	}
	_ = turn
	if _, err := rt.Invoke(ctx, session.ID, "compact", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Invoke(ctx, session.ID, "review", ""); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 300; i++ {
		rt.emit(session.ID, pkg.Event{Type: pkg.EventNotice, Notice: "n"})
	}
	events, reset := rt.Events(session.ID, 0)
	if !reset || len(events) == 0 {
		t.Fatalf("reset=%v n=%d", reset, len(events))
	}
	if rt.state(session.ID).ring.last() < 256 {
		t.Fatal("ring seq")
	}
	if _, err := rt.Invoke(ctx, session.ID, "archive", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeHandshakeStartAndVersionErrors(t *testing.T) {
	ctx := context.Background()
	rt := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "", os.ErrNotExist },
		Start:    LoopbackStarter(NewFakeHandler().Handle),
	})
	defer rt.Close()
	status, err := rt.Probe(ctx)
	if err != nil || !status.Available || status.Version != "" {
		t.Fatal(status, err)
	}

	rt2 := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.1", nil },
		Start:    func(context.Context, string) (Proc, error) { return nil, os.ErrPermission },
	})
	defer rt2.Close()
	status, err = rt2.Probe(ctx)
	if err != nil || status.Hint == "" {
		t.Fatal(status, err)
	}
	if _, err := rt2.CreateSession(ctx, pkg.Settings{}); err == nil {
		t.Fatal("expected start error")
	}

	rt3 := New(Options{
		LookPath: func(string) (string, error) { return "/bin/codex", nil },
		Version:  func(context.Context, string) (string, error) { return "codex-cli 0.1", nil },
		Start: LoopbackStarter(func(env pkg.Envelope) []pkg.Envelope {
			if env.ID != nil {
				return []pkg.Envelope{{ID: env.ID, Error: &pkg.RPCError{Code: -32603, Message: "not initialized"}}}
			}
			return nil
		}),
	})
	defer rt3.Close()
	if _, err := rt3.CreateSession(ctx, pkg.Settings{}); err == nil {
		t.Fatal("expected handshake error")
	}

	fake := NewFakeHandler()
	rt4 := loopRT(t, fake)
	session, err := rt4.CreateSession(ctx, pkg.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	fake.SendAsk = true
	if _, err := rt4.StartTurn(ctx, session.ID, "x", pkg.Input{}, pkg.InputStart); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(rt4.PendingAsks(session.ID)) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := rt4.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeQueueAfterComplete(t *testing.T) {
	fake := NewFakeHandler()
	rt := loopRT(t, fake)
	ctx := context.Background()
	session, err := rt.CreateSession(ctx, pkg.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := rt.StartTurn(ctx, session.ID, "one", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := rt.state(session.ID)
		st.mu.Lock()
		idle := st.active == nil
		st.mu.Unlock()
		if idle {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	second, err := rt.StartTurn(ctx, session.ID, "two", pkg.Input{}, pkg.InputStart)
	if err != nil {
		t.Fatal(err)
	}
	_ = first
	_ = second
	got, _, err := rt.GetSession(ctx, "missing")
	if err == nil {
		t.Fatal(got)
	}
}

func TestRuntimeCancelActiveWithoutCodexID(t *testing.T) {
	fake := NewFakeHandler()
	rt := loopRT(t, fake)
	ctx := context.Background()
	session, err := rt.CreateSession(ctx, pkg.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	st := rt.state(session.ID)
	st.mu.Lock()
	st.active = &pkg.Turn{ID: "local-1", SessionID: session.ID, Status: pkg.TurnRunning}
	st.queue = []queuedTurn{{turn: pkg.Turn{ID: "q1", SessionID: session.ID, Status: pkg.TurnQueued}, input: pkg.Input{Text: "n"}}}
	st.mu.Unlock()
	if err := rt.CancelTurn(ctx, session.ID, "local-1"); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeMapRPCMessages(t *testing.T) {
	err := mapRPC(pkg.RPCError{Message: "unknown thread xyz"})
	if err == nil {
		t.Fatal("expected mapped")
	}
	err = mapRPC(pkg.RPCError{Message: "not initialized"})
	if err == nil {
		t.Fatal("expected unavailable")
	}
	err = mapRPC(pkg.RPCError{Message: "bad params"})
	if err == nil {
		t.Fatal("expected invalid")
	}
	if mapRPC(context.Canceled) != context.Canceled {
		t.Fatal("canceled")
	}
	if mapRPC(nil) != nil {
		t.Fatal("nil")
	}
}

func TestRuntimeTokenUsageNotification(t *testing.T) {
	rt := loopRT(t, nil)
	rt.onNotification(pkg.Message{
		Kind:   pkg.KindNotification,
		Method: pkg.MethodThreadTokenUsageUpdated,
		Params: []byte(`{"threadId":"th","turnId":"u1","tokenUsage":{"modelContextWindow":1000,"last":{"totalTokens":250},"total":{"totalTokens":900}}}`),
	})
	got := rt.Usage("th")
	if got.Used != 250 || got.Window != 1000 {
		t.Fatalf("%+v", got)
	}
	events, _ := rt.Events("th", 0)
	found := false
	for _, ev := range events {
		if ev.Type == pkg.EventTokenUsage && ev.Usage != nil && ev.Usage.Used == 250 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected token.usage event")
	}
}

func TestRuntimeResolveAskNotification(t *testing.T) {
	rt := loopRT(t, nil)
	st := rt.state("th")
	st.mu.Lock()
	st.asks["7"] = &askMem{ask: pkg.ApprovalAsk{ID: "7", TurnID: "u", ThreadID: "th"}}
	st.mu.Unlock()
	rt.onNotification(pkg.Message{
		Kind:   pkg.KindNotification,
		Method: pkg.MethodServerRequestResolved,
		Params: []byte(`{"threadId":"th","requestId":7}`),
	})
	rt.onNotification(pkg.Message{
		Kind:   pkg.KindNotification,
		Method: pkg.MethodServerRequestResolved,
		Params: []byte(`{}`),
	})
	events, _ := rt.Events("th", 0)
	found := false
	for _, ev := range events {
		if ev.Type == pkg.EventAskResolved {
			found = true
		}
	}
	if !found {
		t.Fatal("expected resolved event")
	}
}
