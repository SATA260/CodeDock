package handler

import (
	"context"
	"net/http"

	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
)

// InputMode 控制新用户消息如何处理当前 Active Run。
type InputMode string

const (
	InputInterrupt InputMode = "interrupt"
	InputQueue     InputMode = "queue"
)

type StartRunRequest struct {
	Content   string
	InputMode InputMode
}

type StartRunResponse struct {
	SessionID string
	RunID     string
}

type RunResponse struct {
	Run pkgagent.Run
}

type RunActionResponse struct {
	OK bool
}

// StartRun 创建或排队一次 Run。本阶段为空实现。
func (a *API) StartRun(w http.ResponseWriter, r *http.Request) {
	resp, _ := a.start(r.Context(), "", StartRunRequest{InputMode: InputQueue})
	writeJSON(w, http.StatusOK, resp)
}

// GetRun 查询单个 Run。本阶段为空实现。
func (a *API) GetRun(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.GetRun
	}
	writeJSON(w, http.StatusOK, RunResponse{})
}

// ContinueRun 从恢复点继续 Run。本阶段为空实现。
func (a *API) ContinueRun(w http.ResponseWriter, r *http.Request) {
	_ = a.continueRun(r.Context(), "")
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// RetryRun 重试 Run。本阶段为空实现。
func (a *API) RetryRun(w http.ResponseWriter, r *http.Request) {
	_ = a.continueRun(r.Context(), "")
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// CancelRun 请求停止 Run。本阶段为空实现。
func (a *API) CancelRun(w http.ResponseWriter, r *http.Request) {
	_ = a.cancelRun(r.Context(), "")
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

func (a *API) start(ctx context.Context, sessionID string, _ StartRunRequest) (StartRunResponse, error) {
	if a.queries != nil {
		_ = a.queries.InsertRun
		_ = a.queries.InsertAgentEvent
	}
	if a.bus != nil {
		a.bus.Publish(events.Event{})
	}
	if worker := a.runtime.Worker(); worker != nil {
		_ = worker.Submit(ctx, "")
	}
	return StartRunResponse{SessionID: sessionID}, nil
}

func (a *API) continueRun(ctx context.Context, runID string) error {
	if worker := a.runtime.Worker(); worker != nil {
		return worker.Submit(ctx, runID)
	}
	return nil
}

func (a *API) cancelRun(_ context.Context, runID string) error {
	if worker := a.runtime.Worker(); worker != nil {
		worker.Cancel(runID)
	}
	return nil
}
