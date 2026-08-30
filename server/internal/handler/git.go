package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/git"

	"github.com/google/uuid"
)

const defaultCommitPrompt = "根据已暂存的 diff 写 Git 提交说明。第一行是不超过 72 个字符的标题，空一行后写正文。不要使用代码围栏，不要解释。"

type gitCheckoutRequest struct {
	Checkout string `json:"checkout"`
}

type gitPathsRequest struct {
	Paths    []string `json:"paths"`
	Checkout string   `json:"checkout"`
}

type gitCommitRequest struct {
	Message  string   `json:"message"`
	Paths    []string `json:"paths"`
	Checkout string   `json:"checkout"`
}

type gitResetRequest struct {
	Target   string `json:"target"`
	Mode     string `json:"mode"`
	Checkout string `json:"checkout"`
	Confirm  bool   `json:"confirm"`
}

type gitRevertRequest struct {
	ID       string `json:"id"`
	Checkout string `json:"checkout"`
}

type gitBranchCreateRequest struct {
	Name     string `json:"name"`
	Start    string `json:"start"`
	Checkout string `json:"checkout"`
}

type gitBranchNameRequest struct {
	Name     string `json:"name"`
	Checkout string `json:"checkout"`
}

type gitWorktreeCreateRequest struct {
	Path      string `json:"path"`       // 新检出的目录。
	Branch    string `json:"branch"`     // 挂到已有分支；和 new_branch 二选一或作起点。
	NewBranch string `json:"new_branch"` // 同时新建分支再挂上去；空则只用 branch。
}

type gitConflictWriteRequest struct {
	Path     string `json:"path"`
	Result   string `json:"result"`
	Checkout string `json:"checkout"`
}

type gitPromptRequest struct {
	SystemPrompt string `json:"system_prompt"`
}

type gitStashCreateRequest struct {
	AgentRun string `json:"agent_run"`
	Checkout string `json:"checkout"`
}

type gitStashRestoreRequest struct {
	ID       string `json:"id"`
	Checkout string `json:"checkout"`
}

type gitUndoClickRequest struct {
	ID       string `json:"id"`
	Checkout string `json:"checkout"`
}

// BranchView 给分支页看当前局面和近期分叉图。ahead/behind 记在每条 Branch 上，不合成一对数字。
type BranchView struct {
	Current string       `json:"current"` // 当前分支名；detached 时为空，界面要单独写「游离 HEAD」。
	Locals  []git.Branch `json:"locals"`  // 本地分支，每条自带 ahead/behind。
	Remotes []git.Branch `json:"remotes"` // 远程跟踪分支（fetch 缓存），不是 remote 地址。
	Graph   git.Graph    `json:"graph"`   // 近期分叉图，点只用于看。
}

// ConflictSession 是当时那份检出上的冲突会话，不要串到别的 worktree。
type ConflictSession struct {
	Kind   string             `json:"kind"`   // merge | rebase | cherry_pick | revert；空表示当前没有冲突会话。不是 pull。
	Ours   string             `json:"ours"`   // 我方分支或提交的可读名，对比视图左边用。
	Theirs string             `json:"theirs"` // 对方分支或提交的可读名，对比视图右边用。
	Items  []git.ConflictItem `json:"items"`  // 尚未解决的文件；全部写完才能 Continue。
}

// MessageDraft 是生成的提交说明，用户确认前可以改。
type MessageDraft struct {
	Title string `json:"title"` // 生成的标题行。
	Body  string `json:"body"`  // 生成的正文。
}

// PromptConfig 是生成说明用的 system prompt。
type PromptConfig struct {
	SystemPrompt string `json:"system_prompt"` // 空则用产品默认。
}

// AgentSnapshot 是 Agent 改工作区前的副本，不是用户 stash 列表里的条目。
type AgentSnapshot struct {
	ID           string       `json:"id"`            // 这份快照自己的 ID，撤销按钮用它找回。
	Checkout     git.Checkout `json:"checkout"`      // 创建时的那份检出；恢复必须还在这份检出上。
	Head         string       `json:"head"`          // 创建时的 HEAD；恢复时先回到这个提交。
	StashOID     string       `json:"stash_oid"`     // git stash create 得到的悬空提交，含暂存区加已跟踪工作区。
	HasUntracked bool         `json:"has_untracked"` // 创建时工作区有未跟踪文件；stash create 不备份它们，恢复按钮必须写清。
	AgentRun     string       `json:"agent_run"`     // 对应的 Agent Run，给按钮文案用。
}

// UndoButton 是给用户看的撤销入口，不出现 reset / revert 命令名。
type UndoButton struct {
	ID       string `json:"id"`        // 按钮 ID，点下去交给 Click。
	Label    string `json:"label"`     // 给用户看的文案。
	Risk     string `json:"risk"`      // 空表示可安全撤；有则必须展示，例如会丢掉未推送之外的提交。
	Target   string `json:"target"`    // last_commit | path | uncommitted | integrate | agent_stash。
	TargetID string `json:"target_id"` // 目标提交、路径或快照 ID；针对整份工作区或当前整合时可空。
}

func mapGitErr(err error) error {
	if err == nil {
		return nil
	}
	if cderr.IsNotFound(err) || cderr.IsConflict(err) || cderr.IsInvalid(err) || cderr.IsUnavailable(err) || cderr.IsUnauthorized(err) {
		return err
	}
	switch {
	case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrCurrentBranch):
		return cderr.Invalid("%s", err.Error())
	case errors.Is(err, git.ErrConflict), errors.Is(err, git.ErrDirty), errors.Is(err, git.ErrIntegrating):
		return cderr.Conflict("%s", err.Error())
	default:
		return cderr.Invalid("%s", err.Error())
	}
}

func writeGitError(w http.ResponseWriter, err error) {
	writeError(w, mapGitErr(err))
}

func sameCheckout(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	if ra, err := filepath.EvalSymlinks(aa); err == nil {
		aa = ra
	}
	if rb, err := filepath.EvalSymlinks(bb); err == nil {
		bb = rb
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func (a *API) gitRoot() (string, error) {
	if a != nil && strings.TrimSpace(a.cfg.GitRepo) != "" {
		return filepath.Abs(a.cfg.GitRepo)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", cderr.Unavailable("cannot resolve current folder")
	}
	return cwd, nil
}

func (a *API) openSite(checkout string) (git.Repo, git.Checkout, error) {
	root, err := a.gitRoot()
	if err != nil {
		return git.Repo{}, git.Checkout{}, err
	}
	repo, err := git.Open(root)
	if err != nil {
		return git.Repo{}, git.Checkout{}, mapGitErr(err)
	}
	path := root
	if strings.TrimSpace(checkout) != "" {
		path = checkout
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return git.Repo{}, git.Checkout{}, cderr.Invalid("invalid checkout")
	}
	if !sameCheckout(abs, root) {
		trees, err := git.ListWorktrees(repo)
		if err != nil {
			return git.Repo{}, git.Checkout{}, mapGitErr(err)
		}
		found := false
		for _, tree := range trees {
			if sameCheckout(tree.Path, abs) {
				found = true
				break
			}
		}
		if !found {
			return git.Repo{}, git.Checkout{}, cderr.Invalid("checkout is not a worktree of this repo")
		}
	}
	return repo, git.Checkout{Path: abs}, nil
}

func (a *API) conflictSession(repo git.Repo, co git.Checkout) (ConflictSession, error) {
	state, err := git.Status(repo, co)
	if err != nil {
		return ConflictSession{}, err
	}
	ours, theirs, err := git.ConflictNames(repo, co)
	if err != nil {
		return ConflictSession{}, err
	}
	sess := ConflictSession{Kind: state.Integrating, Ours: ours, Theirs: theirs, Items: []git.ConflictItem{}}
	for _, file := range state.Files {
		if !file.Unmerged {
			continue
		}
		item, err := git.ReadConflict(repo, co, file.Path)
		if err != nil {
			item = git.ConflictItem{Path: file.Path}
		}
		sess.Items = append(sess.Items, item)
	}
	return sess, nil
}

func codedockFile(repo git.Repo, name string) (string, error) {
	gd, err := git.CommonDir(repo)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(gd, "codedock")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func (a *API) loadPrompt(repo git.Repo) string {
	path, err := codedockFile(repo, "commit-message-prompt")
	if err != nil {
		return defaultCommitPrompt
	}
	body, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(body)) == "" {
		return defaultCommitPrompt
	}
	return string(body)
}

func (a *API) savePrompt(repo git.Repo, prompt string) error {
	path, err := codedockFile(repo, "commit-message-prompt")
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(prompt), 0o644)
}

func (a *API) loadSnapshots(repo git.Repo) ([]AgentSnapshot, error) {
	path, err := codedockFile(repo, "snapshots.json")
	if err != nil {
		if errors.Is(err, git.ErrNotRepo) {
			return []AgentSnapshot{}, nil
		}
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []AgentSnapshot{}, nil
		}
		return nil, err
	}
	var items []AgentSnapshot
	if err := json.Unmarshal(body, &items); err != nil {
		return []AgentSnapshot{}, nil
	}
	if items == nil {
		return []AgentSnapshot{}, nil
	}
	return items, nil
}

func (a *API) saveSnapshots(repo git.Repo, items []AgentSnapshot) error {
	path, err := codedockFile(repo, "snapshots.json")
	if err != nil {
		return err
	}
	body, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func latestSnapshot(items []AgentSnapshot, checkout string) AgentSnapshot {
	for i := len(items) - 1; i >= 0; i-- {
		if checkout == "" || sameCheckout(items[i].Checkout.Path, checkout) {
			return items[i]
		}
	}
	return AgentSnapshot{}
}

func snapshotByID(items []AgentSnapshot, id string) (AgentSnapshot, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return AgentSnapshot{}, false
}

func parseDraft(text string) MessageDraft {
	text = strings.TrimSpace(text)
	title, body, ok := strings.Cut(text, "\n")
	if !ok {
		return MessageDraft{Title: text}
	}
	return MessageDraft{Title: strings.TrimSpace(title), Body: strings.TrimSpace(body)}
}

func (a *API) gitModel() pkgagent.ModelConfig {
	opts, err := json.Marshal(map[string]string{
		"api_key":  a.cfg.LLMAPIKey,
		"base_url": a.cfg.LLMBaseURL,
	})
	if err != nil {
		opts = json.RawMessage(`{}`)
	}
	return pkgagent.ModelConfig{
		Provider: a.cfg.LLMProvider,
		Model:    a.cfg.LLMModel,
		Options:  opts,
	}
}

func fallbackDraft(files []git.DiffFile) string {
	if len(files) == 0 {
		return ""
	}
	title := files[0].Kind + " " + files[0].Path
	var body strings.Builder
	for _, file := range files {
		body.WriteString(file.Kind)
		body.WriteByte(' ')
		body.WriteString(file.Path)
		body.WriteByte('\n')
	}
	return title + "\n\n" + strings.TrimSpace(body.String())
}

func checkoutRelPath(root, rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", cderr.Invalid("invalid path")
	}
	abs := filepath.Join(root, rel)
	inside, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(inside, "..") {
		return "", cderr.Invalid("invalid path")
	}
	return abs, nil
}

func undoButtons(state git.SiteState, graph git.Graph, snap AgentSnapshot) []UndoButton {
	buttons := []UndoButton{}
	if !state.Empty && state.Head != "" {
		hasParent := false
		for _, node := range graph.Nodes {
			if node.Commit.ID == state.Head && len(node.Commit.Parents) > 0 {
				hasParent = true
				break
			}
		}
		if hasParent {
			risk := ""
			if state.Upstream != "" && state.Ahead == 0 {
				risk = "这次提交已经推送，撤销会改写已发布历史"
			}
			buttons = append(buttons, UndoButton{
				ID:     "last_commit",
				Label:  "撤销上次提交",
				Risk:   risk,
				Target: "last_commit",
			})
		}
		if len(state.Files) > 0 {
			buttons = append(buttons, UndoButton{
				ID:     "uncommitted",
				Label:  "丢弃未提交的改动",
				Risk:   "工作区和暂存区的改动都会丢掉；未跟踪文件会留下",
				Target: "uncommitted",
			})
		}
		for _, file := range state.Files {
			if file.Unmerged || file.WorktreeStatus == "?" {
				continue
			}
			buttons = append(buttons, UndoButton{
				ID:       "path:" + file.Path,
				Label:    "还原 " + file.Path,
				Risk:     "会丢掉这个文件的未提交改动",
				Target:   "path",
				TargetID: file.Path,
			})
		}
	}
	if state.Integrating != "" {
		buttons = append(buttons, UndoButton{
			ID:     "integrate",
			Label:  "中止当前整合",
			Risk:   "未完成的整合会被放弃",
			Target: "integrate",
		})
	}
	if snap.ID != "" {
		risk := "会回到快照时的提交，之后的提交可能丢掉"
		if snap.HasUntracked {
			risk += "；未跟踪文件不在快照里"
		}
		buttons = append(buttons, UndoButton{
			ID:       "agent_stash",
			Label:    "恢复 Agent 改文件前的快照",
			Risk:     risk,
			Target:   "agent_stash",
			TargetID: snap.ID,
		})
	}
	return buttons
}

// GitStatus 给界面看当前整局。
func (a *API) GitStatus(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// GitDiff 读已暂存或工作区差异。
func (a *API) GitDiff(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "staged"
	}
	files, err := git.Diff(repo, co, scope)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// GitGraph 读近期分叉图。
func (a *API) GitGraph(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	graph, err := git.LogGraph(repo, 50)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

// GitLog 读当前检出线上的近期提交。
func (a *API) GitLog(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n <= 0 {
			writeError(w, cderr.Invalid("invalid limit"))
			return
		}
		limit = n
	}
	commits, err := git.Log(repo, co, limit)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commits": commits})
}

// GitStage 暂存选中路径。
func (a *API) GitStage(w http.ResponseWriter, r *http.Request) {
	var req gitPathsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.Stage(repo, co, req.Paths); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitUnstage 取消暂存选中路径。
func (a *API) GitUnstage(w http.ResponseWriter, r *http.Request) {
	var req gitPathsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.Unstage(repo, co, req.Paths); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitDiscard 撤回工作区改动：已跟踪还原成暂存区，未跟踪删除。
func (a *API) GitDiscard(w http.ResponseWriter, r *http.Request) {
	var req gitPathsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.Discard(repo, co, req.Paths); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitCommit 用已确认的说明提交；paths 非空则先暂存。
func (a *API) GitCommit(w http.ResponseWriter, r *http.Request) {
	var req gitCommitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if len(req.Paths) > 0 {
		if err := git.Stage(repo, co, req.Paths); err != nil {
			writeGitError(w, err)
			return
		}
	}
	commit, err := git.CreateCommit(repo, co, req.Message)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commit": commit})
}

// GitReset 按 mode 重置；mixed / hard 必须 confirm。
func (a *API) GitReset(w http.ResponseWriter, r *http.Request) {
	var req gitResetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Target) == "" {
		writeError(w, cderr.Invalid("target is required"))
		return
	}
	if req.Mode == "" {
		req.Mode = "mixed"
	}
	if req.Mode != "soft" && req.Mode != "mixed" && req.Mode != "hard" {
		writeError(w, cderr.Invalid("mode must be soft, mixed, or hard"))
		return
	}
	if req.Mode != "soft" && !req.Confirm {
		writeError(w, cderr.Invalid("mixed/hard reset requires confirm"))
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.Reset(repo, co, req.Target, req.Mode); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitRevert 用一次新提交回退指定提交。
func (a *API) GitRevert(w http.ResponseWriter, r *http.Request) {
	var req gitRevertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	commit, err := git.Revert(repo, co, git.Commit{ID: req.ID})
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commit": commit})
}

// GitPush 推送当前分支。
func (a *API) GitPush(w http.ResponseWriter, r *http.Request) {
	var req gitCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.Push(repo, co); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitPull 拉取最新；有冲突则交给冲突模块。
func (a *API) GitPull(w http.ResponseWriter, r *http.Request) {
	var req gitCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.Pull(repo, co); err != nil {
		if errors.Is(err, git.ErrConflict) {
			sess, sessErr := a.conflictSession(repo, co)
			if sessErr != nil {
				writeGitError(w, sessErr)
				return
			}
			writeJSON(w, http.StatusConflict, sess)
			return
		}
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitListRemotes 列出已配置的 remote。
func (a *API) GitListRemotes(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	remotes, err := git.ListRemotes(repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remotes": remotes})
}

// GitListWorktrees 列出该仓库的全部检出。
func (a *API) GitListWorktrees(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite("")
	if err != nil {
		writeGitError(w, err)
		return
	}
	trees, err := git.ListWorktrees(repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"worktrees": trees})
}

// GitAddWorktree 创建一份检出。
func (a *API) GitAddWorktree(w http.ResponseWriter, r *http.Request) {
	var req gitWorktreeCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, _, err := a.openSite("")
	if err != nil {
		writeGitError(w, err)
		return
	}
	tree, err := git.AddWorktree(repo, req.Path, req.Branch, req.NewBranch)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"worktree": tree})
}

// GitListBranches 给分支页看当前局面和近期分叉图。
func (a *API) GitListBranches(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeGitError(w, err)
		return
	}
	listed, err := git.ListBranches(repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	graph, err := git.LogGraph(repo, 50)
	if err != nil {
		writeGitError(w, err)
		return
	}
	view := BranchView{Current: state.Branch, Locals: []git.Branch{}, Remotes: []git.Branch{}, Graph: graph}
	for _, b := range listed {
		if b.IsRemote {
			view.Remotes = append(view.Remotes, b)
		} else {
			view.Locals = append(view.Locals, b)
		}
	}
	writeJSON(w, http.StatusOK, view)
}

// GitCreateBranch 从起点建一条分支。
func (a *API) GitCreateBranch(w http.ResponseWriter, r *http.Request) {
	var req gitBranchCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.CreateBranch(repo, co, req.Name, req.Start); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitSwitchBranch 切到指定分支。
func (a *API) GitSwitchBranch(w http.ResponseWriter, r *http.Request) {
	var req gitBranchNameRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.SwitchBranch(repo, co, req.Name); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitDeleteBranch 删除一条本地分支。
func (a *API) GitDeleteBranch(w http.ResponseWriter, r *http.Request) {
	var req gitBranchNameRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, _, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.DeleteBranch(repo, req.Name); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitGetConflict 打开当前冲突会话。
func (a *API) GitGetConflict(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	sess, err := a.conflictSession(repo, co)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// GitWriteConflict 写入某文件的决议。
func (a *API) GitWriteConflict(w http.ResponseWriter, r *http.Request) {
	var req gitConflictWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	abs, err := checkoutRelPath(co.Path, req.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeGitError(w, err)
		return
	}
	if err := os.WriteFile(abs, []byte(req.Result), 0o644); err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.Stage(repo, co, []string{req.Path}); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitContinueConflict 全部写完后继续整合。
func (a *API) GitContinueConflict(w http.ResponseWriter, r *http.Request) {
	var req gitCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.ContinueIntegrate(repo, co); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitAbortConflict 放弃本次整合。
func (a *API) GitAbortConflict(w http.ResponseWriter, r *http.Request) {
	var req gitCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := git.AbortIntegrate(repo, co); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitGetPrompt 读生成说明用的 system prompt。
func (a *API) GitGetPrompt(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, PromptConfig{SystemPrompt: a.loadPrompt(repo)})
}

// GitSetPrompt 保存用户自定义的 system prompt。
func (a *API) GitSetPrompt(w http.ResponseWriter, r *http.Request) {
	var req gitPromptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, _, err := a.openSite("")
	if err != nil {
		writeGitError(w, err)
		return
	}
	if err := a.savePrompt(repo, req.SystemPrompt); err != nil {
		writeGitError(w, err)
		return
	}
	prompt := req.SystemPrompt
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultCommitPrompt
	}
	writeJSON(w, http.StatusOK, PromptConfig{SystemPrompt: prompt})
}

// GitGenerateMessage 读已暂存 diff，生成说明草稿。
func (a *API) GitGenerateMessage(w http.ResponseWriter, r *http.Request) {
	var req gitCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	files, err := git.Diff(repo, co, "staged")
	if err != nil {
		writeGitError(w, err)
		return
	}
	if len(files) == 0 {
		writeError(w, cderr.Invalid("nothing staged"))
		return
	}
	draft := a.generateDraft(r.Context(), repo, files)
	draft.Title = strings.TrimSpace(draft.Title)
	if draft.Title == "" {
		draft = parseDraft(fallbackDraft(files))
	}
	writeJSON(w, http.StatusOK, draft)
}

func (a *API) generateDraft(ctx context.Context, repo git.Repo, files []git.DiffFile) MessageDraft {
	var b strings.Builder
	for _, file := range files {
		if file.Binary {
			b.WriteString("binary " + file.Kind + " " + file.Path + "\n")
			continue
		}
		if file.Patch != "" {
			b.WriteString(file.Patch)
			b.WriteByte('\n')
			continue
		}
		b.WriteString(file.Kind + " " + file.Path + "\n")
	}
	stream, err := pkgagent.Stream(ctx, pkgagent.Chat{
		Model:        a.gitModel(),
		SystemPrompt: a.loadPrompt(repo),
		Messages: []pkgagent.Message{{
			Role:    pkgagent.RoleUser,
			Content: pkgagent.EncodeText(b.String()),
		}},
	})
	if err != nil {
		return parseDraft(fallbackDraft(files))
	}
	defer stream.Close()
	result, err := stream.Result(ctx)
	if err != nil {
		return parseDraft(fallbackDraft(files))
	}
	text := strings.TrimSpace(pkgagent.DecodeText(result.Message.Content))
	if text == "" || text == "ok" {
		return parseDraft(fallbackDraft(files))
	}
	return parseDraft(text)
}

// GitCreateSnapshot 复制当时提交和工作区，不挪走现有改动。
func (a *API) GitCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req gitStashCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeGitError(w, err)
		return
	}
	oid, err := git.CaptureWork(repo, co, req.AgentRun)
	if err != nil {
		writeGitError(w, err)
		return
	}
	snap := AgentSnapshot{
		ID:           uuid.NewString(),
		Checkout:     git.Checkout{Path: co.Path, CurrentBranch: state.Branch, CurrentCommit: state.Head, Detached: state.Detached},
		Head:         state.Head,
		StashOID:     oid,
		HasUntracked: hasUntrackedFiles(state),
		AgentRun:     req.AgentRun,
	}
	items, err := a.loadSnapshots(repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	items = append(items, snap)
	if err := a.saveSnapshots(repo, items); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func hasUntrackedFiles(state git.SiteState) bool {
	for _, file := range state.Files {
		if file.WorktreeStatus == "?" {
			return true
		}
	}
	return false
}

// GitLatestSnapshot 取最近一份 Agent 快照。
func (a *API) GitLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	items, err := a.loadSnapshots(repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, latestSnapshot(items, co.Path))
}

// GitRestoreSnapshot 回到快照时的提交，并把副本铺回工作区。
func (a *API) GitRestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	var req gitStashRestoreRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	items, err := a.loadSnapshots(repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	var snap AgentSnapshot
	if req.ID != "" {
		var ok bool
		snap, ok = snapshotByID(items, req.ID)
		if !ok {
			writeError(w, cderr.NotFound("snapshot not found"))
			return
		}
	} else {
		snap = latestSnapshot(items, co.Path)
		if snap.ID == "" {
			writeError(w, cderr.NotFound("snapshot not found"))
			return
		}
	}
	if err := restoreSnapshotOn(repo, co, snap); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func restoreSnapshotOn(repo git.Repo, co git.Checkout, snap AgentSnapshot) error {
	if snap.ID == "" {
		return cderr.NotFound("snapshot not found")
	}
	if snap.Checkout.Path != "" && !sameCheckout(snap.Checkout.Path, co.Path) {
		return cderr.Invalid("snapshot belongs to another checkout")
	}
	return git.RestoreWork(repo, co, snap.StashOID, snap.Head)
}

// GitListUndo 按当前 SiteState 算出能点的撤销按钮。
func (a *API) GitListUndo(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeGitError(w, err)
		return
	}
	graph, err := git.LogGraph(repo, 50)
	if err != nil {
		writeGitError(w, err)
		return
	}
	items, err := a.loadSnapshots(repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buttons": undoButtons(state, graph, latestSnapshot(items, co.Path))})
}

// GitClickUndo 执行该按钮对应的重置、回退或恢复。
func (a *API) GitClickUndo(w http.ResponseWriter, r *http.Request) {
	var req gitUndoClickRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	switch {
	case req.ID == "last_commit":
		err = git.Reset(repo, co, "HEAD~1", "mixed")
	case req.ID == "uncommitted":
		err = git.Reset(repo, co, "HEAD", "hard")
	case req.ID == "integrate":
		err = git.AbortIntegrate(repo, co)
	case req.ID == "agent_stash" || strings.HasPrefix(req.ID, "agent_stash:"):
		items, loadErr := a.loadSnapshots(repo)
		if loadErr != nil {
			writeGitError(w, loadErr)
			return
		}
		id := strings.TrimPrefix(req.ID, "agent_stash:")
		var snap AgentSnapshot
		if id == "" || id == "agent_stash" {
			snap = latestSnapshot(items, co.Path)
		} else {
			var ok bool
			snap, ok = snapshotByID(items, id)
			if !ok {
				writeError(w, cderr.NotFound("snapshot not found"))
				return
			}
		}
		err = restoreSnapshotOn(repo, co, snap)
	case strings.HasPrefix(req.ID, "path:"):
		err = git.RestorePath(repo, co, strings.TrimPrefix(req.ID, "path:"))
	default:
		writeError(w, cderr.Invalid("unknown undo button"))
		return
	}
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
