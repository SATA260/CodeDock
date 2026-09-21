package agent

import (
	"context"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

const autoReviewerActor = "auto_reviewer"

// ApprovalVerdict 是对一条审批的裁决输入。
type ApprovalVerdict struct {
	ApprovalID string
	Decisions  []pkgagent.ApprovalDecision
	Status     pkgagent.ApprovalStatus
	Scope      pkgagent.ApprovalScope
	ActorID    string
	Reason     string
	Override   pkgagent.OverrideAction
}

// DecideApproval 校验审批状态，将裁决与 tool.approval_decided 同事务写入，再用 RecoverRun 唤醒 Run。
func (r *Runtime) DecideApproval(ctx context.Context, req ApprovalVerdict) (pkgagent.Approval, error) {
	if r == nil || r.db == nil {
		return pkgagent.Approval{}, cderr.Invalid("runtime is not initialized")
	}
	if req.ApprovalID == "" {
		return pkgagent.Approval{}, cderr.Invalid("approval id is required")
	}
	row, err := r.q(ctx).GetApproval(ctx, req.ApprovalID)
	if err != nil {
		return pkgagent.Approval{}, wrapDB(err)
	}
	approval := mapApproval(row)
	if approval.Status != pkgagent.ApprovalPending {
		if err := r.RecoverRun(ctx, approval.RunID); err != nil {
			return pkgagent.Approval{}, err
		}
		return approval, nil
	}
	if req.Scope != "" {
		approval.Scope = req.Scope
	}

	kind := pkgagent.NormalizeApprovalKind(approval.Kind)
	if req.Override != "" && (kind == pkgagent.ApprovalKindVerify || kind == pkgagent.ApprovalKindEvaluate) {
		approval.Override = req.Override
		switch req.Override {
		case pkgagent.OverrideAccept, pkgagent.OverrideRetry:
			approval.Status = pkgagent.ApprovalApproved
		default:
			approval.Status = pkgagent.ApprovalDenied
		}
		for i := range approval.ToolCalls {
			approval.ToolCalls[i].Status = approval.Status
			if req.Reason != "" {
				approval.ToolCalls[i].Reason = req.Reason
			}
		}
		if err := r.persistOverride(ctx, approval, req); err != nil {
			return pkgagent.Approval{}, err
		}
		if err := r.RecoverRun(ctx, approval.RunID); err != nil {
			return pkgagent.Approval{}, err
		}
		return approval, nil
	}

	expired := !approval.ExpiresAt.IsZero() && util.Now().After(approval.ExpiresAt)
	if expired {
		for i := range approval.ToolCalls {
			approval.ToolCalls[i].Status = pkgagent.ApprovalExpired
		}
		approval.Status = pkgagent.ApprovalExpired
	} else {
		decisions, err := normalizeVerdict(req, approval.ToolCalls)
		if err != nil {
			return pkgagent.Approval{}, err
		}
		byID := make(map[string]pkgagent.ApprovalDecision, len(decisions))
		for _, item := range decisions {
			byID[item.ToolCallID] = item
		}
		allDenied := true
		for i, call := range approval.ToolCalls {
			item := byID[call.ID]
			approval.ToolCalls[i].Status = item.Status
			approval.ToolCalls[i].Reason = item.Reason
			if item.Status == pkgagent.ApprovalApproved {
				allDenied = false
			}
		}
		if allDenied {
			approval.Status = pkgagent.ApprovalDenied
		} else {
			approval.Status = pkgagent.ApprovalApproved
		}
	}

	var decided pkgagent.AgentEvent
	err = r.db.WithTx(ctx, func(ctx context.Context) error {
		updated, err := r.q(ctx).UpdateApproval(ctx, sqlite.UpdateApprovalParams{
			Scope:     string(approval.Scope),
			Status:    string(approval.Status),
			ToolCalls: string(pkgagent.MarshalPayload(approval.ToolCalls)),
			ID:        approval.ID,
		})
		if err != nil {
			return wrapDB(err)
		}
		approval = mapApproval(updated)
		ev, err := r.AppendFact(ctx, approval.RunID, approvalDecidedFact(approval, req.Reason))
		if err != nil {
			return err
		}
		decided = ev
		return nil
	})
	if err != nil {
		return pkgagent.Approval{}, err
	}
	r.publish(decided)
	if err := r.RecoverRun(ctx, approval.RunID); err != nil {
		return pkgagent.Approval{}, err
	}
	return approval, nil
}

// persistOverride 写入验证/复审单裁决并记下 OverrideAction。
func (r *Runtime) persistOverride(ctx context.Context, approval pkgagent.Approval, req ApprovalVerdict) error {
	var decided pkgagent.AgentEvent
	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		updated, err := r.q(ctx).UpdateApproval(ctx, sqlite.UpdateApprovalParams{
			Scope:     string(approval.Scope),
			Status:    string(approval.Status),
			ToolCalls: string(pkgagent.MarshalPayload(approval.ToolCalls)),
			ID:        approval.ID,
		})
		if err != nil {
			return wrapDB(err)
		}
		approval = mapApproval(updated)
		approval.Override = req.Override
		ev, err := r.AppendFact(ctx, approval.RunID, approvalDecidedFact(approval, req.Reason))
		if err != nil {
			return err
		}
		decided = ev
		state, _, err := r.LoadAgentState(ctx, approval.RunID)
		if err != nil {
			return err
		}
		state.OverrideAction = req.Override
		state.ApprovalKind = pkgagent.NormalizeApprovalKind(approval.Kind)
		return r.saveHarness(ctx, approval.RunID, state)
	})
	if err != nil {
		return err
	}
	r.publish(decided)
	return nil
}

func approvalDecidedFact(approval pkgagent.Approval, reason string) pkgagent.Fact {
	decisions := make([]pkgagent.ApprovalDecision, 0, len(approval.ToolCalls))
	for _, call := range approval.ToolCalls {
		decisions = append(decisions, pkgagent.ApprovalDecision{
			ToolCallID: call.ID,
			Status:     call.Status,
			Reason:     call.Reason,
		})
	}
	return pkgagent.Fact{
		Type: pkgagent.EventApprovalDecided,
		Payload: pkgagent.MarshalPayload(pkgagent.ApprovalDecidedPayload{
			ApprovalID: approval.ID,
			ToolCallID: approval.ToolCallID,
			Status:     approval.Status,
			Scope:      approval.Scope,
			Reason:     reason,
			Decisions:  decisions,
			ToolCalls:  approval.ToolCalls,
			Kind:       approval.Kind,
			Override:   approval.Override,
		}),
	}
}

func normalizeVerdict(req ApprovalVerdict, calls []pkgagent.ApprovalToolCall) ([]pkgagent.ApprovalDecision, error) {
	decisions := req.Decisions
	if len(decisions) == 0 && req.Status != "" && len(calls) > 0 {
		decisions = make([]pkgagent.ApprovalDecision, 0, len(calls))
		for _, call := range calls {
			decisions = append(decisions, pkgagent.ApprovalDecision{ToolCallID: call.ID, Status: req.Status, Reason: req.Reason})
		}
	}
	decisions = fillVerdict(decisions, calls)
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

func fillVerdict(decisions []pkgagent.ApprovalDecision, calls []pkgagent.ApprovalToolCall) []pkgagent.ApprovalDecision {
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
	out := make([]pkgagent.ApprovalDecision, 0, len(calls))
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
		out = append(out, pkgagent.ApprovalDecision{ToolCallID: call.ID, Status: status, Reason: reason})
	}
	return out
}

// autoReviewPending 用独立复审模型补裁一条待批；说不清则保持 pending 等人。
func (r *Runtime) autoReviewPending(ctx context.Context, runID, approvalID string) {
	if r == nil || approvalID == "" {
		return
	}
	row, err := r.q(ctx).GetApproval(ctx, approvalID)
	if err != nil {
		return
	}
	approval := mapApproval(row)
	if approval.Status != pkgagent.ApprovalPending {
		return
	}
	state, _, err := r.LoadAgentState(ctx, runID)
	if err != nil {
		return
	}
	result, err := pkgagent.Review(ctx, pkgagent.FallbackModel(state.Config.EvaluatorModel, state.Config.Model), approval.ToolCalls)
	if err != nil || result.Escalate || len(result.Decisions) == 0 {
		return
	}
	_, _ = r.DecideApproval(ctx, ApprovalVerdict{
		ApprovalID: approvalID,
		Decisions:  result.Decisions,
		Scope:      pkgagent.ApprovalOnce,
		ActorID:    autoReviewerActor,
		Reason:     result.Reason,
	})
}
