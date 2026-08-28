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

type DecideApprovalRequest struct {
	ApprovalID string                  `json:"approval_id"`
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

// DecideApproval 持久化裁决并恢复或结束 Run。
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

// decide 校验 pending/过期后落裁决；批准则 Continue，拒绝或过期则 failed。
func (a *API) decide(ctx context.Context, req DecideApprovalRequest) (pkgagent.Approval, error) {
	row, err := a.q(ctx).GetApproval(ctx, req.ApprovalID)
	if err != nil {
		return pkgagent.Approval{}, wrapHandlerDB(err)
	}
	approval := mapApproval(row)
	if approval.Status != pkgagent.ApprovalPending {
		return pkgagent.Approval{}, cderr.Conflict("approval already decided")
	}
	if !approval.ExpiresAt.IsZero() && util.Now().After(approval.ExpiresAt) {
		req.Status = pkgagent.ApprovalExpired
	}
	if req.Status != pkgagent.ApprovalApproved && req.Status != pkgagent.ApprovalDenied && req.Status != pkgagent.ApprovalExpired {
		return pkgagent.Approval{}, cderr.Invalid("invalid approval status")
	}
	if req.Scope != "" {
		approval.Scope = req.Scope
	}
	updated, err := a.q(ctx).UpdateApproval(ctx, sqlite.UpdateApprovalParams{
		Scope:  string(approval.Scope),
		Status: string(req.Status),
		ID:     approval.ID,
	})
	if err != nil {
		return pkgagent.Approval{}, wrapHandlerDB(err)
	}
	approval = mapApproval(updated)
	if _, err := a.runtime.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: approval.SessionID,
		RunID:     approval.RunID,
		Type:      pkgagent.EventApprovalDecided,
		Payload: pkgagent.MarshalPayload(pkgagent.ApprovalDecidedPayload{
			ApprovalID: approval.ID,
			ToolCallID: approval.ToolCallID,
			Status:     approval.Status,
			Scope:      approval.Scope,
			Reason:     req.Reason,
		}),
	}); err != nil {
		return pkgagent.Approval{}, err
	}
	a.logger().Info("approval decided", "session_id", approval.SessionID, "run_id", approval.RunID, "approval_id", approval.ID, "status", req.Status)
	if req.Status == pkgagent.ApprovalApproved {
		if err := a.runtime.Worker().Submit(ctx, approval.RunID); err != nil {
			return pkgagent.Approval{}, err
		}
		return approval, nil
	}
	if err := a.runtime.Terminate(ctx, approval.RunID, pkgagent.RunFailed, pkgagent.StopApprovalDenied, "approval_denied"); err != nil {
		return pkgagent.Approval{}, err
	}
	return approval, nil
}
