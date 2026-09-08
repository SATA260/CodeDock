package claude

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	fakeBinOnce sync.Once
	fakeBinPath string
	fakeBinErr  error
)

func fakeClaude(t *testing.T) string {
	t.Helper()
	fakeBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fake-claude-bin")
		if err != nil {
			fakeBinErr = err
			return
		}
		out := filepath.Join(dir, "claude")
		_, file, _, ok := goruntime.Caller(0)
		if !ok {
			fakeBinErr = errUnavailable
			return
		}
		src := filepath.Join(filepath.Dir(file), "testdata", "fakeclaude")
		cmd := exec.Command("go", "build", "-o", out, src)
		cmd.Dir = filepath.Dir(file)
		if body, err := cmd.CombinedOutput(); err != nil {
			fakeBinErr = err
			t.Logf("build fake: %s", body)
			return
		}
		fakeBinPath = out
	})
	if fakeBinErr != nil {
		t.Fatalf("build fake claude: %v", fakeBinErr)
	}
	return fakeBinPath
}

func testEnv(t *testing.T) {
	t.Helper()
	resetRuntime()
	t.Cleanup(resetRuntime)
	t.Setenv("CLAUDE_BIN", fakeClaude(t))
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("GIT_REPO", t.TempDir())
	t.Setenv("FAKE_CLAUDE_MODE", "")
	t.Setenv("FAKE_CLAUDE_AUTH", "1")
	t.Setenv("FAKE_CLAUDE_SLEEP", "")
	t.Setenv("FAKE_CLAUDE_LOG", "")
}

func waitIdle(t *testing.T, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		sess, err := Get(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if sess.ActiveTurnID == "" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("turn still active")
}

func waitAsk(t *testing.T, turnID string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		rt.mu.Lock()
		turn := rt.turns[turnID]
		status := TurnStatus("")
		if turn != nil {
			status = turn.Status
		}
		rt.mu.Unlock()
		if status == TurnWaitingApproval {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("not waiting approval")
}

func TestProbeMissingBinary(t *testing.T) {
	resetRuntime()
	t.Setenv("CLAUDE_BIN", filepath.Join(t.TempDir(), "nope"))
	status, err := Probe()
	if err != nil {
		t.Fatal(err)
	}
	if status.Available {
		t.Fatal("available")
	}
	if status.Hint == "" {
		t.Fatal("hint")
	}
}

func TestProbeUnauthorized(t *testing.T) {
	testEnv(t)
	t.Setenv("FAKE_CLAUDE_AUTH", "0")
	status, err := Probe()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || status.Authorized {
		t.Fatalf("%+v", status)
	}
}

func TestProbeOK(t *testing.T) {
	testEnv(t)
	status, err := Probe()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || !status.Authorized || status.Version == "" {
		t.Fatalf("%+v", status)
	}
}

func TestCatalog(t *testing.T) {
	testEnv(t)
	models, err := ListModels()
	if err != nil || len(models) == 0 {
		t.Fatalf("models %v %v", models, err)
	}
	modes, err := ListModes()
	if err != nil || len(modes) != 6 {
		t.Fatalf("modes %v %v", modes, err)
	}
	cmds := ListCommands()
	if len(cmds) < 8 {
		t.Fatalf("commands %d", len(cmds))
	}
}

func TestSessionLifecycle(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil || sess.ID == "" {
		t.Fatalf("%+v %v", sess, err)
	}
	got, err := Get(sess.ID)
	if err != nil || got.ID != sess.ID {
		t.Fatal(err)
	}
	if err := Rename(sess.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	got, err = Get(sess.ID)
	if err != nil || got.Title != "hello" {
		t.Fatalf("%+v", got)
	}
	if err := Archive(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(sess.ID, "x", Input{Text: "x"}, InputModeStart); err == nil {
		t.Fatal("archived start")
	}
}

func TestForkAndBind(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if err := BindClaudeSession(sess.ID, "11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
	if err := BindClaudeSession(sess.ID, "22222222-2222-2222-2222-222222222222"); err == nil {
		t.Fatal("second bind")
	}
	child, err := Fork(sess.ID)
	if err != nil || child.ID == "" || child.ID == sess.ID {
		t.Fatalf("%+v %v", child, err)
	}
	list, err := ListSessions()
	if err != nil || len(list) == 0 {
		t.Fatalf("%v %v", list, err)
	}
}

func TestSettingsApply(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(sess.ID, Settings{Model: "nope"}); err == nil {
		t.Fatal("bad model")
	}
	out, err := Apply(sess.ID, Settings{Model: "opus", Effort: "high", PermissionMode: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "opus" || out.Effort != "high" || out.PermissionMode != "plan" {
		t.Fatalf("%+v", out)
	}
	if len(out.Overridden) != 3 {
		t.Fatalf("overridden %v", out.Overridden)
	}
	logPath := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("FAKE_CLAUDE_LOG", logPath)
	if _, err := Start(sess.ID, "hi", Input{Text: "hi"}, InputModeStart); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
	body, _ := os.ReadFile(logPath)
	if !strings.Contains(string(body), "--model opus") || !strings.Contains(string(body), "--effort high") {
		t.Fatalf("args %s", body)
	}
}

func TestDraftAndStart(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if err := Mention(sess.ID, "a.go"); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(png, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AttachImage(sess.ID, png); err != nil {
		t.Fatal(err)
	}
	turnID, err := Start(sess.ID, "hi", Input{Text: "hi"}, InputModeStart)
	if err != nil || turnID == "" {
		t.Fatalf("%s %v", turnID, err)
	}
	waitIdle(t, sess.ID)
	items, err := Hydrate(sess.ID)
	if err != nil || items == nil {
		t.Fatal(err)
	}
	draft, err := TakeDraft(sess.ID)
	if err != nil || len(draft.Mentions) != 0 {
		t.Fatalf("%+v %v", draft, err)
	}
}

func TestQueueCancel(t *testing.T) {
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
	queued, err := Start(sess.ID, "two", Input{Text: "two"}, InputModeStart)
	if err != nil || queued == "" {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CLAUDE_MODE", "")
	if err := Cancel(turnID); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
}

func TestAskDecide(t *testing.T) {
	testEnv(t)
	t.Setenv("FAKE_CLAUDE_MODE", "ask")
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	turnID, err := Start(sess.ID, "run", Input{Text: "run"}, InputModeStart)
	if err != nil {
		t.Fatal(err)
	}
	waitAsk(t, turnID)
	if err := Decide("req-1", AskAnswer{Approved: true, Scope: DecisionOnce}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
}

func TestUnknownAsk(t *testing.T) {
	testEnv(t)
	t.Setenv("FAKE_CLAUDE_MODE", "unknown")
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(sess.ID, "x", Input{Text: "x"}, InputModeStart); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
}

func TestQuestionFormEdit(t *testing.T) {
	testEnv(t)
	for _, mode := range []string{"question", "form", "form_values", "edit", "multiedit"} {
		t.Setenv("FAKE_CLAUDE_MODE", mode)
		resetRuntime()
		sess, err := Create("local")
		if err != nil {
			t.Fatal(err)
		}
		turnID, err := Start(sess.ID, mode, Input{Text: mode}, InputModeStart)
		if err != nil {
			t.Fatal(err)
		}
		waitAsk(t, turnID)
		if err := Decide("req-1", AskAnswer{Approved: true, Choice: "a", Values: []string{"n"}}); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, sess.ID)
	}
}

func TestInvoke(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "mcp", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "model", "sonnet"); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "effort", "low"); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "plan", ""); err != nil {
		t.Fatal(err)
	}
	if err := BindClaudeSession(sess.ID, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "compact", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "review", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke(sess.ID, "branch", ""); err != nil {
		t.Fatal(err)
	}
}

func TestHydrateFixture(t *testing.T) {
	testEnv(t)
	cwd := os.Getenv("GIT_REPO")
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	id := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	dir := filepath.Join(cfg, "projects", sanitizeProject(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("testdata", "transcript.jsonl")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	sess, err := ReadSession(id)
	if err != nil || sess.Title == "" {
		t.Fatalf("%+v %v", sess, err)
	}
	items, err := Hydrate(id)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[ProgressKind]bool{}
	for _, item := range items {
		kinds[item.Kind] = true
	}
	for _, kind := range []ProgressKind{ProgressKindUser, ProgressKindText, ProgressKindReasoning, ProgressKindCommand, ProgressKindFileChange, ProgressKindPlan} {
		if !kinds[kind] {
			t.Fatalf("missing %s in %+v", kind, items)
		}
	}
}

func TestParseAndControl(t *testing.T) {
	item, ok := progressFromLine([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`))
	if !ok || item.Kind != ProgressKindText {
		t.Fatalf("%+v", item)
	}
	reqID, req, ok := parseControlRequest([]byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"pwd"}}}`))
	if !ok || reqID != "r1" {
		t.Fatal(reqID)
	}
	ask, known := askFromTool(req)
	if !known || ask.Kind != AskKindCommand {
		t.Fatalf("%+v", ask)
	}
	if id := parseResultSessionID("{\"session_id\":\"abc\"}\n"); id != "abc" {
		t.Fatal(id)
	}
	if authLoggedIn(`{"loggedIn":true}`) != true {
		t.Fatal("auth")
	}
	buf := &closeBuffer{}
	if err := writeUserMessage(buf, Input{Text: "hi", Mentions: []string{"a.go"}, Images: []string{"/nope.png"}}); err == nil {
		t.Fatal("missing image")
	}
}

func TestExpireContinueReject(t *testing.T) {
	testEnv(t)
	id, err := Require("turn", ApprovalAsk{Kind: AskKindCommand, ExternalRequestID: "exp-1"})
	if err != nil || id != "exp-1" {
		t.Fatal(err)
	}
	if err := Expire("exp-1"); err != nil {
		t.Fatal(err)
	}
	if err := Continue("turn"); err != nil {
		t.Fatal(err)
	}
	if err := RejectUnknown("missing"); err != nil {
		t.Fatal(err)
	}
	if err := Cancel(""); err != nil {
		t.Fatal(err)
	}
	if err := Interrupt("", "missing"); err != nil {
		t.Fatal(err)
	}
}

func TestStartTurnExported(t *testing.T) {
	testEnv(t)
	sess, err := Create("local")
	if err != nil {
		t.Fatal(err)
	}
	id, err := StartSession(os.Getenv("GIT_REPO"), Settings{})
	if err != nil || id == "" {
		t.Fatal(err)
	}
	if err := ResumeSession(id); err != nil {
		t.Fatal(err)
	}
	if err := BindClaudeSession(sess.ID, id); err != nil {
		t.Fatal(err)
	}
	turnID, err := StartTurn(id, Input{Text: "hi", Mentions: emptyStrings(), Images: emptyStrings()}, Settings{Cwd: os.Getenv("GIT_REPO")})
	if err != nil || turnID == "" {
		t.Fatal(err)
	}
	waitIdle(t, sess.ID)
}

type closeBuffer struct{ bytes.Buffer }

func (c *closeBuffer) Close() error { return nil }

var png1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}
