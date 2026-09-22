package handler

import (
	"context"
	"encoding/json"
	"time"

	"codedock/internal/board"
	cderr "codedock/internal/errors"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/claude"
	pkgcodex "codedock/pkg/codex"
	"codedock/pkg/db/sqlite"
	"codedock/pkg/git"
	"codedock/pkg/github"
)

// boardPorts 把建会话、Git、gh、三引擎问票接到看板服务。
func (a *API) boardPorts() board.Ports {
	return board.Ports{
		StartNative: a.startNativeBoard,
		StartClaude: a.startClaudeBoard,
		StartCodex:  a.startCodexBoard,
		Lookup:      a.lookupBoardSession,
		ListEngine:  a.listBoardEngine,
		InspectDir:  inspectBoardDir,
		ViewIssue:   viewBoardIssue,
		ViewPull:    viewBoardPull,
		ListAsks:    a.listBoardAsks,
		Decide:      a.decideBoardAsk,
		ApplyDir:    a.applyBoardDir,
	}
}

// applyBoardDir 把会话目录写进对应引擎的工作目录。
func (a *API) applyBoardDir(ctx context.Context, engine board.Engine, sessionID, path string) error {
	if path == "" {
		return nil
	}
	switch engine {
	case board.EngineNative:
		if a == nil || a.q(ctx) == nil {
			return cderr.Unavailable("database is required")
		}
		return a.q(ctx).SetSessionWorkspace(ctx, sqlite.SetSessionWorkspaceParams{
			WorkspaceID: path,
			UpdatedAt:   util.FormatTime(util.Now()),
			ID:          sessionID,
		})
	case board.EngineClaude:
		_, err := claude.Apply(sessionID, claude.Settings{Cwd: path, Overridden: []string{"cwd"}})
		return err
	case board.EngineCodex:
		if a == nil || a.codex == nil {
			return cderr.Unavailable("codex is not configured")
		}
		a.codex.SetCwd(sessionID, path)
		return nil
	default:
		return cderr.Invalid("unknown engine")
	}
}

// startNativeBoard 走现有 CreateSession：冻 workspace_id。
func (a *API) startNativeBoard(ctx context.Context, spec board.StartSpec) (board.Started, error) {
	if a == nil || a.q(ctx) == nil {
		return board.Started{}, cderr.Unavailable("database is required")
	}
	if spec.UserID == "" {
		return board.Started{}, cderr.Invalid("user_id is required")
	}
	if spec.TenantID == "" {
		spec.TenantID = "default"
	}
	if spec.AgentID == "" {
		spec.AgentID = "default"
	}
	root, err := freezeSessionWorkspace(spec.WorkspaceID, a.cfg.DefaultRoot())
	if err != nil {
		return board.Started{}, err
	}
	now := util.FormatTime(util.Now())
	row, err := a.q(ctx).InsertSession(ctx, sqlite.InsertSessionParams{
		ID:          util.NewID(),
		TenantID:    spec.TenantID,
		UserID:      spec.UserID,
		AgentID:     spec.AgentID,
		WorkspaceID: root,
		Status:      string(pkgagent.SessionActive),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return board.Started{}, err
	}
	return board.Started{ID: row.ID, UpdatedAt: row.UpdatedAt}, nil
}

// startClaudeBoard 走现有 Claude 建会话，目录会话再记 cwd。
func (a *API) startClaudeBoard(_ context.Context, spec board.StartSpec) (board.Started, error) {
	sess, err := claude.Create(spec.UserID)
	if err != nil {
		return board.Started{}, err
	}
	if spec.WorkspaceID != "" {
		if _, applyErr := claude.Apply(sess.ID, claude.Settings{Cwd: spec.WorkspaceID, Overridden: []string{"cwd"}}); applyErr != nil {
			a.logger().Info("claude cwd apply skipped", "session_id", sess.ID, "error", applyErr)
		}
	}
	return board.Started{
		ID:        sess.ID,
		Summary:   sess.Title,
		UpdatedAt: stampRFC(sess.UpdatedAt),
	}, nil
}

// startCodexBoard 走现有 Codex 建会话。
func (a *API) startCodexBoard(ctx context.Context, spec board.StartSpec) (board.Started, error) {
	if a == nil || a.codex == nil {
		return board.Started{}, cderr.Unavailable("codex is not configured")
	}
	sess, err := a.codex.CreateSession(ctx, pkgcodex.Settings{Cwd: spec.WorkspaceID})
	if err != nil {
		return board.Started{}, err
	}
	return board.Started{
		ID:        sess.ID,
		Summary:   firstNonEmpty(sess.Title, sess.Preview),
		UpdatedAt: stampRFC(sess.UpdatedAt),
	}, nil
}

// lookupBoardSession 读三引擎会话摘要。
func (a *API) lookupBoardSession(ctx context.Context, engine board.Engine, sessionID string) (board.SessionMeta, error) {
	switch engine {
	case board.EngineNative:
		row, err := a.q(ctx).GetSession(ctx, sessionID)
		if err != nil {
			return board.SessionMeta{}, wrapHandlerDB(err)
		}
		pending, _ := a.q(ctx).CountPendingApprovals(ctx, sessionID)
		return board.SessionMeta{
			ID:        row.ID,
			Summary:   row.Summary,
			UpdatedAt: row.UpdatedAt,
			Running:   row.ActiveRunID.Valid && row.ActiveRunID.String != "",
			Pending:   int(pending),
			Archived:  row.Status == string(pkgagent.SessionArchived),
		}, nil
	case board.EngineClaude:
		sess, err := claude.Get(sessionID)
		if err != nil {
			return board.SessionMeta{}, err
		}
		return board.SessionMeta{
			ID:        sess.ID,
			Summary:   firstNonEmpty(sess.Title, sess.ClaudeSessionID),
			UpdatedAt: stampRFC(sess.UpdatedAt),
			Running:   sess.ActiveTurnID != "",
			Pending:   len(claude.PendingAsks(sess.ID)),
			Archived:  sess.Archived,
		}, nil
	case board.EngineCodex:
		if a.codex == nil {
			return board.SessionMeta{}, cderr.Unavailable("codex is not configured")
		}
		sess, _, err := a.codex.GetSession(ctx, sessionID)
		if err != nil {
			return board.SessionMeta{}, err
		}
		return board.SessionMeta{
			ID:        sess.ID,
			Summary:   firstNonEmpty(sess.Title, sess.Preview),
			UpdatedAt: stampRFC(sess.UpdatedAt),
			Running:   sess.ActiveTurnID != "",
			Pending:   len(a.codex.PendingAsks(sessionID)),
			Archived:  sess.Archived,
		}, nil
	default:
		return board.SessionMeta{}, cderr.Invalid("unknown engine")
	}
}

// listBoardEngine 列 Claude / Codex 会话给未归组列。
func (a *API) listBoardEngine(ctx context.Context, engine board.Engine) ([]board.SessionMeta, error) {
	switch engine {
	case board.EngineClaude:
		sessions, err := claude.ListSessions()
		if err != nil {
			return nil, err
		}
		out := make([]board.SessionMeta, 0, len(sessions))
		for _, sess := range sessions {
			out = append(out, board.SessionMeta{
				ID:        sess.ID,
				Summary:   firstNonEmpty(sess.Title, sess.ClaudeSessionID),
				UpdatedAt: stampRFC(sess.UpdatedAt),
				Running:   sess.ActiveTurnID != "",
				Pending:   len(claude.PendingAsks(sess.ID)),
				Archived:  sess.Archived,
			})
		}
		return out, nil
	case board.EngineCodex:
		if a.codex == nil {
			return nil, nil
		}
		page, err := a.codex.ListSessions(ctx, false, "")
		if err != nil {
			return nil, err
		}
		out := make([]board.SessionMeta, 0, len(page.Sessions))
		for _, sess := range page.Sessions {
			out = append(out, board.SessionMeta{
				ID:        sess.ID,
				Summary:   firstNonEmpty(sess.Title, sess.Preview),
				UpdatedAt: stampRFC(sess.UpdatedAt),
				Running:   sess.ActiveTurnID != "",
				Pending:   len(a.codex.PendingAsks(sess.ID)),
				Archived:  sess.Archived,
			})
		}
		return out, nil
	default:
		return nil, nil
	}
}

// listBoardAsks 列 Claude / Codex 待批，payload 保持原问票。
func (a *API) listBoardAsks(engine board.Engine, sessionID string) []board.InboxItem {
	var items []board.InboxItem
	switch engine {
	case board.EngineClaude:
		for _, ask := range claude.PendingAsks(sessionID) {
			raw, _ := json.Marshal(ask)
			items = append(items, board.InboxItem{
				Engine:    board.PublicEngine(engine),
				SessionID: sessionID,
				TicketID:  ask.ExternalRequestID,
				Summary:   firstNonEmpty(ask.Prompt, ask.Command, string(ask.Kind)),
				Payload:   raw,
			})
		}
	case board.EngineCodex:
		if a == nil || a.codex == nil {
			return items
		}
		for _, ask := range a.codex.PendingAsks(sessionID) {
			raw, _ := json.Marshal(ask)
			items = append(items, board.InboxItem{
				Engine:    board.PublicEngine(engine),
				SessionID: sessionID,
				TicketID:  firstNonEmpty(ask.ID, ask.ExternalRequestID),
				Summary:   firstNonEmpty(ask.Prompt, ask.Command, string(ask.Kind)),
				Payload:   raw,
			})
		}
	}
	return items
}

// decideBoardAsk 按引擎转给已有裁决。
func (a *API) decideBoardAsk(ctx context.Context, engine board.Engine, ticketID string, answer board.DecideAnswer) error {
	switch engine {
	case board.EngineNative:
		if answer.Native == nil {
			return cderr.Invalid("native decision is required")
		}
		req := DecideApprovalRequest{
			ApprovalID: ticketID,
			Status:     pkgagent.ApprovalStatus(answer.Native.Status),
			Scope:      pkgagent.ApprovalScope(answer.Native.Scope),
			ActorID:    answer.Native.ActorID,
			Reason:     answer.Native.Reason,
			Override:   pkgagent.OverrideAction(answer.Native.Override),
		}
		for _, item := range answer.Native.Decisions {
			req.Decisions = append(req.Decisions, ToolDecision{
				ToolCallID: item.ToolCallID,
				Status:     pkgagent.ApprovalStatus(item.Status),
				Reason:     item.Reason,
			})
		}
		_, err := a.decide(ctx, req)
		return err
	case board.EngineClaude:
		if answer.Claude == nil {
			return cderr.Invalid("claude decision is required")
		}
		return claude.Decide(ticketID, claude.AskAnswer{
			Approved: answer.Claude.Approved,
			Scope:    claude.DecisionScope(answer.Claude.Scope),
			Choice:   answer.Claude.Choice,
			Values:   answer.Claude.Values,
		})
	case board.EngineCodex:
		if a == nil || a.codex == nil {
			return cderr.Unavailable("codex is not configured")
		}
		if answer.Codex == nil {
			return cderr.Invalid("codex decision is required")
		}
		return a.codex.Decide(ctx, ticketID, pkgcodex.AskAnswer{
			Approved: answer.Codex.Approved,
			Scope:    pkgcodex.DecisionScope(answer.Codex.Scope),
			Choice:   answer.Codex.Choice,
			Values:   answer.Codex.Values,
			Answers:  answer.Codex.Answers,
		})
	default:
		return cderr.Invalid("unknown engine")
	}
}

// inspectBoardDir 现问 Git：分支、dirty、同仓其他 worktree。
func inspectBoardDir(path string) board.DirView {
	view := board.DirView{Path: path, SharedWriters: []string{}}
	repo, err := git.Open(path)
	if err != nil {
		return view
	}
	state, err := git.Status(repo, git.Checkout{Path: path})
	if err != nil || !state.IsRepo {
		return view
	}
	view.Branch = state.Branch
	view.Dirty = git.Dirty(state)
	if writers, err := git.SharedWriters(repo, git.Checkout{Path: path}); err == nil && writers != nil {
		view.SharedWriters = writers
	}
	return view
}

// viewBoardIssue 用本机 gh 拉 Issue 快照。
func viewBoardIssue(repo string, number int) (board.IssueSnap, error) {
	issue, err := github.ViewIssue(context.Background(), repo, int64(number))
	if err != nil {
		return board.IssueSnap{}, err
	}
	return board.IssueSnap{
		Repo:   issue.Repo,
		Number: int(issue.Number),
		Title:  issue.Title,
		Body:   issue.Body,
		URL:    issue.URL,
		State:  issue.State,
	}, nil
}

// viewBoardPull 用本机 gh 拉 PR 快照。
func viewBoardPull(repo string, number int) (board.PullSnap, error) {
	pull, err := github.ViewPull(context.Background(), repo, int64(number))
	if err != nil {
		return board.PullSnap{}, err
	}
	return board.PullSnap{
		Repo:   pull.Repo,
		Number: int(pull.Number),
		Title:  pull.Title,
		Body:   pull.Body,
		URL:    pull.URL,
		State:  pull.State,
	}, nil
}

// stampRFC 把 Unix 秒收成 RFC3339。
// stampRFC 把 Unix 秒收成 RFC3339；空值保持空串。
func stampRFC(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

// firstNonEmpty 取第一个非空字符串。
// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
