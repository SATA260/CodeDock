package codex

import (
	"context"

	cderr "codedock/internal/errors"
	pkg "codedock/pkg/codex"
)

// ListSessions 从官方 thread/list 取对话。
func (rt *Runtime) ListSessions(ctx context.Context, archived bool, cursor string) (pkg.SessionPage, error) {
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return pkg.SessionPage{}, err
	}
	flag := archived
	page, err := client.ThreadList(ctx, pkg.ThreadListParams{Archived: &flag, Cursor: cursor, Limit: 50})
	if err != nil {
		return pkg.SessionPage{}, mapRPC(err)
	}
	out := pkg.SessionPage{NextCursor: page.NextCursor}
	for _, th := range page.Data {
		st := rt.state(th.ID)
		st.mu.Lock()
		active := ""
		if st.active != nil {
			active = st.active.ID
		}
		st.archived = archived
		st.mu.Unlock()
		out.Sessions = append(out.Sessions, pkg.MapThread(th, archived, active))
	}
	return out, nil
}

// CreateSession 向 Codex 开一条 thread，session_id 即 thread_id。
func (rt *Runtime) CreateSession(ctx context.Context, settings pkg.Settings) (pkg.Session, error) {
	client, err := rt.requireReady(ctx)
	if err != nil {
		return pkg.Session{}, err
	}
	params := pkg.ApplyThreadOverrides(pkg.ThreadStartParams{Cwd: settings.Cwd}, settings)
	res, err := client.ThreadStart(ctx, params)
	if err != nil {
		return pkg.Session{}, mapRPC(err)
	}
	st := rt.state(res.Thread.ID)
	st.mu.Lock()
	st.settings = settings.MergeOverride(settings)
	if st.settings.Cwd == "" {
		st.settings.Cwd = res.Cwd
	}
	if st.settings.Model == "" {
		st.settings.Model = res.Model
	}
	st.mu.Unlock()
	return pkg.MapThread(res.Thread, false, ""), nil
}

// GetSession 读一条对话：官方 thread 加本进程的执行位。
func (rt *Runtime) GetSession(ctx context.Context, sessionID string) (pkg.Session, []pkg.Progress, error) {
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return pkg.Session{}, nil, err
	}
	res, err := client.ThreadRead(ctx, pkg.ThreadReadParams{ThreadID: sessionID, IncludeTurns: true})
	if err != nil {
		return pkg.Session{}, nil, mapRPC(err)
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	active := ""
	if st.active != nil {
		active = st.active.ID
	}
	archived := st.archived
	st.mu.Unlock()
	return pkg.MapThread(res.Thread, archived, active), pkg.HydrateProgress(res.Thread.Turns), nil
}

// Rename 改标题。
func (rt *Runtime) Rename(ctx context.Context, sessionID, title string) error {
	if title == "" {
		return cderr.Invalid("title is required")
	}
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return err
	}
	return mapRPC(client.ThreadSetName(ctx, sessionID, title))
}

// Archive 归档。不删本机 Codex 记录。
func (rt *Runtime) Archive(ctx context.Context, sessionID string) error {
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return err
	}
	if err := mapRPC(client.ThreadArchive(ctx, sessionID)); err != nil {
		return err
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	st.archived = true
	st.mu.Unlock()
	return nil
}

// Fork 按官方历史分叉。
func (rt *Runtime) Fork(ctx context.Context, sessionID string) (pkg.Session, error) {
	client, err := rt.requireReady(ctx)
	if err != nil {
		return pkg.Session{}, err
	}
	res, err := client.ThreadFork(ctx, pkg.ThreadForkParams{ThreadID: sessionID})
	if err != nil {
		return pkg.Session{}, mapRPC(err)
	}
	src := rt.state(sessionID)
	dst := rt.state(res.Thread.ID)
	src.mu.Lock()
	settings := src.settings
	src.mu.Unlock()
	dst.mu.Lock()
	dst.settings = settings
	dst.mu.Unlock()
	return pkg.MapThread(res.Thread, false, ""), nil
}

// Effective 返回 Codex 默认与用户覆盖合并后的配置。
func (rt *Runtime) Effective(ctx context.Context, sessionID string) (pkg.Settings, error) {
	st := rt.state(sessionID)
	st.mu.Lock()
	local := st.settings
	st.mu.Unlock()
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return local, err
	}
	cfg, err := client.ConfigRead(ctx, local.Cwd)
	if err != nil {
		return local, nil
	}
	out := local
	if out.Model == "" {
		out.Model = pkg.ConfigString(cfg, "model")
	}
	if out.Effort == "" {
		out.Effort = pkg.ConfigString(cfg, "model_reasoning_effort")
	}
	if out.ApprovalPolicy == "" {
		out.ApprovalPolicy = pkg.ConfigString(cfg, "approval_policy")
	}
	if out.Sandbox == "" {
		out.Sandbox = pkg.ConfigString(cfg, "sandbox_mode")
	}
	return out, nil
}

// ApplySettings 记下用户改过的项。
func (rt *Runtime) ApplySettings(ctx context.Context, sessionID string, patch pkg.Settings) (pkg.Settings, error) {
	st := rt.state(sessionID)
	st.mu.Lock()
	st.settings = st.settings.MergeOverride(patch)
	st.mu.Unlock()
	return rt.Effective(ctx, sessionID)
}

// Mention 把仓库内文件挂到草稿上。
func (rt *Runtime) Mention(_ context.Context, sessionID, path string) error {
	if path == "" {
		return cderr.Invalid("path is required")
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	st.draft.Mentions = append(st.draft.Mentions, path)
	st.mu.Unlock()
	return nil
}

// AttachImage 把本地图片挂到草稿上。
func (rt *Runtime) AttachImage(_ context.Context, sessionID, path string) error {
	if path == "" {
		return cderr.Invalid("path is required")
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	st.draft.Images = append(st.draft.Images, path)
	st.mu.Unlock()
	return nil
}

func (rt *Runtime) takeDraft(sessionID string, content string, extra pkg.Input) pkg.Input {
	st := rt.state(sessionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	in := st.draft
	st.draft = pkg.Input{}
	if extra.Text != "" {
		in.Text = extra.Text
	}
	if content != "" {
		in.Text = content
	}
	in.Mentions = append(in.Mentions, extra.Mentions...)
	in.Images = append(in.Images, extra.Images...)
	return in
}
