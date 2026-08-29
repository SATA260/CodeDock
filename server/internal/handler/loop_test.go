package handler_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"codedock/internal/agent"
	"codedock/internal/agent/memory"
	agenttools "codedock/internal/agent/tools"
	"codedock/internal/events"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

type flakyTool struct {
	fails atomic.Int32
}

// Definition 返回会先失败再成功的测试工具定义。
func (f *flakyTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "flaky",
		Prompt:           "Fails a few times then returns ok.",
		ParametersSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Permission:       tool.Permission{},
		SupportsRetry:    true,
		Version:          "1",
	}
}

// Execute 前几次返回失败，之后返回 ok，用于验证工具重试。
func (f *flakyTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if f.fails.Add(-1) >= 0 {
		return tool.Result{CallID: input.Call.ID, Name: "flaky", Success: false, Error: "flaky"}, fmt.Errorf("flaky")
	}
	return tool.Result{CallID: input.Call.ID, Name: "flaky", Output: json.RawMessage(`{"ok":true}`), Success: true}, nil
}

type slowTool struct{}

// Definition 返回可取消的慢工具定义。
func (slowTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "slow",
		Prompt:           "Sleeps until cancelled.",
		ParametersSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Permission:       tool.Permission{},
		SupportsCancel:   true,
		SupportsRetry:    false,
		Version:          "1",
	}
}

// Execute 阻塞到取消或超时，用于验证工具执行中取消。
func (slowTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	select {
	case <-ctx.Done():
		return tool.Result{CallID: input.Call.ID, Name: "slow", Success: false, Error: ctx.Err().Error()}, ctx.Err()
	case <-time.After(2 * time.Second):
		return tool.Result{CallID: input.Call.ID, Name: "slow", Output: json.RawMessage(`{"ok":true}`), Success: true}, nil
	}
}

type fixture struct {
	api     *handler.API
	router  http.Handler
	cancel  context.CancelFunc
	queries *sqlite.Queries
	runtime *agent.Runtime
}

// newFixture 打开内存 SQLite、装配 Runtime（默认工具由 New 注册），并启动 Worker。
func newFixture(t *testing.T, extras ...tool.Tool) *fixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	client, err := db.Open(ctx, db.Config{
		Engine: db.EngineSQLite,
		DSN:    fmt.Sprintf("file:%s?mode=memory&cache=shared", name),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := db.Migrate(ctx, client.DB()); err != nil {
		t.Fatal(err)
	}

	registry := tool.NewRegistry()
	bus := events.New()
	queries := db.SQLiteQueries(client)
	runtime := agent.New(client, queries, bus, registry, nil, agenttools.Ports{})
	for _, extra := range extras {
		if err := registry.Register(extra); err != nil {
			t.Fatal(err)
		}
	}
	runtime.Start(ctx)

	defaults := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "hello"}}}),
	})
	api := handler.New(client, queries, runtime, bus, defaults, nil)
	return &fixture{api: api, router: testRouter(api), cancel: cancel, queries: queries, runtime: runtime}
}

// testRouter 注册验收测试用到的 HTTP 路由。
func testRouter(api *handler.API) http.Handler {
	r := chi.NewRouter()
	r.Post("/sessions", api.CreateSession)
	r.Get("/sessions", api.ListSessions)
	r.Get("/sessions/{session_id}", api.GetSession)
	r.Post("/sessions/{session_id}/runs", api.StartRun)
	r.Post("/sessions/{session_id}/messages", api.CreateMessage)
	r.Get("/sessions/{session_id}/messages", api.ListMessages)
	r.Get("/sessions/{session_id}/events", api.SubscribeEvents)
	r.Get("/sessions/{session_id}/usage", api.GetSessionUsage)
	r.Get("/sessions/{session_id}/approvals", api.ListApprovals)
	r.Get("/runs/{run_id}", api.GetRun)
	r.Get("/runs/{run_id}/usage", api.GetRunUsage)
	r.Post("/runs/{run_id}/continue", api.ContinueRun)
	r.Post("/runs/{run_id}/retry", api.RetryRun)
	r.Post("/runs/{run_id}/cancel", api.CancelRun)
	r.Get("/approvals/{approval_id}", api.GetApproval)
	r.Post("/approvals/{approval_id}/decision", api.DecideApproval)
	r.Get("/memories", api.ListTextMemories)
	r.Get("/memories/{scope}/{scope_id}", api.GetTextMemory)
	r.Delete("/memories/{scope}/{scope_id}", api.DeleteTextMemory)
	return r
}

func decideAll(approval pkgagent.Approval, status pkgagent.ApprovalStatus) handler.DecideApprovalRequest {
	if len(approval.ToolCalls) == 0 {
		return handler.DecideApprovalRequest{Status: status}
	}
	decisions := make([]handler.ToolDecision, 0, len(approval.ToolCalls))
	for _, call := range approval.ToolCalls {
		decisions = append(decisions, handler.ToolDecision{ToolCallID: call.ID, Status: status})
	}
	return handler.DecideApprovalRequest{Decisions: decisions}
}

// mustJSON 把值编码成 JSON，失败则 panic。
func mustJSON(v any) json.RawMessage {
	body, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return body
}

// do 向测试路由发一次 HTTP 请求。
func (f *fixture) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(mustJSON(body))
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// createSession 创建一个测试会话并返回 ID。
func (f *fixture) createSession(t *testing.T) string {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{UserID: "u1", TenantID: "t1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create session %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Session.ID
}

// start 发起一次 Run 并返回 run_id。
func (f *fixture) start(t *testing.T, sessionID string, req handler.StartRunRequest) string {
	t.Helper()
	if req.Mode == "" {
		req.Mode = pkgagent.ModeAutoApprove
	}
	rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start run %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.StartRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.RunID
}

// waitRun 轮询直到 Run 到达指定状态或终态。
func (f *fixture) waitRun(t *testing.T, runID string, want ...pkgagent.RunStatus) pkgagent.Run {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec := f.do(t, http.MethodGet, "/runs/"+runID, nil)
		if rec.Code != http.StatusOK {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		var resp handler.RunResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		for _, status := range want {
			if resp.Run.Status == status {
				return resp.Run
			}
		}
		if len(want) == 0 && pkgagent.IsTerminal(resp.Run.Status) {
			return resp.Run
		}
		time.Sleep(20 * time.Millisecond)
	}
	rec := f.do(t, http.MethodGet, "/runs/"+runID, nil)
	t.Fatalf("run %s did not reach %v; last=%s", runID, want, rec.Body.String())
	return pkgagent.Run{}
}

// withFake 把 fake 模型脚本写入 Run 配置快照。
func withFake(cfg pkgagent.RunConfigSnapshot, opts pkgagent.FakeOptions) *pkgagent.RunConfigSnapshot {
	cfg.Model = pkgagent.ModelConfig{Provider: "fake", Model: "fake", Options: mustJSON(opts)}
	return &cfg
}

// TestPlainTextRun 验收纯文本 Run：完整 Run / Turn / 助手消息 / usage。
func TestPlainTextRun(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hi",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "hello"}}}),
	})
	run := f.waitRun(t, runID, pkgagent.RunCompleted)
	if run.StopReason == nil || *run.StopReason != pkgagent.StopCompleted {
		t.Fatalf("stop reason = %v", run.StopReason)
	}
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/messages", nil)
	var msgs handler.ListMessagesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &msgs)
	if len(msgs.Messages) < 2 {
		t.Fatalf("messages = %+v", msgs.Messages)
	}
	if msgs.AsOfEventSeq == 0 {
		t.Fatal("as_of_event_seq should be set")
	}
	usage := f.do(t, http.MethodGet, "/runs/"+runID+"/usage", nil)
	if usage.Code != http.StatusOK {
		t.Fatalf("usage status %d %s", usage.Code, usage.Body.String())
	}
	var usageResp handler.UsageResponse
	if err := json.Unmarshal(usage.Body.Bytes(), &usageResp); err != nil {
		t.Fatalf("usage json %s: %v", usage.Body.String(), err)
	}
	if len(usageResp.Records) == 0 {
		t.Fatalf("expected usage records, body=%s", usage.Body.String())
	}
}

// TestSerialAndParallelTools 验收多 Tool Call 的串行与并行回填。
func TestSerialAndParallelTools(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	cfg.ToolExecutionMode = tool.ExecutionSerial
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "tools",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{Text: "calling", ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}, {Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Text: "done"},
		}}),
	})
	if got := f.waitRun(t, runID, pkgagent.RunCompleted); got.Status != pkgagent.RunCompleted {
		t.Fatalf("serial status = %s", got.Status)
	}

	cfg.ToolExecutionMode = tool.ExecutionParallel
	cfg.Limits.MaxParallelTools = 2
	cfg.ToolFailurePolicy = tool.FailureCollectAll
	runID = f.start(t, sessionID, handler.StartRunRequest{
		Content:   "parallel",
		InputMode: handler.InputQueue,
		Mode:      pkgagent.ModeAutoApprove,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}, {Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Text: "parallel done"},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
}

// TestApprovalPauseAndResume 验收审批暂停后从 checkpoint 继续，不重跑已完成工具。
func TestApprovalPauseAndResume(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "need ping",
		Mode:    pkgagent.ModeAskForApproval,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Text: "approved"},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunWaitingApproval)
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil)
	var listed handler.ListApprovalsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Approvals) == 0 {
		t.Fatal("expected approval")
	}
	dec := f.do(t, http.MethodPost, "/approvals/"+listed.Approvals[0].ID+"/decision", decideAll(listed.Approvals[0], pkgagent.ApprovalApproved))
	if dec.Code != http.StatusOK {
		t.Fatalf("decide %d %s", dec.Code, dec.Body.String())
	}
	f.waitRun(t, runID, pkgagent.RunCompleted)
}

// TestApprovalBatchTwoPings 验收一轮两个 ping 合成一条审批，一次提交两条批准后完成。
func TestApprovalBatchTwoPings(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "need pings",
		Mode:    pkgagent.ModeAskForApproval,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}, {Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Text: "done"},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunWaitingApproval)
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil)
	var listed handler.ListApprovalsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Approvals) != 1 || len(listed.Approvals[0].ToolCalls) != 2 {
		t.Fatalf("approvals=%+v", listed.Approvals)
	}
	dec := f.do(t, http.MethodPost, "/approvals/"+listed.Approvals[0].ID+"/decision", decideAll(listed.Approvals[0], pkgagent.ApprovalApproved))
	if dec.Code != http.StatusOK {
		t.Fatalf("decide %d %s", dec.Code, dec.Body.String())
	}
	f.waitRun(t, runID, pkgagent.RunCompleted)
}

// TestApprovalPartialDecisionsRejected 验收未交齐全部裁决时不流转。
func TestApprovalPartialDecisionsRejected(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "need pings",
		Mode:    pkgagent.ModeAskForApproval,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}, {Name: "ping", Arguments: json.RawMessage(`{}`)}}},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunWaitingApproval)
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil)
	var listed handler.ListApprovalsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	partial := handler.DecideApprovalRequest{Decisions: []handler.ToolDecision{{
		ToolCallID: listed.Approvals[0].ToolCalls[0].ID,
		Status:     pkgagent.ApprovalApproved,
	}}}
	dec := f.do(t, http.MethodPost, "/approvals/"+listed.Approvals[0].ID+"/decision", partial)
	if dec.Code != http.StatusBadRequest {
		t.Fatalf("decide %d %s", dec.Code, dec.Body.String())
	}
	if got := f.waitRun(t, runID, pkgagent.RunWaitingApproval); got.Status != pkgagent.RunWaitingApproval {
		t.Fatalf("status = %s", got.Status)
	}
}

// TestPlanModeAllowsMemoryWrite 验收 plan 覆盖 memory 时可以调用 memory_write。
func TestPlanModeAllowsMemoryWrite(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModePlan, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "write memory",
		Mode:    pkgagent.ModePlan,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "memory_write", Arguments: json.RawMessage(`{"scope":"user","name":"index","content":"x"}`)}}},
			{Text: "saved"},
		}}),
	})
	if got := f.waitRun(t, runID, pkgagent.RunCompleted); got.Status != pkgagent.RunCompleted {
		t.Fatalf("status = %s", got.Status)
	}
}

// TestApprovalDeniedContinuesRun 验收拒绝后把失败结果喂回模型，不打死 Run。
func TestApprovalDeniedContinuesRun(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "need ping",
		Mode:    pkgagent.ModeAskForApproval,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Text: "denied"},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunWaitingApproval)
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil)
	var listed handler.ListApprovalsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	dec := f.do(t, http.MethodPost, "/approvals/"+listed.Approvals[0].ID+"/decision", decideAll(listed.Approvals[0], pkgagent.ApprovalDenied))
	if dec.Code != http.StatusOK {
		t.Fatalf("decide %d %s", dec.Code, dec.Body.String())
	}
	if got := f.waitRun(t, runID, pkgagent.RunCompleted); got.Status != pkgagent.RunCompleted {
		t.Fatalf("status = %s", got.Status)
	}
}

// TestApprovalMixedDecisions 验收一条批一条拒后 Run 继续。
func TestApprovalMixedDecisions(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "need pings",
		Mode:    pkgagent.ModeAskForApproval,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}, {Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Text: "partial"},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunWaitingApproval)
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil)
	var listed handler.ListApprovalsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Approvals[0].ToolCalls) != 2 {
		t.Fatalf("tool_calls=%+v", listed.Approvals[0].ToolCalls)
	}
	dec := f.do(t, http.MethodPost, "/approvals/"+listed.Approvals[0].ID+"/decision", handler.DecideApprovalRequest{
		Decisions: []handler.ToolDecision{
			{ToolCallID: listed.Approvals[0].ToolCalls[0].ID, Status: pkgagent.ApprovalApproved},
			{ToolCallID: listed.Approvals[0].ToolCalls[1].ID, Status: pkgagent.ApprovalDenied},
		},
	})
	if dec.Code != http.StatusOK {
		t.Fatalf("decide %d %s", dec.Code, dec.Body.String())
	}
	if got := f.waitRun(t, runID, pkgagent.RunCompleted); got.Status != pkgagent.RunCompleted {
		t.Fatalf("status = %s", got.Status)
	}
}

// TestCancelDuringStreamAndTools 验收模型流与工具执行期间取消，已落库内容保留。
func TestCancelDuringStreamAndTools(t *testing.T) {
	f := newFixture(t, slowTool{})
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hang",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Hang: true}),
	})
	time.Sleep(50 * time.Millisecond)
	if rec := f.do(t, http.MethodPost, "/runs/"+runID+"/cancel", nil); rec.Code != http.StatusOK {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body.String())
	}
	run := f.waitRun(t, runID, pkgagent.RunCancelled)
	if run.Status != pkgagent.RunCancelled {
		t.Fatalf("status = %s", run.Status)
	}

	runID = f.start(t, sessionID, handler.StartRunRequest{
		Content: "slow tool",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "slow", Arguments: json.RawMessage(`{}`)}}},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunExecutingTools)
	_ = f.do(t, http.MethodPost, "/runs/"+runID+"/cancel", nil)
	f.waitRun(t, runID, pkgagent.RunCancelled)
}

// TestSSEReplayAndLive 验收 SSE 首次连接与按 afterSeq 回放，不丢不重。
func TestSSEReplayAndLive(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.router.ServeHTTP(rec, req)
	}()
	time.Sleep(30 * time.Millisecond)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "sse",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "streamed"}}}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if !strings.Contains(rec.Body.String(), "run.created") || !strings.Contains(rec.Body.String(), "assistant.delta") {
		t.Fatalf("sse body = %s", rec.Body.String())
	}

	replay := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/events?after=0", nil)
	replayCtx, replayCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer replayCancel()
	replay = replay.WithContext(replayCtx)
	replayRec := httptest.NewRecorder()
	f.router.ServeHTTP(replayRec, replay)
	seen := map[string]int{}
	scanner := bufio.NewScanner(strings.NewReader(replayRec.Body.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			seen[strings.TrimPrefix(line, "id: ")]++
		}
	}
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("duplicate sse id %s count %d", id, n)
		}
	}
}

// TestCompactionCheckpoint 验收超预算后写压缩 checkpoint，后续只装摘要之后的消息。
func TestCompactionCheckpoint(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	cfg.Limits.MaxInputTokens = 2
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: strings.Repeat("context ", 40),
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(cfg, pkgagent.FakeOptions{
			Turns:          []pkgagent.FakeTurn{{Text: "compacted reply"}},
			CompactSummary: "earlier chat summary",
		}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/usage", nil)
	var usage handler.UsageResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &usage)
	found := false
	for _, item := range usage.Records {
		if item.UsageType == "compaction" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected compaction usage: %+v", usage.Records)
	}
}

// TestRetries 验收 Context / Model / Tool 三类重试，达上限停止。
func TestRetries(t *testing.T) {
	flaky := &flakyTool{}
	flaky.fails.Store(2)
	f := newFixture(t, flaky)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	cfg.RetryPolicy.Model.MaxAttempts = 3
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "retry model",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(cfg, pkgagent.FakeOptions{
			FailTimes: 2,
			Turns:     []pkgagent.FakeTurn{{Text: "recovered"}},
		}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)

	runID = f.start(t, sessionID, handler.StartRunRequest{
		Content:   "retry tool",
		InputMode: handler.InputQueue,
		Mode:      pkgagent.ModeAutoApprove,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "flaky", Arguments: json.RawMessage(`{}`)}}},
			{Text: "tool recovered"},
		}}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
}

// TestLimits 验收墙钟、Turn、Token、工具次数超限后以明确 StopReason 结束。
func TestLimits(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	cfg.Limits.MaxTurns = 1
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "max turns",
		Mode:    pkgagent.ModeAutoApprove,
		Config: withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{
			{ToolCalls: []pkgagent.FakeToolCall{{Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Text: "should not run"},
		}}),
	})
	run := f.waitRun(t, runID, pkgagent.RunFailed)
	if run.StopReason == nil || *run.StopReason != pkgagent.StopMaxTurns {
		t.Fatalf("expected max_turns, got %v", run.StopReason)
	}

	cfg = pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	cfg.Limits.MaxWallTime = 30 * time.Millisecond
	runID = f.start(t, sessionID, handler.StartRunRequest{
		Content:   "timeout",
		InputMode: handler.InputQueue,
		Mode:      pkgagent.ModeAutoApprove,
		Config:    withFake(cfg, pkgagent.FakeOptions{Hang: true}),
	})
	run = f.waitRun(t, runID, pkgagent.RunFailed, pkgagent.RunCancelled)
	if run.StopReason == nil || (*run.StopReason != pkgagent.StopTimeout && *run.StopReason != pkgagent.StopCancelled) {
		t.Fatalf("expected timeout, got %v %s", run.StopReason, run.Status)
	}
}

// TestRecoverQueuedRun 验收进程重启后可恢复进行中 Run，waiting_approval 不自动恢复。
func TestRecoverQueuedRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client, err := db.Open(ctx, db.Config{Engine: db.EngineSQLite, DSN: "file:recover?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := db.Migrate(ctx, client.DB()); err != nil {
		t.Fatal(err)
	}
	queries := db.SQLiteQueries(client)
	bus := events.New()
	registry := tool.NewRegistry()
	defaults := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "recovered"}}}),
	})
	runtime := agent.New(client, queries, bus, registry, nil, agenttools.Ports{})
	api := handler.New(client, queries, runtime, bus, defaults, nil)
	f := &fixture{api: api, router: testRouter(api), cancel: cancel}
	sessionID := f.createSession(t)
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "recover me",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  &defaults,
	})
	time.Sleep(30 * time.Millisecond)
	runtime.Start(ctx)
	f.waitRun(t, runID, pkgagent.RunCompleted)
}

// TestSingleActiveRunQueueAndInterrupt 验收同一 Session 的 interrupt 与 queue。
func TestSingleActiveRunQueueAndInterrupt(t *testing.T) {
	f := newFixture(t)
	sessionID := f.createSession(t)
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	first := f.start(t, sessionID, handler.StartRunRequest{
		Content: "first",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Hang: true}),
	})
	queued := f.start(t, sessionID, handler.StartRunRequest{
		Content:   "queued",
		InputMode: handler.InputQueue,
		Mode:      pkgagent.ModeAutoApprove,
		Config:    withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "queued ok"}}}),
	})
	session := f.do(t, http.MethodGet, "/sessions/"+sessionID, nil)
	var sess handler.SessionResponse
	_ = json.Unmarshal(session.Body.Bytes(), &sess)
	if sess.Session.ActiveRunID == nil || *sess.Session.ActiveRunID != first {
		t.Fatalf("active run = %v, want %s", sess.Session.ActiveRunID, first)
	}
	_ = f.do(t, http.MethodPost, "/runs/"+first+"/cancel", nil)
	f.waitRun(t, first, pkgagent.RunCancelled)
	f.waitRun(t, queued, pkgagent.RunCompleted)

	hanging := f.start(t, sessionID, handler.StartRunRequest{
		Content: "interrupt me",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Hang: true}),
	})
	time.Sleep(30 * time.Millisecond)
	next := f.start(t, sessionID, handler.StartRunRequest{
		Content:   "new input",
		InputMode: handler.InputInterrupt,
		Mode:      pkgagent.ModeAutoApprove,
		Config:    withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "interrupted"}}}),
	})
	f.waitRun(t, hanging, pkgagent.RunCancelled)
	f.waitRun(t, next, pkgagent.RunCompleted)
	session = f.do(t, http.MethodGet, "/sessions/"+sessionID, nil)
	_ = json.Unmarshal(session.Body.Bytes(), &sess)
	if sess.Session.ActiveRunID != nil && *sess.Session.ActiveRunID == hanging {
		t.Fatal("interrupted run should not remain active")
	}
}

func TestMemoryHTTP(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := memory.Upsert(ctx, f.queries, memory.TextMemory{Scope: memory.ScopeUser, ScopeID: "u1", Name: memory.NameIndex, Content: "user index"}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Upsert(ctx, f.queries, memory.TextMemory{Scope: memory.ScopeUser, ScopeID: "u1", Name: "debugging", Content: "topic"}); err != nil {
		t.Fatal(err)
	}
	rec := f.do(t, http.MethodGet, "/memories?user_id=u1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	var listed handler.ListTextMemoriesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Items) != 2 {
		t.Fatalf("list items %+v", listed.Items)
	}
	rec = f.do(t, http.MethodGet, "/memories/user/u1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get index %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodGet, "/memories/user/u1?name=debugging", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get topic %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodDelete, "/memories/user/u1?name=debugging", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete topic %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, http.MethodDelete, "/memories/user/u1?all=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete all %d %s", rec.Code, rec.Body.String())
	}
	listed = handler.ListTextMemoriesResponse{}
	rec = f.do(t, http.MethodGet, "/memories?user_id=u1", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Items) != 0 {
		t.Fatalf("expected empty list %+v", listed.Items)
	}
}

func TestMemoryLoopIndexAndFreeze(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sessionID := f.createSession(t)
	if _, err := memory.Upsert(ctx, f.queries, memory.TextMemory{Scope: memory.ScopeUser, ScopeID: "u1", Name: memory.NameIndex, Content: "v1 pointers"}); err != nil {
		t.Fatal(err)
	}
	cfg := pkgagent.DefaultRunConfig(pkgagent.ModeAutoApprove, pkgagent.ModelConfig{})
	runID := f.start(t, sessionID, handler.StartRunRequest{
		Content: "hi memory",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "hello"}}}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	user, _, ok := f.runtime.FrozenMemoryIndexes(sessionID)
	if !ok || user != "v1 pointers" {
		t.Fatalf("frozen user %q ok=%v", user, ok)
	}
	if _, err := memory.Upsert(ctx, f.queries, memory.TextMemory{Scope: memory.ScopeUser, ScopeID: "u1", Name: memory.NameIndex, Content: "v2 pointers"}); err != nil {
		t.Fatal(err)
	}
	runID = f.start(t, sessionID, handler.StartRunRequest{
		Content: "second",
		Mode:    pkgagent.ModeAutoApprove,
		Config:  withFake(cfg, pkgagent.FakeOptions{Turns: []pkgagent.FakeTurn{{Text: "again"}}}),
	})
	f.waitRun(t, runID, pkgagent.RunCompleted)
	user, _, ok = f.runtime.FrozenMemoryIndexes(sessionID)
	if !ok || user != "v1 pointers" {
		t.Fatalf("freeze should stay v1, got %q", user)
	}
	hits, err := memory.SearchMessages(ctx, f.queries, memory.Search{WorkspaceID: "default", Query: "hi memory"})
	if err != nil || len(hits) == 0 {
		t.Fatalf("expected indexed user message: %v %+v", err, hits)
	}
}

func TestMemoryIndexBackgroundCompact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.runtime.SetModel(pkgagent.ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options:  mustJSON(pkgagent.FakeOptions{IndexCompactSummary: "short index"}),
	})
	over := strings.Repeat("line\n", memory.IndexMaxLines) + "overflow"
	item, err := memory.Upsert(ctx, f.queries, memory.TextMemory{Scope: memory.ScopeUser, ScopeID: "u1", Name: memory.NameIndex, Content: over})
	if err != nil || !item.OverBudget {
		t.Fatalf("seed %+v err=%v", item, err)
	}
	_ = f.createSession(t)
	f.runtime.EnqueueIndexCompact(memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "u1", Kind: memory.KindIndex, Name: memory.NameIndex})
	f.runtime.WaitIndexCompact()
	got, err := memory.Get(ctx, f.queries, memory.TextMemoryKey{Scope: memory.ScopeUser, ScopeID: "u1", Name: memory.NameIndex})
	if err != nil {
		t.Fatal(err)
	}
	if got.OverBudget || got.Content != "short index" {
		t.Fatalf("compacted %+v", got)
	}
}
