package codex

import (
	"context"
	"strings"

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
	out.Sessions = dedupeSessions(out.Sessions)
	return out, nil
}

// dedupeSessions 去掉官方 thread/list 里同一 thread_id 的重复行，留下更新时间最新的一条。
func dedupeSessions(in []pkg.Session) []pkg.Session {
	if len(in) < 2 {
		return in
	}
	index := make(map[string]int, len(in))
	out := make([]pkg.Session, 0, len(in))
	for _, sess := range in {
		if i, ok := index[sess.ID]; ok {
			if sess.UpdatedAt >= out[i].UpdatedAt {
				out[i] = sess
			}
			continue
		}
		index[sess.ID] = len(out)
		out = append(out, sess)
	}
	return out
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
	st.loaded = true
	if st.settings.Cwd == "" {
		st.settings.Cwd = res.Cwd
	}
	if st.settings.Model == "" {
		st.settings.Model = res.Model
	}
	st.mu.Unlock()
	return pkg.MapThread(res.Thread, false, ""), nil
}

// GetSession 打开一条对话。历史会话必须先 thread/resume，否则随后 turn/start 会报 thread not found。
func (rt *Runtime) GetSession(ctx context.Context, sessionID string) (pkg.Session, []pkg.Progress, error) {
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return pkg.Session{}, nil, err
	}
	if resumed, err := rt.resumeThread(ctx, client, sessionID); err == nil {
		rt.markLoaded(sessionID)
		return rt.sessionFromThread(sessionID, resumed.Thread), pkg.HydrateProgress(resumed.Thread.Turns), nil
	} else if !readFallback(err) {
		return pkg.Session{}, nil, mapRPC(err)
	}
	res, err := client.ThreadRead(ctx, pkg.ThreadReadParams{ThreadID: sessionID, IncludeTurns: true})
	if err != nil && includeTurnsUnavailable(err) {
		res, err = client.ThreadRead(ctx, pkg.ThreadReadParams{ThreadID: sessionID})
	}
	if err != nil {
		if emptyThread(err) {
			return stubSession(rt, sessionID), nil, nil
		}
		return pkg.Session{}, nil, mapRPC(err)
	}
	return rt.sessionFromThread(sessionID, res.Thread), pkg.HydrateProgress(res.Thread.Turns), nil
}

func (rt *Runtime) resumeThread(ctx context.Context, client *pkg.Client, sessionID string) (pkg.ThreadStartResult, error) {
	params := pkg.ThreadResumeParams{ThreadID: sessionID}
	if settings, err := rt.Effective(ctx, sessionID); err == nil {
		params.Model = settings.Model
		params.Cwd = settings.Cwd
	}
	return client.ThreadResume(ctx, params)
}

func (rt *Runtime) ensureResumed(ctx context.Context, client *pkg.Client, sessionID string) error {
	st := rt.state(sessionID)
	st.mu.Lock()
	if st.loaded {
		st.mu.Unlock()
		return nil
	}
	st.mu.Unlock()
	_, err := rt.resumeThread(ctx, client, sessionID)
	if err == nil || alreadyOpen(err) {
		rt.markLoaded(sessionID)
		return nil
	}
	return err
}

func (rt *Runtime) markLoaded(sessionID string) {
	st := rt.state(sessionID)
	st.mu.Lock()
	st.loaded = true
	st.mu.Unlock()
}

func (rt *Runtime) sessionFromThread(sessionID string, th pkg.ThreadObject) pkg.Session {
	st := rt.state(sessionID)
	st.mu.Lock()
	active := ""
	if st.active != nil {
		active = st.active.ID
	}
	archived := st.archived
	if st.settings.Cwd == "" && th.Cwd != "" {
		st.settings.Cwd = th.Cwd
	}
	st.mu.Unlock()
	return pkg.MapThread(th, archived, active)
}

// SetCwd 记下这条对话接下来使用的工作目录。
func (rt *Runtime) SetCwd(sessionID, cwd string) {
	if rt == nil || sessionID == "" || cwd == "" {
		return
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	st.settings.Cwd = cwd
	st.mu.Unlock()
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

// Fork 按官方历史分叉，标题加 (1)(2)(3)，原对话不动。
func (rt *Runtime) Fork(ctx context.Context, sessionID string) (pkg.Session, error) {
	client, err := rt.requireReady(ctx)
	if err != nil {
		return pkg.Session{}, err
	}
	base := rt.threadTitle(ctx, client, sessionID)
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
	title := nextForkTitle(base, rt.sessionTitles(ctx))
	if err := mapRPC(client.ThreadSetName(ctx, res.Thread.ID, title)); err != nil {
		return pkg.Session{}, err
	}
	res.Thread.Name = title
	return pkg.MapThread(res.Thread, false, ""), nil
}

// threadTitle 读官方 thread 标题，没有名字则用预览。
func (rt *Runtime) threadTitle(ctx context.Context, client *pkg.Client, sessionID string) string {
	res, err := client.ThreadRead(ctx, pkg.ThreadReadParams{ThreadID: sessionID})
	if err != nil {
		return ""
	}
	if name := strings.TrimSpace(res.Thread.Name); name != "" {
		return name
	}
	return strings.TrimSpace(res.Thread.Preview)
}

// sessionTitles 收集本机已有对话标题，用来给 fork 编号。
func (rt *Runtime) sessionTitles(ctx context.Context) []string {
	page, err := rt.ListSessions(ctx, false, "")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(page.Sessions))
	for _, sess := range page.Sessions {
		if sess.Title != "" {
			out = append(out, sess.Title)
			continue
		}
		if sess.Preview != "" {
			out = append(out, sess.Preview)
		}
	}
	return out
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

func includeTurnsUnavailable(err error) bool {
	return emptyThread(err)
}

func emptyThread(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "includeturns") ||
		strings.Contains(msg, "not materialized") ||
		strings.Contains(msg, "thread not loaded")
}

func threadUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "unknown thread") ||
		strings.Contains(msg, "not materialized") ||
		strings.Contains(msg, "thread not loaded")
}

func alreadyOpen(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "active writer") || strings.Contains(msg, "already loaded") {
		return true
	}
	return strings.Contains(msg, "already") &&
		(strings.Contains(msg, "open") || strings.Contains(msg, "owned") || strings.Contains(msg, "writer"))
}

func readFallback(err error) bool {
	return threadUnavailable(err) || emptyThread(err) || alreadyOpen(err) ||
		strings.Contains(strings.ToLower(err.Error()), "another process")
}

func stubSession(rt *Runtime, sessionID string) pkg.Session {
	st := rt.state(sessionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	active := ""
	if st.active != nil {
		active = st.active.ID
	}
	return pkg.Session{
		ID:           sessionID,
		ThreadID:     sessionID,
		Cwd:          st.settings.Cwd,
		ActiveTurnID: active,
		Archived:     st.archived,
	}
}
