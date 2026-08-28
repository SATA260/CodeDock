package handler

import (
	"context"
	"net/http"

	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
)

type DecideApprovalRequest struct {
	ApprovalID string
	Status     pkgagent.ApprovalStatus
	Scope      pkgagent.ApprovalScope
	ActorID    string
	Reason     string
}

type ApprovalResponse struct {
	Approval pkgagent.Approval
}

type ListApprovalsResponse struct {
	Approvals []pkgagent.Approval
}

// ListApprovals 查询会话下的审批。
// TODO: 按 session_id 列出审批。
func (a *API) ListApprovals(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListSessionApprovals
	}
	writeJSON(w, http.StatusOK, ListApprovalsResponse{})
}

// GetApproval 查询单条审批。
// TODO: 按 approval_id 读取审批。
func (a *API) GetApproval(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.GetApproval
	}
	writeJSON(w, http.StatusOK, ApprovalResponse{})
}

// DecideApproval 持久化裁决并恢复或结束 Run。
// TODO: 解析裁决请求并调用 decide。
func (a *API) DecideApproval(w http.ResponseWriter, r *http.Request) {
	_ = a.decide(r.Context(), DecideApprovalRequest{})
	writeJSON(w, http.StatusOK, ApprovalResponse{})
}

// TODO: 持久化审批裁决，发布事件，并按结果恢复或结束 Run。
func (a *API) decide(ctx context.Context, _ DecideApprovalRequest) error {
	if a.queries != nil {
		_ = a.queries.UpdateApproval
		_ = a.queries.InsertAgentEvent
	}
	if a.bus != nil {
		a.bus.Publish(events.Event{})
	}
	return a.continueRun(ctx, "")
}
