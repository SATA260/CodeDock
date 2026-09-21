package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codedock/internal/config"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

const promptC01 = "在工作区创建 hello.txt，内容仅一行 hi。必须用工具写盘，不要只口头描述。"

// requireLiveModel 加载仓库根 .env；没有真实模型则跳过整段编码验收。
func requireLiveModel(t *testing.T) pkgagent.ModelConfig {
	t.Helper()
	if err := config.LoadDotEnv(); err != nil {
		t.Fatalf("load .env: %v", err)
	}
	cfg := config.Load()
	if strings.TrimSpace(cfg.LLMAPIKey) == "" || strings.EqualFold(cfg.LLMProvider, "fake") || strings.TrimSpace(cfg.LLMProvider) == "" {
		t.Skip("coding loop tests require a live LLM in .env (LLM_API_KEY and LLM_PROVIDER)")
	}
	opts, err := json.Marshal(map[string]string{
		"api_key":  cfg.LLMAPIKey,
		"base_url": cfg.LLMBaseURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkgagent.ModelConfig{
		Provider: cfg.LLMProvider,
		Model:    cfg.LLMModel,
		Options:  opts,
	}
}

// liveModelFromEnv 组装主模型与复审回落，不把 Key 打进日志。
func liveModelFromEnv() (pkgagent.ModelConfig, pkgagent.ModelConfig, config.Config) {
	cfg := config.Load()
	opts, _ := json.Marshal(map[string]string{
		"api_key":  cfg.LLMAPIKey,
		"base_url": cfg.LLMBaseURL,
	})
	model := pkgagent.ModelConfig{Provider: cfg.LLMProvider, Model: cfg.LLMModel, Options: opts}
	eval := model
	if strings.TrimSpace(cfg.EvaluatorProvider) != "" {
		eval.Provider = cfg.EvaluatorProvider
	}
	if strings.TrimSpace(cfg.EvaluatorModel) != "" {
		eval.Model = cfg.EvaluatorModel
	}
	if strings.TrimSpace(cfg.EvaluatorAPIKey) != "" || strings.TrimSpace(cfg.EvaluatorBaseURL) != "" {
		key := cfg.EvaluatorAPIKey
		if key == "" {
			key = cfg.LLMAPIKey
		}
		base := cfg.EvaluatorBaseURL
		if base == "" {
			base = cfg.LLMBaseURL
		}
		evalOpts, _ := json.Marshal(map[string]string{"api_key": key, "base_url": base})
		eval.Options = evalOpts
	}
	return model, eval, cfg
}

// liveTimeout 返回真实模型一轮等待上限，可用 CODEDOCK_CODING_TEST_TIMEOUT 覆盖。
func liveTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("CODEDOCK_CODING_TEST_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 3 * time.Minute
}

// projectTimeout 加长项目一轮等待上限，可用 CODEDOCK_CODING_PROJECT_TIMEOUT 覆盖。
func projectTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("CODEDOCK_CODING_PROJECT_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 20 * time.Minute
}

// liveProjectRunConfig 给从零搭项目的任务加轮次和墙钟。
func liveProjectRunConfig(mode pkgagent.WorkMode, approval pkgagent.ApprovalMode) pkgagent.RunConfigSnapshot {
	cfg := liveRunConfig(mode, approval)
	cfg.Limits.MaxWallTime = 20 * time.Minute
	cfg.Limits.MaxTurns = 40
	cfg.Limits.MaxToolCalls = 80
	cfg.Limits.MaxVerifyRounds = 5
	cfg.Limits.MaxEvaluateRounds = 3
	return cfg
}

// liveRunConfig 冻结一份给真实模型用的 Run 配置。
func liveRunConfig(mode pkgagent.WorkMode, approval pkgagent.ApprovalMode) pkgagent.RunConfigSnapshot {
	model, eval, _ := liveModelFromEnv()
	cfg := pkgagent.DefaultRunConfig(mode, model)
	cfg.Approval = approval
	cfg.EvaluatorModel = eval
	cfg.Limits.MaxWallTime = 4 * time.Minute
	cfg.Limits.MaxTurns = 20
	cfg.Limits.MaxToolCalls = 32
	cfg.RetryPolicy.Context = pkgagent.RetryConfig{MaxAttempts: 3, InitialBackoff: time.Second, MaxBackoff: 8 * time.Second, Multiplier: 2}
	cfg.RetryPolicy.Model = cfg.RetryPolicy.Context
	cfg.RetryPolicy.Tool = cfg.RetryPolicy.Context
	return cfg
}

// newLiveFixture 用真实模型打开内存库（不测重启）。
func newLiveFixture(t *testing.T) *fixture {
	t.Helper()
	_ = requireLiveModel(t)
	return openFixture(t, fixtureOpen{
		dsn:      "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared",
		defaults: liveRunConfig(pkgagent.WorkAgent, pkgagent.ApprovalYolo),
		cfg:      config.Load(),
	})
}

// newLiveFileFixture 用文件型 SQLite，供停进程再挂 Runtime。
func newLiveFileFixture(t *testing.T) *fixture {
	t.Helper()
	_ = requireLiveModel(t)
	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "coding.db")
	return openFixture(t, fixtureOpen{
		dsn:      dsn,
		defaults: liveRunConfig(pkgagent.WorkAgent, pkgagent.ApprovalYolo),
		cfg:      config.Load(),
	})
}

// crash 关掉库并停 Worker，不走 RequestCancel，模拟进程被杀。
func (f *fixture) crash() {
	if f == nil {
		return
	}
	if f.client != nil && !f.closed {
		_ = f.client.Close()
		f.closed = true
	}
	if f.cancel != nil {
		f.cancel()
	}
}

// reopenLiveFixture 在同一 DSN 上挂新 Runtime，启动时不 RecoverActive。
func reopenLiveFixture(t *testing.T, dsn string) *fixture {
	t.Helper()
	_ = requireLiveModel(t)
	return openFixture(t, fixtureOpen{
		dsn:      dsn,
		defaults: liveRunConfig(pkgagent.WorkAgent, pkgagent.ApprovalYolo),
		cfg:      config.Load(),
	})
}

// initCodingWorkspace 初始化带空提交的 Git 工作区，便于快照与 restore。
func initCodingWorkspace(t *testing.T) string {
	t.Helper()
	dir := initGitRepo(t)
	gitCmd(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

// createWorkspaceSession 冻结已存在工作区并返回会话 ID。
func (f *fixture) createWorkspaceSession(t *testing.T, workspace string) string {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/sessions", handler.CreateSessionRequest{
		UserID:      "u1",
		TenantID:    "t1",
		WorkspaceID: workspace,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create workspace session %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Session.ID
}

// startLive 用真实模型启动一次 Run。
func (f *fixture) startLive(t *testing.T, sessionID, content string, mode pkgagent.WorkMode, approval pkgagent.ApprovalMode) string {
	t.Helper()
	cfg := liveRunConfig(mode, approval)
	return f.start(t, sessionID, handler.StartRunRequest{
		Content:  content,
		Mode:     mode,
		Approval: approval,
		Config:   &cfg,
	})
}

// startLiveProject 用加长限额启动真实模型 Run。
func (f *fixture) startLiveProject(t *testing.T, sessionID, content string, mode pkgagent.WorkMode, approval pkgagent.ApprovalMode) string {
	t.Helper()
	cfg := liveProjectRunConfig(mode, approval)
	return f.start(t, sessionID, handler.StartRunRequest{
		Content:  content,
		Mode:     mode,
		Approval: approval,
		Config:   &cfg,
	})
}

// waitProjectDone 等到加长任务终态；复审说不清时接受，验证失败单则失败。
func (f *fixture) waitProjectDone(t *testing.T, sessionID, runID string) pkgagent.Run {
	t.Helper()
	deadline := time.Now().Add(projectTimeout())
	for time.Now().Before(deadline) {
		run := f.getRun(t, runID)
		if pkgagent.IsTerminal(run.Status) {
			return run
		}
		if run.Status == pkgagent.RunWaitingApproval {
			handled := false
			for _, item := range f.listApprovals(t, sessionID) {
				if item.Status != pkgagent.ApprovalPending || item.RunID != runID {
					continue
				}
				switch item.Kind {
				case pkgagent.ApprovalKindEvaluate:
					decideOverride(t, f, item.ID, pkgagent.OverrideAccept)
					handled = true
				case pkgagent.ApprovalKindVerify:
					t.Fatalf("verify ticket left open: kind=%s id=%s", item.Kind, item.ID)
				default:
					t.Fatalf("unexpected approval kind=%s id=%s", item.Kind, item.ID)
				}
			}
			if !handled {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("project run %s timed out status=%s", runID, f.getRun(t, runID).Status)
	return pkgagent.Run{}
}

// decideOverride 对验证或复审单提交人工裁决。
func decideOverride(t *testing.T, f *fixture, approvalID string, action pkgagent.OverrideAction) {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/approvals/"+approvalID+"/decision", handler.DecideApprovalRequest{
		Override: action,
		ActorID:  "u1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("override %d %s", rec.Code, rec.Body.String())
	}
}

// waitAfterDeny 拒绝后等到终态或再次待批，避免模型再要工具时空等 completed。
func (f *fixture) waitAfterDeny(t *testing.T, sessionID, runID string) pkgagent.Run {
	t.Helper()
	deadline := time.Now().Add(liveTimeout())
	var last pkgagent.Run
	for time.Now().Before(deadline) {
		last = f.getRun(t, runID)
		if pkgagent.IsTerminal(last.Status) {
			return last
		}
		if last.Status == pkgagent.RunWaitingApproval {
			pending := 0
			for _, item := range f.listApprovals(t, sessionID) {
				if item.Status == pkgagent.ApprovalPending && item.RunID == runID {
					pending++
				}
			}
			if pending > 0 {
				return last
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	return last
}

// waitLiveDrainingTools 等到终态；manual 下继续批准后续工具批，验证单仍视为失败。
func (f *fixture) waitLiveDrainingTools(t *testing.T, sessionID, runID string) pkgagent.Run {
	t.Helper()
	deadline := time.Now().Add(liveTimeout())
	for time.Now().Before(deadline) {
		run := f.getRun(t, runID)
		if pkgagent.IsTerminal(run.Status) {
			return run
		}
		if run.Status == pkgagent.RunWaitingApproval {
			handled := false
			for _, item := range f.listApprovals(t, sessionID) {
				if item.Status != pkgagent.ApprovalPending || item.RunID != runID {
					continue
				}
				switch item.Kind {
				case pkgagent.ApprovalKindVerify, pkgagent.ApprovalKindEvaluate:
					t.Fatalf("verify ticket left open: kind=%s id=%s", item.Kind, item.ID)
				default:
					decideApproval(t, f, item.ID, pkgagent.ApprovalApproved)
					handled = true
				}
			}
			if handled {
				continue
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	t.Fatalf("run %s timed out status=%s", runID, f.getRun(t, runID).Status)
	return pkgagent.Run{}
}

// waitLive 等到终态或指定状态，超时按真实模型放宽。
func (f *fixture) waitLive(t *testing.T, runID string, want ...pkgagent.RunStatus) pkgagent.Run {
	t.Helper()
	return f.waitRunFor(t, runID, liveTimeout(), want...)
}

// waitLeftQueue 等到 Run 离开 queued。
func (f *fixture) waitLeftQueue(t *testing.T, runID string) pkgagent.Run {
	t.Helper()
	deadline := time.Now().Add(liveTimeout())
	for time.Now().Before(deadline) {
		run := f.getRun(t, runID)
		if run.Status != pkgagent.RunQueued {
			return run
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("run %s stayed queued", runID)
	return pkgagent.Run{}
}

// waitStatus 等到任一状态，超时返回最后一次。
func (f *fixture) waitStatus(t *testing.T, runID string, timeout time.Duration, want ...pkgagent.RunStatus) (pkgagent.Run, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last pkgagent.Run
	for time.Now().Before(deadline) {
		last = f.getRun(t, runID)
		for _, status := range want {
			if last.Status == status {
				return last, true
			}
		}
		if pkgagent.IsTerminal(last.Status) {
			return last, false
		}
		time.Sleep(40 * time.Millisecond)
	}
	return last, false
}

// getRun 读取单个 Run。
func (f *fixture) getRun(t *testing.T, runID string) pkgagent.Run {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/runs/"+runID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get run %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.RunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Run
}

// getSession 读取会话。
func (f *fixture) getSession(t *testing.T, sessionID string) pkgagent.Session {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get session %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Session
}

// listApprovals 列出会话审批。
func (f *fixture) listApprovals(t *testing.T, sessionID string) []pkgagent.Approval {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/approvals", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list approvals %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListApprovalsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Approvals
}

// waitApproval 等到至少一条指定状态的审批。
func (f *fixture) waitApproval(t *testing.T, sessionID string, status pkgagent.ApprovalStatus) pkgagent.Approval {
	t.Helper()
	deadline := time.Now().Add(liveTimeout())
	for time.Now().Before(deadline) {
		for _, item := range f.listApprovals(t, sessionID) {
			if item.Status == status {
				return item
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	t.Fatalf("no %s approval", status)
	return pkgagent.Approval{}
}

// readHarness 读 runs.harness，不经 HTTP。
func (f *fixture) readHarness(t *testing.T, runID string) pkgagent.RunHarness {
	t.Helper()
	row, err := f.queries.GetRunHarness(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	var harness pkgagent.RunHarness
	if row.Harness != "" && row.Harness != "{}" {
		if err := json.Unmarshal([]byte(row.Harness), &harness); err != nil {
			t.Fatal(err)
		}
	}
	return harness
}

// cancelRun 请求取消并等到 cancelled。
func (f *fixture) cancelRun(t *testing.T, runID string) pkgagent.Run {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/runs/"+runID+"/cancel", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body.String())
	}
	return f.waitLive(t, runID, pkgagent.RunCancelled)
}

// continueRun 调用 Continue。
func (f *fixture) continueRun(t *testing.T, runID string) {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/runs/"+runID+"/continue", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("continue %d %s", rec.Code, rec.Body.String())
	}
}

// restoreFiles 按文件粒度回滚工作区。
func (f *fixture) restoreFiles(t *testing.T, runID string) {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/runs/"+runID+"/restore", handler.RestoreRunRequest{Mode: pkgagent.RestoreFiles})
	if rec.Code != http.StatusOK {
		t.Fatalf("restore %d %s", rec.Code, rec.Body.String())
	}
}

// assertCancelled 检查取消收束。
func (f *fixture) assertCancelled(t *testing.T, sessionID, runID string) {
	t.Helper()
	run := f.getRun(t, runID)
	if run.Status != pkgagent.RunCancelled {
		t.Fatalf("status=%s want cancelled", run.Status)
	}
	if !run.CancelRequested {
		t.Fatal("cancel_requested")
	}
	if f.runtime.Worker() != nil && f.runtime.Worker().Busy(runID) {
		t.Fatal("worker still busy")
	}
	sess := f.getSession(t, sessionID)
	if sess.ActiveRunID != nil && *sess.ActiveRunID != "" {
		t.Fatalf("active run still %s", *sess.ActiveRunID)
	}
}

// assertFile 断言文件存在且去空白后匹配。
func assertFile(t *testing.T, dir, name, want string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if strings.TrimSpace(string(body)) != want {
		t.Fatalf("%s=%q want %q", name, strings.TrimSpace(string(body)), want)
	}
}

// assertNoFile 断言路径不存在。
func assertNoFile(t *testing.T, dir, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		t.Fatalf("%s should not exist", name)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// readTrim 读文件并去掉首尾空白；不存在返回空串。
func readTrim(dir, name string) string {
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

// writeWorkspaceFile 预置工作区文件。
func writeWorkspaceFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// siblingPlan 返回一份合法但与本轮无关的计划正文。
func siblingPlan() string {
	return "---\n{\"title\":\"other\",\"items\":[{\"id\":\"O1\",\"description\":\"create other.txt\",\"verify_cmd\":\"test -f other.txt\"}]}\n---\n# other\n"
}

// failingPlan 返回验收命令恒失败的计划。
func failingPlan() string {
	return "---\n{\"title\":\"fail\",\"items\":[{\"id\":\"F1\",\"description\":\"must fail\",\"verify_cmd\":\"false\"}]}\n---\n# fail\n"
}

// listCursorPlans 列出 .cursor 下 markdown。
func listCursorPlans(dir string) []string {
	entries, err := os.ReadDir(filepath.Join(dir, ".cursor"))
	if err != nil {
		return nil
	}
	var out []string
	for _, item := range entries {
		if !item.IsDir() && strings.HasSuffix(item.Name(), ".md") {
			out = append(out, item.Name())
		}
	}
	return out
}

// planHasContract 判断计划正文能抽出验收清单。
func planHasContract(body string) bool {
	contract, _, err := pkgagent.ParsePlanContract(body)
	return err == nil && len(contract.Items) > 0
}

// assistantText 拼接助手回复。
func assistantText(t *testing.T, f *fixture, sessionID string) string {
	t.Helper()
	var b strings.Builder
	for _, msg := range listMessages(t, f, sessionID, "").Messages {
		if msg.Role == pkgagent.RoleAssistant {
			b.WriteString(pkgagent.DecodeText(msg.Content))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// startCode 启动 Run 并返回状态码，用于断言 409。
func (f *fixture) startCode(t *testing.T, sessionID, content string, mode pkgagent.WorkMode, approval pkgagent.ApprovalMode) int {
	t.Helper()
	cfg := liveRunConfig(mode, approval)
	rec := f.do(t, http.MethodPost, "/sessions/"+sessionID+"/runs", handler.StartRunRequest{
		Content:  content,
		Mode:     mode,
		Approval: approval,
		Config:   &cfg,
	})
	return rec.Code
}
