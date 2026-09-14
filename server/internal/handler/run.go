package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/profile"
)

type StartRunRequest struct {
	Content  string                      `json:"content"`          // 用户输入
	Mode     pkgagent.WorkMode           `json:"mode"`             // 选出哪个内置 Agent：ask / plan / agent
	Approval pkgagent.ApprovalMode       `json:"approval"`         // 流水线第三层：manual / auto / yolo
	Config   *pkgagent.RunConfigSnapshot `json:"config,omitempty"` // 测试可覆盖快照；线上通常不传
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
	run := mapRun(row)
	run.NeedsRecover = a.runNeedsRecover(r.Context(), run.ID)
	writeJSON(w, http.StatusOK, RunResponse{Run: run})
}

// ContinueRun 恢复已中断的 Run，或在审批写入后继续执行。
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

// CancelRun 请求取消 Run，并等待当前步骤结束。
func (a *API) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if err := a.runtime.RequestCancel(r.Context(), runID); err != nil {
		a.requestLog(r).Error("cancel run failed", "run_id", runID, "error", err)
		writeError(w, err)
		return
	}
	if worker := a.runtime.Worker(); worker != nil {
		worker.CancelAndWait(runID)
	}
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// start 创建 AgentState 并投递 user_input 步骤。已有活跃 Run 时返回 409。
func (a *API) start(ctx context.Context, sessionID string, req StartRunRequest) (StartRunResponse, error) {
	if sessionID == "" {
		return StartRunResponse{}, cderr.Invalid("session_id is required")
	}
	if req.Content == "" {
		return StartRunResponse{}, cderr.Invalid("content is required")
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
	if req.Approval != "" {
		config.Approval = req.Approval
	}
	if config.Mode == "" {
		config.Mode = pkgagent.WorkAgent
	}
	if config.Approval == "" {
		config.Approval = pkgagent.ApprovalManual
	}
	config.Profile = profile.For(string(config.Mode))

	if session.ActiveRunID != nil && *session.ActiveRunID != "" {
		return StartRunResponse{}, cderr.Conflict("session already has an active run")
	}

	runID, err := a.runtime.CreateAgentState(ctx, sessionID, req.Content, config.Mode, config)
	if err != nil {
		return StartRunResponse{}, err
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
	a.logger().Info("run started", "session_id", sessionID, "run_id", runID, "claimed", claimed)
	return StartRunResponse{SessionID: sessionID, RunID: runID}, nil
}
