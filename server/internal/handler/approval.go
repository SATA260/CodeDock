package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

type ToolDecision struct {
	ToolCallID string                  `json:"tool_call_id"`
	Status     pkgagent.ApprovalStatus `json:"status"`
	Reason     string                  `json:"reason"`
}

type DecideApprovalRequest struct {
	ApprovalID string                  `json:"approval_id"`
	Decisions  []ToolDecision          `json:"decisions"`
	Status     pkgagent.ApprovalStatus `json:"status"`
	Scope      pkgagent.ApprovalScope  `json:"scope"`
	ActorID    string                  `json:"actor_id"`
	Reason     string                  `json:"reason"`
}

type ApprovalResponse struct {
	Approval pkgagent.Approval `json:"approval"`
}

type ListApprovalsResponse struct {
	Approvals []pkgagent.Approval `json:"approvals"`
	PageInfo
}

// ListApprovals 分页查询会话下的审批。
func (a *API) ListApprovals(w http.ResponseWriter, r *http.Request) {
	session, err := a.loadSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	page, err := ParsePageQuery(r, approvalPageDefaults)
	if err != nil {
		writeError(w, err)
		return
	}
	q := a.q(r.Context())
	total, err := q.CountSessionApprovals(r.Context(), session.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	rows, err := q.ListSessionApprovals(r.Context(), sqlite.ListSessionApprovalsParams{
		SessionID: session.ID,
		SortBy:    page.SortBy,
		SortOrder: page.SortOrder,
		Limit:     int64(page.Limit()),
		Offset:    int64(page.Offset()),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]pkgagent.Approval, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapApproval(row))
	}
	writeJSON(w, http.StatusOK, ListApprovalsResponse{Approvals: items, PageInfo: page.Info(total)})
}

// GetApproval 查询单条审批。
func (a *API) GetApproval(w http.ResponseWriter, r *http.Request) {
	row, err := a.q(r.Context()).GetApproval(r.Context(), chi.URLParam(r, "approval_id"))
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, ApprovalResponse{Approval: mapApproval(row)})
}

// DecideApproval 持久化一批裁决并恢复 Run。
func (a *API) DecideApproval(w http.ResponseWriter, r *http.Request) {
	var req DecideApprovalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.ApprovalID = chi.URLParam(r, "approval_id")
	approval, err := a.decide(r.Context(), req)
	if err != nil {
		a.requestLog(r).Error("decide approval failed", "approval_id", req.ApprovalID, "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ApprovalResponse{Approval: approval})
}

// decide 校验 pending/过期后同事务落齐裁决，提交后再 Submit 恢复。
func (a *API) decide(ctx context.Context, req DecideApprovalRequest) (pkgagent.Approval, error) {
	row, err := a.q(ctx).GetApproval(ctx, req.ApprovalID)
	if err != nil {
		return pkgagent.Approval{}, wrapHandlerDB(err)
	}
	approval := mapApproval(row)
	if approval.Status != pkgagent.ApprovalPending {
		return a.resubmitDecidedApproval(ctx, approval)
	}
	if req.Scope != "" {
		approval.Scope = req.Scope
	}

	expired := !approval.ExpiresAt.IsZero() && util.Now().After(approval.ExpiresAt)
	var approved, denied []string
	if expired {
		for i := range approval.ToolCalls {
			approval.ToolCalls[i].Status = pkgagent.ApprovalExpired
			denied = append(denied, approval.ToolCalls[i].ID)
		}
		approval.Status = pkgagent.ApprovalExpired
	} else {
		decisions, err := normalizeDecisions(req, approval.ToolCalls)
		if err != nil {
			return pkgagent.Approval{}, err
		}
		byID := make(map[string]ToolDecision, len(decisions))
		for _, item := range decisions {
			byID[item.ToolCallID] = item
		}
		allDenied := true
		for i, call := range approval.ToolCalls {
			item := byID[call.ID]
			approval.ToolCalls[i].Status = item.Status
			approval.ToolCalls[i].Reason = item.Reason
			if item.Status == pkgagent.ApprovalApproved {
				approved = append(approved, call.ID)
				allDenied = false
			} else {
				denied = append(denied, call.ID)
			}
		}
		if allDenied {
			approval.Status = pkgagent.ApprovalDenied
		} else {
			approval.Status = pkgagent.ApprovalApproved
		}
	}

	var ev pkgagent.AgentEvent
	err = a.db.WithTx(ctx, func(ctx context.Context) error {
		if err := a.runtime.RecordToolDecisions(ctx, approval.RunID, approved, denied); err != nil {
			return err
		}
		updated, err := a.q(ctx).UpdateApproval(ctx, sqlite.UpdateApprovalParams{
			Scope:     string(approval.Scope),
			Status:    string(approval.Status),
			ToolCalls: string(pkgagent.MarshalPayload(approval.ToolCalls)),
			ID:        approval.ID,
		})
		if err != nil {
			return wrapHandlerDB(err)
		}
		approval = mapApproval(updated)
		decisions := make([]pkgagent.ApprovalDecision, 0, len(approval.ToolCalls))
		for _, call := range approval.ToolCalls {
			decisions = append(decisions, pkgagent.ApprovalDecision{
				ToolCallID: call.ID,
				Status:     call.Status,
				Reason:     call.Reason,
			})
		}
		ev, err = a.runtime.AppendEvent(ctx, pkgagent.AgentEvent{
			SessionID: approval.SessionID,
			RunID:     approval.RunID,
			Type:      pkgagent.EventApprovalDecided,
			Payload: pkgagent.MarshalPayload(pkgagent.ApprovalDecidedPayload{
				ApprovalID: approval.ID,
				ToolCallID: approval.ToolCallID,
				Status:     approval.Status,
				Scope:      approval.Scope,
				Reason:     req.Reason,
				Decisions:  decisions,
				ToolCalls:  approval.ToolCalls,
			}),
		})
		return err
	})
	if err != nil {
		return pkgagent.Approval{}, err
	}
	a.runtime.Publish(ev)
	a.logger().Info("approval decided", "session_id", approval.SessionID, "run_id", approval.RunID, "approval_id", approval.ID, "status", approval.Status)
	if err := a.submitDecidedRun(ctx, approval.RunID); err != nil {
		return pkgagent.Approval{}, err
	}
	return approval, nil
}

// resubmitDecidedApproval 在审批已落库但 Run 仍停在 waiting_approval 时只补投 Worker。
func (a *API) resubmitDecidedApproval(ctx context.Context, approval pkgagent.Approval) (pkgagent.Approval, error) {
	run, err := a.runtime.GetRun(ctx, approval.RunID)
	if err != nil {
		return pkgagent.Approval{}, err
	}
	if run.Status != pkgagent.RunWaitingApproval {
		return pkgagent.Approval{}, cderr.Conflict("approval already decided")
	}
	a.logger().Info("resubmit decided approval", "session_id", approval.SessionID, "run_id", approval.RunID, "approval_id", approval.ID)
	if err := a.submitDecidedRun(ctx, approval.RunID); err != nil {
		return pkgagent.Approval{}, err
	}
	return approval, nil
}

func (a *API) submitDecidedRun(ctx context.Context, runID string) error {
	if worker := a.runtime.Worker(); worker != nil {
		return worker.Submit(ctx, runID)
	}
	return nil
}

func normalizeDecisions(req DecideApprovalRequest, calls []pkgagent.ApprovalToolCall) ([]ToolDecision, error) {
	decisions := req.Decisions
	if len(decisions) == 0 && req.Status != "" && len(calls) == 1 {
		decisions = []ToolDecision{{ToolCallID: calls[0].ID, Status: req.Status, Reason: req.Reason}}
	}
	if len(decisions) != len(calls) {
		return nil, cderr.Invalid("decisions must cover every tool call")
	}
	want := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		want[call.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decisions))
	for _, item := range decisions {
		if item.Status != pkgagent.ApprovalApproved && item.Status != pkgagent.ApprovalDenied {
			return nil, cderr.Invalid("invalid approval status")
		}
		if _, ok := want[item.ToolCallID]; !ok {
			return nil, cderr.Invalid("unknown tool_call_id")
		}
		if _, dup := seen[item.ToolCallID]; dup {
			return nil, cderr.Invalid("duplicate tool_call_id")
		}
		seen[item.ToolCallID] = struct{}{}
	}
	if len(seen) != len(want) {
		return nil, cderr.Invalid("decisions must cover every tool call")
	}
	return decisions, nil
}
