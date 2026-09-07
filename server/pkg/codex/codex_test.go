package codex

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestJSONLTruncatedAndLimit(t *testing.T) {
	left, right := PipePair()
	limited := left.(*JSONL)
	limited.max = 8
	go func() { _ = right.Write(context.Background(), []byte(strings.Repeat("a", 32))) }()
	if _, err := limited.Read(context.Background()); err == nil {
		t.Fatal("expected overflow")
	}
	_ = left.Close()
	_ = right.Close()
}

func TestHandshakeAndCall(t *testing.T) {
	client := StartLoopback(func(env Envelope) []Envelope {
		switch env.Method {
		case MethodInitialize:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}}
		case MethodInitialized:
			return nil
		case MethodModelList:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[{"id":"m","displayName":"M","defaultReasoningEffort":"low","supportedReasoningEfforts":["low",{"effort":"high"}],"isDefault":true}],"nextCursor":null}`)}}
		case MethodAccountRead:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"requiresOpenaiAuth":true,"account":{"type":"apiKey"}}`)}}
		default:
			return []Envelope{{ID: env.ID, Error: &RPCError{Code: CodeMethodNotFound, Message: env.Method}}}
		}
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Handshake(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	models, err := client.ModelList(ctx, ModelListParams{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := ParseModel(models.Data[0])
	if err != nil || info.ID != "m" || len(info.Efforts) != 2 {
		t.Fatalf("%+v %v", info, err)
	}
	account, err := client.AccountRead(ctx)
	if err != nil || !Authorized(account) {
		t.Fatal(account, err)
	}
}

func TestCallCancelDropsLateResponse(t *testing.T) {
	started := make(chan struct{})
	client := StartLoopback(func(env Envelope) []Envelope {
		if env.Method == MethodInitialize {
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}}
		}
		if env.Method == "slow" {
			close(started)
			time.Sleep(200 * time.Millisecond)
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"ok":true}`)}}
		}
		return nil
	})
	defer client.Close()
	ctx := context.Background()
	if _, err := client.Handshake(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	slow, cancel := context.WithCancel(ctx)
	go func() {
		<-started
		cancel()
	}()
	_, err := client.CallRaw(slow, "slow", map[string]any{})
	if err == nil {
		t.Fatal("expected cancel")
	}
}

func TestOutOfOrderResponses(t *testing.T) {
	var first *Envelope
	client := StartLoopback(func(env Envelope) []Envelope {
		switch env.Method {
		case MethodInitialize:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}}
		case "hold":
			cp := env
			first = &cp
			return nil
		case "fast":
			out := []Envelope{{ID: env.ID, Result: json.RawMessage(`{"n":2}`)}}
			if first != nil {
				out = append(out, Envelope{ID: first.ID, Result: json.RawMessage(`{"n":1}`)})
			}
			return out
		default:
			return nil
		}
	})
	defer client.Close()
	ctx := context.Background()
	if _, err := client.Handshake(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		var out map[string]int
		done <- client.Call(ctx, "hold", map[string]any{}, &out)
	}()
	time.Sleep(20 * time.Millisecond)
	var fast map[string]int
	if err := client.Call(ctx, "fast", map[string]any{}, &fast); err != nil || fast["n"] != 2 {
		t.Fatal(fast, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestServerRequestAndNotify(t *testing.T) {
	client := StartLoopback(func(env Envelope) []Envelope {
		if env.Method == MethodInitialize {
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}}
		}
		if env.Method == MethodTurnStart {
			ask := IntID(42)
			return []Envelope{
				{ID: env.ID, Result: json.RawMessage(`{"turn":{"id":"t1","status":"inProgress","items":[]}}`)},
				{ID: &ask, Method: MethodItemCommandApproval, Params: json.RawMessage(`{"threadId":"th","turnId":"t1","command":"ls","itemId":"i","startedAtMs":1}`)},
				{Method: MethodAgentMessageDelta, Params: json.RawMessage(`{"threadId":"th","turnId":"t1","itemId":"m","delta":"hi"}`)},
			}
		}
		return nil
	})
	defer client.Close()
	ctx := context.Background()
	if _, err := client.Handshake(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TurnStart(ctx, TurnStartParams{ThreadID: "th", Input: UserInputs(Input{Text: "hi"})}); err != nil {
		t.Fatal(err)
	}
	gotAsk, gotDelta := false, false
	deadline := time.After(time.Second)
	for !gotAsk || !gotDelta {
		select {
		case msg := <-client.Incoming():
			if msg.Method == MethodItemCommandApproval {
				ask, ok := ParseAsk(msg)
				if !ok || ask.Command != "ls" {
					t.Fatalf("%+v", ask)
				}
				if err := client.Reply(ctx, msg.ID, ReplyBody(ask, AskAnswer{Approved: true})); err != nil {
					t.Fatal(err)
				}
				gotAsk = true
			}
			if msg.Method == MethodAgentMessageDelta {
				ev, ok := MapNotification(msg)
				if !ok || ev.Progress.Text != "hi" {
					t.Fatal(ev)
				}
				gotDelta = true
			}
		case <-deadline:
			t.Fatal("timeout")
		}
	}
}

func TestUnknownRequestReplyError(t *testing.T) {
	client := StartLoopback(func(env Envelope) []Envelope {
		if env.Method == MethodInitialize {
			id := StringID("srv")
			return []Envelope{
				{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)},
				{ID: &id, Method: "future/unknown", Params: json.RawMessage(`{}`)},
			}
		}
		return nil
	})
	defer client.Close()
	ctx := context.Background()
	if _, err := client.Handshake(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-client.Incoming():
		if KnownAskMethod(msg.Method) {
			t.Fatal(msg.Method)
		}
		if err := client.ReplyError(ctx, msg.ID, CodeMethodNotFound, "unsupported"); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestRPCErrorAndSettings(t *testing.T) {
	err := RPCError{Code: 1, Message: "nope"}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatal(err)
	}
	s := Settings{Model: "m"}.MergeOverride(Settings{Effort: "high", ApprovalPolicy: "never", Sandbox: "read-only", CollaborationMode: "plan", Cwd: "/tmp"})
	over := s.Override()
	if over.Model != "" || over.Effort != "high" || over.Cwd != "/tmp" {
		t.Fatalf("%+v", over)
	}
	params := ApplyTurnOverrides(TurnStartParams{ThreadID: "t", Input: UserInputs(Input{Text: "a"})}, s)
	if params.Effort != "high" || params.SandboxPolicy == nil || params.CollaborationMode == nil {
		t.Fatalf("%+v", params)
	}
	tp := ApplyThreadOverrides(ThreadStartParams{}, s)
	if tp.Sandbox != "read-only" {
		t.Fatal(tp)
	}
}

func TestCommandsAndProgress(t *testing.T) {
	spec, ok := LookupCommand("mcp")
	if !ok || spec.Action != ActionHint {
		t.Fatal(spec)
	}
	if _, ok := LookupCommand("nope"); ok {
		t.Fatal("unknown")
	}
	item, ok := MapItem(json.RawMessage(`{"id":"1","type":"fileChange","status":"completed","changes":[{"path":"a.go","diff":"+x"}]}`))
	if !ok || item.Kind != ProgressFileChange || item.Paths[0] != "a.go" {
		t.Fatal(item)
	}
	turns := []TurnObject{{Items: []json.RawMessage{json.RawMessage(`{"id":"u","type":"userMessage","content":[{"type":"text","text":"hi"}]}`)}}}
	if HydrateProgress(turns)[0].Text != "hi" {
		t.Fatal(HydrateProgress(turns))
	}
	if MapTurnStatus("interrupted", false) != TurnCancelled {
		t.Fatal("cancel")
	}
	if MapTurnStatus("inProgress", true) != TurnWaitingApproval {
		t.Fatal("ask")
	}
	if !Authorized(AccountResult{RequiresOpenaiAuth: false}) {
		t.Fatal("no auth required")
	}
	if Authorized(AccountResult{RequiresOpenaiAuth: true}) {
		t.Fatal("missing account")
	}
}

func TestParseAskKinds(t *testing.T) {
	id := IntID(1)
	cases := []struct {
		method string
		params string
		kind   AskKind
	}{
		{MethodItemFileApproval, `{"threadId":"t","turnId":"u","itemId":"i","startedAtMs":1,"changes":[{"path":"a","diff":"+"}]}`, AskFileChange},
		{MethodItemPermissionsApproval, `{"threadId":"t","turnId":"u","itemId":"i","startedAtMs":1,"cwd":"/","permissions":{}}`, AskPermissions},
		{MethodItemToolUserInput, `{"threadId":"t","turnId":"u","itemId":"i","isBlocking":true,"questions":[{"id":"q1","header":"pick","options":["a",{"label":"b"}]}]}`, AskQuestion},
		{MethodMCPElicitation, `{"serverName":"s","threadId":"t","requestedSchema":{"properties":{"name":{}}}}`, AskForm},
		{MethodExecCommandApproval, `{"command":"pwd"}`, AskCommand},
		{MethodApplyPatchApproval, `{"fileChanges":[{"path":"b","diff":"-"}]}`, AskFileChange},
	}
	for _, tc := range cases {
		ask, ok := ParseAsk(Message{Kind: KindRequest, ID: id, Method: tc.method, Params: json.RawMessage(tc.params)})
		if !ok || ask.Kind != tc.kind {
			t.Fatalf("%s: %+v", tc.method, ask)
		}
		_ = ReplyBody(ask, AskAnswer{Approved: true, Scope: ScopeSession, Choice: "a", Values: []string{"v"}})
		_ = ReplyBody(ask, AskAnswer{Approved: false})
	}
}

func TestCollaborationAndSandboxHelpers(t *testing.T) {
	if SandboxPolicy("read-only") == nil || SandboxPolicy("nope") != nil {
		t.Fatal("sandbox")
	}
	if CollaborationModeParams("", "m") != nil || CollaborationModeParams("plan", "") == nil {
		t.Fatal("collab")
	}
	if len(CollaborationModes()) != 2 {
		t.Fatal("modes")
	}
	raw, _ := json.Marshal(map[string]any{"id": "p", "description": "d", "allowed": true})
	info, err := ParsePermissionProfile(raw)
	if err != nil || info.Kind != "permission" {
		t.Fatal(info, err)
	}
	cfg := ConfigReadResult{Config: map[string]json.RawMessage{"model": json.RawMessage(`"x"`)}}
	if ConfigString(cfg, "model") != "x" || ConfigString(cfg, "missing") != "" {
		t.Fatal(cfg)
	}
}

func TestUserInputsEmpty(t *testing.T) {
	items := UserInputs(Input{})
	if len(items) != 1 || items[0].Type != "text" {
		t.Fatal(items)
	}
	items = UserInputs(Input{Text: "a", Mentions: []string{"f.go"}, Images: []string{"p.png"}})
	if len(items) != 3 {
		t.Fatal(items)
	}
}

func TestJSONLSkipEmptyLine(t *testing.T) {
	left, right := PipePair()
	defer left.Close()
	defer right.Close()
	go func() {
		_, _ = right.(*JSONL).writer.Write([]byte("\n{\"ok\":true}\n"))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	frame, err := left.Read(ctx)
	if err != nil || string(frame) != `{"ok":true}` {
		t.Fatalf("%q %v", frame, err)
	}
}

func TestJSONLCRLFAndClose(t *testing.T) {
	left, right := PipePair()
	defer left.Close()
	defer right.Close()
	go func() {
		_, _ = right.(*JSONL).writer.Write([]byte("{\"ok\":1}\r\n"))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	frame, err := left.Read(ctx)
	if err != nil || string(frame) != `{"ok":1}` {
		t.Fatalf("%q %v", frame, err)
	}
	tr := NewJSONL(strings.NewReader(""), nil, nil, 0)
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientRPCWrappers(t *testing.T) {
	client := StartLoopback(func(env Envelope) []Envelope {
		if env.Method == MethodInitialize {
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}}
		}
		if env.ID == nil {
			return nil
		}
		switch env.Method {
		case MethodThreadList:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[{"id":"t1","name":"n","preview":"p","cwd":"/","createdAt":1,"updatedAt":1}]}`)}}
		case MethodThreadRead:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"thread":{"id":"t1","name":"n","preview":"p","cwd":"/","ephemeral":true}}`)}}
		case MethodThreadStart, MethodThreadResume, MethodThreadFork:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"thread":{"id":"t1","name":"n"},"model":"m","cwd":"/"}`)}}
		case MethodTurnStart:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"turn":{"id":"u1","status":"inProgress"}}`)}}
		case MethodModelList:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[{"model":"only-model","displayName":"M","supportedReasoningEfforts":[{"id":"low"}]}]}`)}}
		case MethodPermissionProfileList:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[]}`)}}
		case MethodAccountRead:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"requiresOpenaiAuth":false}`)}}
		case MethodConfigRead:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"config":{"model":"x","n":1}}`)}}
		default:
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{}`)}}
		}
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Handshake(ctx, ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Initialize(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ThreadStart(ctx, ThreadStartParams{Cwd: "/tmp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ThreadResume(ctx, ThreadResumeParams{ThreadID: "t1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ThreadFork(ctx, ThreadForkParams{ThreadID: "t1"}); err != nil {
		t.Fatal(err)
	}
	if err := client.ThreadArchive(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if err := client.ThreadUnarchive(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if err := client.ThreadSetName(ctx, "t1", "n"); err != nil {
		t.Fatal(err)
	}
	page, err := client.ThreadList(ctx, ThreadListParams{})
	if err != nil || len(page.Data) != 1 {
		t.Fatal(page, err)
	}
	read, err := client.ThreadRead(ctx, ThreadReadParams{ThreadID: "t1", IncludeTurns: true})
	if err != nil || read.Thread.ID != "t1" {
		t.Fatal(read, err)
	}
	if err := client.ThreadCompact(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TurnStart(ctx, TurnStartParams{ThreadID: "t1"}); err != nil {
		t.Fatal(err)
	}
	if err := client.TurnInterrupt(ctx, "t1", "u1"); err != nil {
		t.Fatal(err)
	}
	if err := client.ReviewStart(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	models, err := client.ModelList(ctx, ModelListParams{})
	if err != nil || len(models.Data) != 1 {
		t.Fatal(models, err)
	}
	info, err := ParseModel(models.Data[0])
	if err != nil || info.ID != "only-model" || len(info.Efforts) != 1 {
		t.Fatal(info, err)
	}
	if _, err := client.PermissionProfileList(ctx, CursorListParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.AccountRead(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, err := client.ConfigRead(ctx, "/tmp")
	if err != nil || ConfigString(cfg, "n") != "" {
		t.Fatal(cfg, err)
	}
	sess := MapThread(read.Thread, true, "u1")
	if sess.ID != "t1" || !sess.Archived || sess.ActiveTurnID != "u1" {
		t.Fatal(sess)
	}
}

func TestParseModelAndAskEdges(t *testing.T) {
	if _, err := ParseModel(json.RawMessage(`{`)); err == nil {
		t.Fatal("bad model")
	}
	if _, err := ParsePermissionProfile(json.RawMessage(`{`)); err == nil {
		t.Fatal("bad profile")
	}
	if SandboxPolicy("workspace-write") == nil || SandboxPolicy("danger-full-access") == nil {
		t.Fatal("sandbox variants")
	}
	blank := Input{}
	filled := Input{Text: "a"}
	if !blank.Empty() || filled.Empty() {
		t.Fatal("empty")
	}
	_ = UserInputs(Input{Mentions: []string{""}, Images: []string{""}})
	if KnownAskMethod("nope") {
		t.Fatal("unknown ask")
	}
	if _, ok := ParseAsk(Message{Kind: KindNotification, Method: MethodItemCommandApproval}); ok {
		t.Fatal("not a request")
	}
	id := IntID(3)
	ask, ok := ParseAsk(Message{Kind: KindRequest, ID: id, Method: MethodItemCommandApproval, Params: json.RawMessage(`{"command":{"command":"pwd"}}`)})
	if !ok || ask.Command != "pwd" {
		t.Fatal(ask)
	}
	ask, ok = ParseAsk(Message{Kind: KindRequest, ID: id, Method: MethodItemFileApproval, Params: json.RawMessage(`{"grantRoot":"/tmp"}`)})
	if !ok || len(ask.Paths) != 1 {
		t.Fatal(ask)
	}
	ask, ok = ParseAsk(Message{Kind: KindRequest, ID: id, Method: MethodItemToolUserInput, Params: json.RawMessage(`{"prompt":"q","questions":[{"id":"q1","question":"pick","options":[{"id":"o1"}]}]}`)})
	if !ok || len(ask.Options) != 1 {
		t.Fatal(ask)
	}
	_ = ReplyBody(ask, AskAnswer{Approved: true, Values: []string{"v"}})
	ask, ok = ParseAsk(Message{Kind: KindRequest, ID: id, Method: MethodMCPElicitation, Params: json.RawMessage(`{"message":{"requestedSchema":{"properties":{"n":{}}}}}`)})
	if !ok || len(ask.Fields) != 1 {
		t.Fatal(ask)
	}
	_ = ReplyBody(ask, AskAnswer{Approved: true, Choice: "x"})
	_ = ReplyBody(ApprovalAsk{Method: "future"}, AskAnswer{})
	if MapTurnStatus("failed", false) != TurnFailed || MapTurnStatus("other", false) != TurnRunning {
		t.Fatal("status")
	}
}

func TestMapNotificationKinds(t *testing.T) {
	note := func(method, params string) Message {
		return Message{Kind: KindNotification, Method: method, Params: json.RawMessage(params)}
	}
	if _, ok := MapNotification(Message{Kind: KindRequest, Method: MethodError}); ok {
		t.Fatal("request")
	}
	cases := []string{
		MethodReasoningSummaryDelta,
		MethodItemStarted,
		MethodTurnStarted,
		MethodTurnCompleted,
		"item/unknown",
		"other",
	}
	for _, method := range cases {
		_, _ = MapNotification(note(method, `{"threadId":"t","turnId":"u","itemId":"i","delta":"x","text":"y","item":{"id":"i","type":"agentMessage","text":"z"}}`))
	}
	if _, ok := MapItem(nil); ok {
		t.Fatal("empty item")
	}
	if _, ok := MapItem(json.RawMessage(`{`)); ok {
		t.Fatal("bad item")
	}
	if p, ok := MapItem(json.RawMessage(`{"id":"1"}`)); ok {
		t.Fatal(p)
	}
}

func TestCallResultDecodeError(t *testing.T) {
	client := StartLoopback(func(env Envelope) []Envelope {
		if env.Method == MethodInitialize {
			return []Envelope{{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}}
		}
		return []Envelope{{ID: env.ID, Result: json.RawMessage(`"nope"`)}}
	})
	defer client.Close()
	ctx := context.Background()
	if _, err := client.Handshake(ctx, DefaultClientInfo()); err != nil {
		t.Fatal(err)
	}
	var out ThreadStartResult
	if err := client.Call(ctx, "x", map[string]any{"ch": make(chan int)}, &out); err == nil {
		t.Fatal("expected decode error")
	}
}
