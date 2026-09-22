package agent

import (
	"context"

	pkgagent "codedock/pkg/agent"
)

// explore 给 explore 工具用：受限只读小循环。
func (r *Runtime) explore(ctx context.Context, input pkgagent.ExploreInput, workspaceRoot, sessionID, runID string) (pkgagent.ExploreOutput, error) {
	if r == nil {
		return pkgagent.ExploreOutput{Completed: false, Error: "runtime is not initialized"}, nil
	}
	req := pkgagent.ExploreRequest{
		Input:         input,
		Model:         r.model,
		Registry:      r.tools,
		WorkspaceRoot: workspaceRoot,
		SessionID:     sessionID,
		RunID:         runID,
		BoundNames:    []string{"read", "grep", "find", "ls", "memory_search"},
	}
	if runID != "" {
		if state, hist, err := r.LoadAgentState(ctx, runID); err == nil {
			req.Model = pkgagent.FallbackModel(state.Config.SubagentModel, pkgagent.FallbackModel(state.Config.EvaluatorModel, state.Config.Model))
			scope := pkgagent.ResolvePlanScope(state.ActivePlan, hist.Messages)
			req.ActivePlan = scope.ActivePlan
			req.MentionedPlans = scope.Mentioned
			req.MaxOutputTokens = state.Config.Limits.MaxOutputTokens
		}
	}
	return pkgagent.Explore(ctx, req)
}
