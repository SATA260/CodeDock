package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

// InputMode 控制新用户消息如何处理当前 Active Run。
type InputMode string

const (
	InputInterrupt InputMode = "interrupt"
	InputQueue     InputMode = "queue"
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

// StartRun 创建或排队一次 Run。
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

// GetRun 查询单个 Run。
func (a *API) GetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	row, err := a.q(r.Context()).GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, RunResponse{Run: mapRun(row)})
}

// ContinueRun 从恢复点继续 Run。
func (a *API) ContinueRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if err := a.continueRun(r.Context(), runID); err != nil {
		a.requestLog(r).Error("continue run failed", "run_id", runID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// RetryRun 重试 Run。
func (a *API) RetryRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if err := a.continueRun(r.Context(), runID); err != nil {
		a.requestLog(r).Error("retry run failed", "run_id", runID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// CancelRun 请求停止 Run。
func (a *API) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	if err := a.cancelRun(r.Context(), runID); err != nil {
		a.requestLog(r).Error("cancel run failed", "run_id", runID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RunActionResponse{OK: true})
}

// start 写用户消息与 queued Run，再按 input_mode 领取或排队。
// interrupt 先取消当前 Run；queue 只落库；空闲或中断后 Claim 再 Submit。
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
	if session.ActiveRunID != nil && req.InputMode == InputInterrupt {
		active, err := a.runtime.GetRun(ctx, *session.ActiveRunID)
		if err != nil {
			return StartRunResponse{}, err
		}
		if !pkgagent.IsTerminal(active.Status) {
			_ = a.cancelRun(ctx, active.ID)
			if worker := a.runtime.Worker(); worker != nil {
				worker.CancelAndWait(active.ID)
			}
		}
		sessionRow, err = a.q(ctx).GetSession(ctx, sessionID)
		if err != nil {
			return StartRunResponse{}, wrapHandlerDB(err)
		}
		session = mapSession(sessionRow)
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

	var created pkgagent.AgentEvent
	var runID string
	var submit bool
	err = a.db.WithTx(ctx, func(ctx context.Context) error {
		if session.ActiveRunID != nil {
			if req.InputMode == InputQueue {
				run, ev, err := a.insertQueued(ctx, session, req.Content, config)
				if err != nil {
					return err
				}
				created = ev
				runID = run.ID
				return nil
			}
			active, err := a.q(ctx).GetRun(ctx, *session.ActiveRunID)
			if err == nil && !pkgagent.IsTerminal(mapRun(active).Status) {
				if err := a.cancelInTx(ctx, mapRun(active)); err != nil {
					return err
				}
			}
			session.ActiveRunID = nil
		}

		run, ev, err := a.insertQueued(ctx, session, req.Content, config)
		if err != nil {
			return err
		}
		current, err := a.q(ctx).GetSession(ctx, session.ID)
		if err != nil {
			return wrapHandlerDB(err)
		}
		if current.ActiveRunID.Valid {
			_ = a.q(ctx).ClearActiveRun(ctx, sqlite.ClearActiveRunParams{
				UpdatedAt:   util.FormatTime(util.Now()),
				ID:          session.ID,
				ActiveRunID: current.ActiveRunID,
			})
		}
		if _, err := a.q(ctx).ClaimActiveRun(ctx, sqlite.ClaimActiveRunParams{
			ActiveRunID: nullString(run.ID),
			UpdatedAt:   util.FormatTime(util.Now()),
			ID:          session.ID,
		}); err != nil {
			return wrapHandlerDB(err)
		}
		created = ev
		runID = run.ID
		submit = true
		return nil
	})
	if err != nil {
		return StartRunResponse{}, err
	}
	if created.EventID != "" {
		a.runtime.Publish(created)
	}
	if submit {
		if err := a.runtime.Worker().Submit(ctx, runID); err != nil {
			return StartRunResponse{}, err
		}
	}
	path := "idle"
	if req.InputMode == InputInterrupt {
		path = "interrupt"
	} else if !submit {
		path = "queue"
	}
	a.logger().Info("run started", "session_id", sessionID, "run_id", runID, "input_mode", path)
	return StartRunResponse{SessionID: sessionID, RunID: runID}, nil
}

// insertQueued 同事务写入用户消息、queued Run 和 run.created。
func (a *API) insertQueued(ctx context.Context, session pkgagent.Session, content string, config pkgagent.RunConfigSnapshot) (pkgagent.Run, pkgagent.AgentEvent, error) {
	now := util.Now()
	msgID := util.NewID()
	runID := util.NewID()
	seq, err := a.q(ctx).IncrementEventSeq(ctx, sqlite.IncrementEventSeqParams{
		UpdatedAt: util.FormatTime(now),
		ID:        session.ID,
	})
	if err != nil {
		return pkgagent.Run{}, pkgagent.AgentEvent{}, wrapHandlerDB(err)
	}
	if _, err := a.q(ctx).InsertMessage(ctx, sqlite.InsertMessageParams{
		ID:        msgID,
		SessionID: session.ID,
		RunID:     nullString(runID),
		Role:      string(pkgagent.RoleUser),
		Content:   string(pkgagent.EncodeText(content)),
		EventSeq:  seq,
		CreatedAt: util.FormatTime(now),
	}); err != nil {
		return pkgagent.Run{}, pkgagent.AgentEvent{}, wrapHandlerDB(err)
	}
	cfg, _ := json.Marshal(config)
	row, err := a.q(ctx).InsertRun(ctx, sqlite.InsertRunParams{
		ID:               runID,
		SessionID:        session.ID,
		TriggerMessageID: msgID,
		Mode:             string(config.Mode),
		Config:           string(cfg),
		Status:           string(pkgagent.RunQueued),
	})
	if err != nil {
		return pkgagent.Run{}, pkgagent.AgentEvent{}, wrapHandlerDB(err)
	}
	ev, err := a.runtime.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: session.ID,
		RunID:     runID,
		Type:      pkgagent.EventRunCreated,
		Payload: pkgagent.MarshalPayload(pkgagent.RunCreatedPayload{
			TriggerMessageID: msgID,
			Mode:             config.Mode,
			Status:           pkgagent.RunQueued,
			Config:           config,
		}),
	})
	if err != nil {
		return pkgagent.Run{}, pkgagent.AgentEvent{}, err
	}
	return mapRun(row), ev, nil
}

// cancelInTx 在当前事务内把 Run 标为 cancelled 并清 active_run_id。
func (a *API) cancelInTx(ctx context.Context, run pkgagent.Run) error {
	if pkgagent.IsTerminal(run.Status) {
		return nil
	}
	run.CancelRequested = true
	reason := pkgagent.StopCancelled
	run.StopReason = &reason
	now := util.Now()
	run.FinishedAt = &now
	status := pkgagent.RunCancelled
	if _, err := a.q(ctx).UpdateRun(ctx, sqlite.UpdateRunParams{
		Status:          string(status),
		CurrentTurnID:   nullString(deref(run.CurrentTurnID)),
		StopReason:      nullString(string(reason)),
		CancelRequested: 1,
		StartedAt:       nullTime(run.StartedAt),
		FinishedAt:      nullTime(run.FinishedAt),
		ID:              run.ID,
	}); err != nil {
		return wrapHandlerDB(err)
	}
	_ = a.q(ctx).ClearActiveRun(ctx, sqlite.ClearActiveRunParams{
		UpdatedAt:   util.FormatTime(now),
		ID:          run.SessionID,
		ActiveRunID: nullString(run.ID),
	})
	if worker := a.runtime.Worker(); worker != nil {
		worker.Cancel(run.ID)
	}
	_, err := a.runtime.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		Type:      pkgagent.EventRunCancelled,
		Payload:   pkgagent.MarshalPayload(pkgagent.RunTerminalPayload{Status: status, StopReason: &reason}),
	})
	return err
}

// continueRun 把可恢复 Run 重新交给 Worker；未裁定的 waiting_approval 拒绝，审完待领取的允许补投。
func (a *API) continueRun(ctx context.Context, runID string) error {
	run, err := a.runtime.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status == pkgagent.RunWaitingApproval {
		decided, err := a.runtime.HasRecordedToolDecisions(ctx, runID)
		if err != nil {
			return err
		}
		if !decided {
			return cderr.Conflict("run is waiting for approval")
		}
	}
	if pkgagent.IsTerminal(run.Status) && run.Status != pkgagent.RunFailed && run.Status != pkgagent.RunCancelled {
		return cderr.Conflict("run is already complete")
	}
	a.logger().Info("continue run", "session_id", run.SessionID, "run_id", runID, "status", run.Status)
	return a.runtime.Worker().Submit(ctx, runID)
}

// cancelRun 标记 cancel_requested；queued 直接终态并领取下一条，其余交给 Worker。
func (a *API) cancelRun(ctx context.Context, runID string) error {
	run, err := a.runtime.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if pkgagent.IsTerminal(run.Status) {
		return nil
	}
	a.logger().Info("cancel run", "session_id", run.SessionID, "run_id", runID, "status", run.Status)
	run.CancelRequested = true
	if err := a.db.WithTx(ctx, func(ctx context.Context) error {
		_, err := a.q(ctx).UpdateRun(ctx, sqlite.UpdateRunParams{
			Status:          string(run.Status),
			CurrentTurnID:   nullString(deref(run.CurrentTurnID)),
			StopReason:      nullString(string(pkgagent.StopCancelled)),
			CancelRequested: 1,
			StartedAt:       nullTime(run.StartedAt),
			FinishedAt:      nullTime(run.FinishedAt),
			ID:              run.ID,
		})
		return wrapHandlerDB(err)
	}); err != nil {
		return err
	}
	if run.Status == pkgagent.RunQueued {
		if err := a.runtime.Terminate(ctx, run.ID, pkgagent.RunCancelled, pkgagent.StopCancelled, "cancelled"); err != nil {
			return err
		}
		return a.runtime.TryDequeue(ctx, run.SessionID, run.ID)
	}
	if worker := a.runtime.Worker(); worker != nil {
		worker.Cancel(run.ID)
	}
	return nil
}

// nullTime 把时间指针格式化成可空字符串列。
func nullTime(value *time.Time) sql.NullString {
	if value == nil || value.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: util.FormatTime(*value), Valid: true}
}
