package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"codedock/internal/agent"
	cderr "codedock/internal/errors"
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
	ActorID    string                   `json:"actor_id"`
	Reason     string                   `json:"reason"`
	Override   pkgagent.OverrideAction `json:"override,omitempty"`
}

type ApprovalResponse struct {
	Approval pkgagent.Approval `json:"approval"`
}

type ListApprovalsResponse struct {
	Approvals []pkgagent.Approval `json:"approvals"`
	PageInfo
}

// ListApprovals 分页查询会话下的全部审批。
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

// GetApproval 查询单条审批详情。
func (a *API) GetApproval(w http.ResponseWriter, r *http.Request) {
	row, err := a.q(r.Context()).GetApproval(r.Context(), chi.URLParam(r, "approval_id"))
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, ApprovalResponse{Approval: mapApproval(row)})
}

// DecideApproval 提交对审批的裁决，并恢复对应 Run 的执行。
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

// decide 把 HTTP 裁决交给 Runtime。
func (a *API) decide(ctx context.Context, req DecideApprovalRequest) (pkgagent.Approval, error) {
	decisions := make([]pkgagent.ApprovalDecision, 0, len(req.Decisions))
	for _, item := range req.Decisions {
		decisions = append(decisions, pkgagent.ApprovalDecision{
			ToolCallID: item.ToolCallID,
			Status:     item.Status,
			Reason:     item.Reason,
		})
	}
	approval, err := a.runtime.DecideApproval(ctx, agent.ApprovalVerdict{
		ApprovalID: req.ApprovalID,
		Decisions:  decisions,
		Status:     req.Status,
		Scope:      req.Scope,
		ActorID:    req.ActorID,
		Reason:     req.Reason,
		Override:   req.Override,
	})
	if err != nil {
		return pkgagent.Approval{}, err
	}
	a.logger().Info("approval decided", "session_id", approval.SessionID, "run_id", approval.RunID, "approval_id", approval.ID, "status", approval.Status)
	return approval, nil
}

// normalizeDecisions 校验裁决状态合法；整单同一裁定时补齐未列出的 tool_call。
func normalizeDecisions(req DecideApprovalRequest, calls []pkgagent.ApprovalToolCall) ([]ToolDecision, error) {
	decisions := req.Decisions
	if len(decisions) == 0 && req.Status != "" && len(calls) > 0 {
		decisions = make([]ToolDecision, 0, len(calls))
		for _, call := range calls {
			decisions = append(decisions, ToolDecision{ToolCallID: call.ID, Status: req.Status, Reason: req.Reason})
		}
	}
	decisions = fillBatchDecisions(decisions, calls)
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

// fillBatchDecisions 在整单同一裁决时，把未点到的 tool_call 补成相同状态。
func fillBatchDecisions(decisions []ToolDecision, calls []pkgagent.ApprovalToolCall) []ToolDecision {
	if len(decisions) == 0 || len(calls) == 0 || len(decisions) == len(calls) {
		return decisions
	}
	status := decisions[0].Status
	reason := decisions[0].Reason
	for _, item := range decisions[1:] {
		if item.Status != status {
			return decisions
		}
	}
	seen := make(map[string]struct{}, len(decisions))
	out := make([]ToolDecision, 0, len(calls))
	for _, item := range decisions {
		if item.ToolCallID == "" {
			continue
		}
		if _, dup := seen[item.ToolCallID]; dup {
			continue
		}
		seen[item.ToolCallID] = struct{}{}
		out = append(out, item)
	}
	for _, call := range calls {
		if _, ok := seen[call.ID]; ok {
			continue
		}
		out = append(out, ToolDecision{ToolCallID: call.ID, Status: status, Reason: reason})
	}
	return out
}
