package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
)

// InputMode 决定新输入与当前活跃 Run 的关系。
type InputMode string

const (
	InputInterrupt InputMode = "interrupt" // 取消当前 Run 并立即开启新 Run
	InputQueue     InputMode = "queue"     // 在当前 Run 结束后再执行新 Run
)

type StartRunRequest struct {
	Content   string                      `json:"content"`
	InputMode InputMode                   `json:"input_mode"`
	Mode      pkgagent.AgentMode          `json:"mode"`
	Config    *pkgagent.RunConfigSnapshot `json:"config,omitempty"`
}

type StartRunResponse struct {
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
}

type RunResponse struct {
	Run pkgagent.Run `json:"run"`
}

type RunActionResponse struct {
	OK bool `json:"ok"`
}

// StartRun 创建一次 Run 并投递第一个步骤。
func (a *API) StartRun(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	var req StartRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	resp, err := a.start(r.Context(), sessionID, req)
	if err != nil {
		a.requestLog(r).Error("start run failed", "session_id", sessionID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetRun 查询单个 Run 详情。
func (a *API) GetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	row, err := a.q(r.Context()).GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, RunResponse{Run: mapRun(row)})
}

// ContinueRun 继续执行已暂停的 Run（审批通过后）。
func (a *API) ContinueRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if err := a.runtime.RecoverRun(r.Context(), runID); err != nil {
		a.requestLog(r).Error("continue run failed", "run_id", runID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// RetryRun 重试当前 Run（与 Continue 同行为）。
func (a *API) RetryRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if err := a.runtime.RecoverRun(r.Context(), runID); err != nil {
		a.requestLog(r).Error("retry run failed", "run_id", runID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// CancelRun 请求取消 Run 并取消当前运行中的步骤。
func (a *API) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if err := a.runtime.RequestCancel(r.Context(), runID); err != nil {
		a.requestLog(r).Error("cancel run failed", "run_id", runID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// start 创建 AgentState 并投递 user_input 步骤。
// 若 input_mode 为 interrupt，则先取消当前活跃 Run 再开新 Run。
func (a *API) start(ctx context.Context, sessionID string, req StartRunRequest) (StartRunResponse, error) {
	if sessionID == "" {
		return StartRunResponse{}, cderr.Invalid("session_id is required")
	}
	if req.Content == "" {
		return StartRunResponse{}, cderr.Invalid("content is required")
	}
	if req.InputMode == "" {
		req.InputMode = InputQueue
	}

	sessionRow, err := a.q(ctx).GetSession(ctx, sessionID)
	if err != nil {
		return StartRunResponse{}, wrapHandlerDB(err)
	}
	session := mapSession(sessionRow)
	if session.Status == pkgagent.SessionArchived {
		return StartRunResponse{}, cderr.Conflict("session is archived")
	}

	config := a.defaults
	if req.Config != nil {
		config = *req.Config
	}
	if req.Mode != "" {
		config.Mode = req.Mode
	}
	if config.Mode == "" {
		config.Mode = pkgagent.ModeAskForApproval
	}

	if session.ActiveRunID != nil && req.InputMode == InputInterrupt {
		release := a.runtime.HoldDequeue(sessionID)
		defer func() {
			release()
			fresh, err := a.q(ctx).GetSession(ctx, sessionID)
			if err != nil || (fresh.ActiveRunID.Valid && fresh.ActiveRunID.String != "") {
				return
			}
			_ = a.runtime.DequeueNext(ctx, sessionID, "")
		}()
		_ = a.runtime.RequestCancel(ctx, *session.ActiveRunID)
		if worker := a.runtime.Worker(); worker != nil {
			worker.CancelAndWait(*session.ActiveRunID)
		}
	}

	runID, err := a.runtime.CreateAgentState(ctx, sessionID, req.Content, config.Mode, config)
	if err != nil {
		return StartRunResponse{}, err
	}

	if req.InputMode == InputQueue {
		fresh, err := a.q(ctx).GetSession(ctx, sessionID)
		if err != nil {
			return StartRunResponse{}, wrapHandlerDB(err)
		}
		if fresh.ActiveRunID.Valid && fresh.ActiveRunID.String != "" && fresh.ActiveRunID.String != runID {
			a.logger().Info("run queued", "session_id", sessionID, "run_id", runID, "active_run_id", fresh.ActiveRunID.String)
			return StartRunResponse{SessionID: sessionID, RunID: runID}, nil
		}
	}

	claimed, err := a.runtime.ClaimSession(ctx, sessionID, runID)
	if err != nil {
		return StartRunResponse{}, err
	}
	if claimed {
		if err := a.runtime.Enqueue(ctx, pkgagent.StepJob{
			RunID:     runID,
			StepIndex: 1,
			Phase:     pkgagent.PhaseUserInput,
		}); err != nil {
			return StartRunResponse{}, err
		}
	}
	a.logger().Info("run started", "session_id", sessionID, "run_id", runID, "input_mode", req.InputMode, "claimed", claimed)
	return StartRunResponse{SessionID: sessionID, RunID: runID}, nil
}
