package codex

import (
	"context"
	"strings"

	cderr "codedock/internal/errors"
	pkg "codedock/pkg/codex"
)

// Invoke 执行与 Codex 同名的动作。
func (rt *Runtime) Invoke(ctx context.Context, sessionID, name, args string) (pkg.CommandResult, error) {
	spec, ok := pkg.LookupCommand(name)
	if !ok {
		return pkg.CommandResult{}, cderr.Invalid("unknown command %s", name)
	}
	switch spec.Action {
	case pkg.ActionHint:
		return pkg.CommandResult{Hint: spec.Hint, Action: spec.Action, Handled: true}, nil
	case pkg.ActionApplySettings:
		patch := pkg.Settings{}
		switch spec.Field {
		case "model":
			patch.Model = args
		case "effort":
			patch.Effort = args
		case "collaboration_mode":
			patch.CollaborationMode = first(args, "plan")
		case "sandbox":
			patch.Sandbox = args
		case "approval_policy":
			patch.ApprovalPolicy = args
		}
		if _, err := rt.ApplySettings(ctx, sessionID, patch); err != nil {
			return pkg.CommandResult{}, err
		}
		return pkg.CommandResult{Action: spec.Action, Handled: true}, nil
	case pkg.ActionTurn:
		switch spec.Name {
		case "stop":
			st := rt.state(sessionID)
			st.mu.Lock()
			id := ""
			if st.active != nil {
				id = st.active.ID
			}
			st.mu.Unlock()
			if id == "" {
				return pkg.CommandResult{}, cderr.Invalid("no running turn")
			}
			return pkg.CommandResult{Action: spec.Action, Handled: true}, rt.CancelTurn(ctx, sessionID, id)
		case "compact":
			return pkg.CommandResult{Action: spec.Action, Handled: true}, rt.Compact(ctx, sessionID)
		case "review":
			return pkg.CommandResult{Action: spec.Action, Handled: true}, rt.Review(ctx, sessionID)
		}
	case pkg.ActionSession:
		switch spec.Name {
		case "archive":
			return pkg.CommandResult{Action: spec.Action, Handled: true}, rt.Archive(ctx, sessionID)
		case "rename":
			return pkg.CommandResult{Action: spec.Action, Handled: true}, rt.Rename(ctx, sessionID, strings.TrimSpace(args))
		case "fork":
			_, err := rt.Fork(ctx, sessionID)
			return pkg.CommandResult{Action: spec.Action, Handled: true}, err
		}
	case pkg.ActionAttach:
		switch spec.Name {
		case "mention":
			return pkg.CommandResult{Action: spec.Action, Handled: true}, rt.Mention(ctx, sessionID, strings.TrimSpace(args))
		case "image":
			return pkg.CommandResult{Action: spec.Action, Handled: true}, rt.AttachImage(ctx, sessionID, strings.TrimSpace(args))
		}
	}
	return pkg.CommandResult{Action: spec.Action}, nil
}
