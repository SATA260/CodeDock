package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"codedock/internal/agent"
	agenttools "codedock/internal/agent/tools"
	"codedock/internal/config"
	"codedock/internal/events"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
)

// fixture 聚合测试所需的 API、路由、Runtime 与清理函数。
type fixture struct {
	api     *handler.API
	router  http.Handler
	cancel  context.CancelFunc
	runtime *agent.Runtime
}

// newFixture 打开内存 SQLite、装配 Runtime 并启动 Worker。
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
	api := handler.New(client, queries, runtime, bus, defaults, config.Config{}, nil)
	return &fixture{api: api, router: testRouter(api), cancel: cancel, runtime: runtime}
}

// testRouter 注册测试用到的 HTTP 路由。
func testRouter(api *handler.API) http.Handler {
	r := chi.NewRouter()
	r.Post("/sessions", api.CreateSession)
	r.Get("/sessions", api.ListSessions)
	r.Get("/sessions/{session_id}", api.GetSession)
	r.Post("/sessions/{session_id}/runs", api.StartRun)
	r.Post("/sessions/{session_id}/messages", api.CreateMessage)
	r.Get("/sessions/{session_id}/messages", api.ListMessages)
	r.Get("/sessions/{session_id}/event-log", api.ListEvents)
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

// mustJSON 将值序列化为 JSON，失败则 panic。
func mustJSON(v any) json.RawMessage {
	body, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return body
}

// do 发送一次 HTTP 请求并返回响应记录。
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

// createSession 创建一个测试会话并返回其 ID。
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

// start 在指定会话下启动一次 Run，返回 Run ID。
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

// waitRun 轮询直到 Run 到达任一指定状态或超时。
// 若未指定状态，则等到任一终态。
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

// withFake 把模型配置切换为 fake 模型，用于测试。
func withFake(cfg pkgagent.RunConfigSnapshot, opts pkgagent.FakeOptions) *pkgagent.RunConfigSnapshot {
	cfg.Model = pkgagent.ModelConfig{Provider: "fake", Model: "fake", Options: mustJSON(opts)}
	return &cfg
}
