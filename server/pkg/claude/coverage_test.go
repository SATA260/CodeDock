package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestControlOtherAndFail(t *testing.T) {
	testEnv(t)
	t.Setenv("FAKE_CLAUDE_MODE", "other")
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(sess.ID, "x", Input{Text: "x"}, InputModeStart); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)

	t.Setenv("FAKE_CLAUDE_MODE", "fail")
	resetRuntime()
	sess, err = Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(sess.ID, "x", Input{Text: "x"}, InputModeStart); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
}

func TestQueueModeAndResume(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	queued, err := Start(sess.ID, "queued", Input{Text: "queued"}, InputModeQueue)
	if err != nil || queued == "" {
		t.Fatal(err)
	}
	turnID, err := Start(sess.ID, "first", Input{Text: "first"}, InputModeStart)
	if err != nil || turnID == "" {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
	time.Sleep(50 * time.Millisecond)
	waitIdle(t, sess.ID)
	turn2, err := Start(sess.ID, "second", Input{Text: "second"}, InputModeStart)
	if err != nil || turn2 == "" {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
}

func TestStartUnavailable(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BIN", filepath.Join(t.TempDir(), "missing"))
	if _, err := Start(sess.ID, "x", Input{Text: "x"}, InputModeStart); err == nil {
		t.Fatal("expected unavailable")
	}
}

func TestClaimConflictAndInterrupt(t *testing.T) {
	testEnv(t)
	t.Setenv("FAKE_CLAUDE_MODE", "hang")
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	turnID, err := Start(sess.ID, "one", Input{Text: "one"}, InputModeStart)
	if err != nil {
		t.Fatal(err)
	}
	if err := ClaimActiveTurn(sess.ID, "other"); err == nil {
		t.Fatal("expected conflict")
	}
	got, err := Get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := Interrupt(got.ClaudeSessionID, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CLAUDE_MODE", "")
	_ = Cancel(turnID)
	waitIdle(t, sess.ID)
}

func TestInvokeUnknownStartsTurn(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "not-a-slash", "hello"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
}

func TestApplyValidationAndCwd(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(sess.ID, Settings{PermissionMode: "nope"}); err == nil {
		t.Fatal("bad mode")
	}
	if _, err := Apply(sess.ID, Settings{Model: "haiku", Effort: "max"}); err == nil {
		t.Fatal("bad effort")
	}
	cwd := t.TempDir()
	out, err := Apply(sess.ID, Settings{Cwd: cwd, Model: "haiku", Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Cwd != cwd || out.Model != "haiku" {
		t.Fatalf("%+v", out)
	}
	if _, err := Apply("", Settings{Model: "sonnet"}); err == nil {
		t.Fatal("empty session")
	}
	base, err := Effective("")
	if err != nil || base.Model == "" {
		t.Fatalf("%+v %v", base, err)
	}
}

func TestSettingsFileAndPaths(t *testing.T) {
	testEnv(t)
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	body := `{"model":"opus","permissionMode":"plan","effortLevel":"high"}`
	if err := os.WriteFile(filepath.Join(cfg, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadSettings("x")
	if err != nil || settings.Model != "opus" || settings.PermissionMode != "plan" || settings.Effort != "high" {
		t.Fatalf("%+v %v", settings, err)
	}
	_ = os.WriteFile(filepath.Join(cfg, "settings.json"), []byte(`{"defaultMode":"auto","model":"haiku"}`), 0o644)
	settings, err = ReadSettings("")
	if err != nil || settings.PermissionMode != "auto" || settings.Model != "haiku" {
		t.Fatalf("%+v %v", settings, err)
	}
	_ = os.WriteFile(filepath.Join(cfg, "settings.json"), []byte(`{`), 0o644)
	settings, err = ReadSettings("")
	if err != nil || settings.Model != "sonnet" {
		t.Fatalf("%+v %v", settings, err)
	}
	long := strings.Repeat("a", 220)
	if name := sanitizeProject("/tmp/" + long); len(name) <= 200 {
		t.Fatalf("long name %d", len(name))
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("GIT_REPO", "")
	if dir := configDir(); dir == "" {
		t.Fatal("home config")
	}
	if cwd := defaultCwd(); cwd == "" {
		t.Fatal("cwd")
	}
}

func TestAuthAndParseMore(t *testing.T) {
	if !authLoggedIn(`{"status":"logged_in"}`) {
		t.Fatal("status")
	}
	if !authLoggedIn(`{"authenticated":"true"}`) {
		t.Fatal("authenticated")
	}
	if !authLoggedIn("logged in as user") {
		t.Fatal("text")
	}
	if authLoggedIn("{") || authLoggedIn("") || authLoggedIn(`{"loggedIn":false}`) {
		t.Fatal("false")
	}
	if wrapErr(errInvalid, "") != errInvalid {
		t.Fatal("wrap empty")
	}
	if forkTitleStem("ping (2)") != "ping" || forkTitleStem("ping") != "ping" {
		t.Fatal("stem")
	}
	if nextForkTitle("ping", []string{"ping"}) != "ping (1)" {
		t.Fatal("first")
	}
	if nextForkTitle("ping (1)", []string{"ping", "ping (1)", "ping (2)"}) != "ping (3)" {
		t.Fatal("next")
	}
	if nextForkTitle("", nil) != "fork (1)" {
		t.Fatal("empty")
	}
	if parseResultSessionID("") != "" {
		t.Fatal("empty result")
	}
	id := parseResultSessionID("not json\n{\"type\":\"result\",\"session_id\":\"abc\"}\n")
	if id != "abc" {
		t.Fatal(id)
	}
	item, ok := progressFromLine([]byte("not-json"))
	if ok {
		t.Fatalf("%+v", item)
	}
	item, ok = progressFromLine([]byte(`{"type":"user","message":"hi there"}`))
	if !ok || item.Kind != ProgressKindUser {
		t.Fatalf("%+v", item)
	}
	item, ok = progressFromLine([]byte(`{"type":"assistant","message":{"content":{"type":"text","text":"one"}}}`))
	if !ok || item.Text != "one" {
		t.Fatalf("%+v", item)
	}
	item, ok = progressFromLine([]byte(`{"type":"assistant","message":{"content":[{"type":"thinking","text":"why"}]}}`))
	if !ok || item.Kind != ProgressKindReasoning {
		t.Fatalf("%+v", item)
	}
	item, ok = progressFromLine([]byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"MultiEdit","input":{"file_path":"b.go","new_string":"z"}}]}}`))
	if !ok || item.Kind != ProgressKindFileChange || len(item.Paths) != 1 {
		t.Fatalf("%+v", item)
	}
	item, ok = progressFromLine([]byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"TodoWrite","input":{}}]}}`))
	if !ok || item.Kind != ProgressKindPlan {
		t.Fatalf("%+v", item)
	}
	item, ok = progressFromLine([]byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"OtherTool","input":{}}]}}`))
	if !ok || item.Command != "OtherTool" {
		t.Fatalf("%+v", item)
	}
	item, ok = progressFromLine([]byte(`{"type":"system","subtype":"permission_denied","result":"no"}`))
	if !ok || item.Kind != ProgressKindNotice {
		t.Fatalf("%+v", item)
	}
	if titleFromLine([]byte(`{"title":"T"}`)) != "T" {
		t.Fatal("title")
	}
	_, req, ok := parseControlRequest([]byte(`{"type":"user"}`))
	if ok {
		t.Fatalf("%+v", req)
	}
	_, _, ok = parseControlRequest([]byte(`{"type":"control_request","request_id":"x","request":"nope"}`))
	if ok {
		t.Fatal("bad request")
	}
	ask, known := askFromTool(controlRequest{ToolName: "Write", Input: []byte(`{"file_path":"a.go"}`)})
	if !known || ask.Kind != AskKindFileChange {
		t.Fatalf("%+v", ask)
	}
	ask, known = askFromTool(controlRequest{ToolName: "AskUserQuestion", Input: []byte(`{"question":"q","options":["a"]}`)})
	if !known || len(ask.Options) != 1 {
		t.Fatalf("%+v", ask)
	}
	if stringField(nil, "x") != "" || stringList(nil, "x") != nil && len(stringList(map[string]any{}, "missing")) != 0 {
		t.Fatal("empty fields")
	}
	if _, err := imagePart(filepath.Join(t.TempDir(), "no.jpg")); err == nil {
		t.Fatal("missing image")
	}
	png := filepath.Join(t.TempDir(), "a.webp")
	if err := os.WriteFile(png, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	part, err := imagePart(png)
	if err != nil || part["type"] != "image" {
		t.Fatal(err)
	}
	gif := filepath.Join(t.TempDir(), "a.gif")
	if err := os.WriteFile(gif, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := imagePart(gif); err != nil {
		t.Fatal(err)
	}
	jpg := filepath.Join(t.TempDir(), "a.jpeg")
	if err := os.WriteFile(jpg, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := imagePart(jpg); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentErrors(t *testing.T) {
	testEnv(t)
	if err := Mention("", "a.go"); err == nil {
		t.Fatal("mention session")
	}
	if err := Mention("s", ""); err == nil {
		t.Fatal("mention path")
	}
	if err := AttachImage("", "a.png"); err == nil {
		t.Fatal("image session")
	}
	if err := AttachImage("s", ""); err == nil {
		t.Fatal("image path")
	}
	if _, err := TakeDraft(""); err == nil {
		t.Fatal("draft")
	}
	if _, err := Hydrate(""); err == nil {
		t.Fatal("hydrate")
	}
	if _, err := Get(""); err == nil {
		t.Fatal("get")
	}
	if err := BindClaudeSession("", "x"); err == nil {
		t.Fatal("bind")
	}
	if err := ResumeSession(""); err == nil {
		t.Fatal("resume")
	}
	if err := Compact(""); err == nil {
		t.Fatal("compact")
	}
	if err := Review(""); err == nil {
		t.Fatal("review")
	}
	merged := mergeInput("t", Input{}, Input{Mentions: nil, Images: nil})
	if merged.Text != "t" || merged.Mentions == nil || merged.Images == nil {
		t.Fatalf("%+v", merged)
	}
}

func TestTimeoutAndLookPath(t *testing.T) {
	testEnv(t)
	t.Setenv("FAKE_CLAUDE_SLEEP", "300ms")
	t.Setenv("CLAUDE_TIMEOUT", "50ms")
	out := runClaude("", "-v")
	if out.err == nil {
		t.Fatal("expected timeout")
	}
	t.Setenv("FAKE_CLAUDE_SLEEP", "")
	t.Setenv("CLAUDE_TIMEOUT", "")
	t.Setenv("CLAUDE_BIN", "claude")
	t.Setenv("PATH", t.TempDir())
	if err := lookClaude(); err == nil {
		t.Fatal("look path")
	}
	if err := signalInterrupt(nil); err != nil {
		t.Fatal(err)
	}
	_ = claudeErr(claudeOutput{err: errUnavailable, stderr: "  ", stdout: "boom"})
	_ = claudeErr(claudeOutput{err: nil})
}

func TestStartTurnNoSessionAndWriteNil(t *testing.T) {
	testEnv(t)
	if err := writeUserMessage(nil, Input{Text: "x"}); err != nil {
		t.Fatal(err)
	}
	turnID, err := StartTurn("", Input{Text: "hi", Mentions: emptyStrings(), Images: emptyStrings()}, Settings{Cwd: os.Getenv("GIT_REPO")})
	if err != nil || turnID == "" {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		rt.mu.Lock()
		turn := rt.turns[turnID]
		status := TurnStatus("")
		if turn != nil {
			status = turn.Status
		}
		rt.mu.Unlock()
		if status == TurnCompleted || status == TurnFailed || status == TurnCancelled {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("turn")
}

func TestReplyAskWithoutWaiter(t *testing.T) {
	testEnv(t)
	if err := ReplyAsk("", AskAnswer{}); err != nil {
		t.Fatal(err)
	}
	if err := ReplyAsk("missing", AskAnswer{Approved: true}); err != nil {
		t.Fatal(err)
	}
	id, err := Require("", ApprovalAsk{Kind: AskKindCommand})
	if err != nil || id == "" {
		t.Fatal(err)
	}
	if err := Decide(id, AskAnswer{Approved: false}); err != nil {
		t.Fatal(err)
	}
}

func TestForkWithoutBind(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Fork(sess.ID); err == nil {
		t.Fatal("fork without transcript")
	}
	if _, err := ForkSession(""); err == nil {
		t.Fatal("empty fork")
	}
	id, err := StartSession("", Settings{})
	if err != nil || id == "" {
		t.Fatal(err)
	}
}

func TestListSessionsFromDisk(t *testing.T) {
	testEnv(t)
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	cwd := os.Getenv("GIT_REPO")
	dir := filepath.Join(cfg, "projects", sanitizeProject(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(`{"type":"user","message":{"content":"hello world from disk"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sess := range list {
		if sess.ID == id || sess.ClaudeSessionID == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", list)
	}
	items, err := Hydrate(id)
	if err != nil || len(items) == 0 {
		t.Fatalf("%v %v", items, err)
	}
}

func TestAddOverrideIdempotent(t *testing.T) {
	names := addOverride(nil, "model")
	names = addOverride(names, "model")
	if len(names) != 1 {
		t.Fatal(names)
	}
	if knownModel(nil, "sonnet") || knownMode(nil, "plan") || knownEffort(nil, "sonnet", "low") {
		t.Fatal("empty catalogs")
	}
}
