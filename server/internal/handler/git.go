package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	cderr "codedock/internal/errors"
	"codedock/pkg/git"
)

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
		return git.Repo{}, git.Checkout{}, err
	}
	path := root
	if strings.TrimSpace(checkout) != "" {
		path = checkout
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return git.Repo{}, git.Checkout{}, cderr.Invalid("invalid checkout")
	}
	// TODO: checkout 必须是主仓或 ListWorktrees 里的路径。
	_, _ = git.ListWorktrees(repo)
	return repo, git.Checkout{Path: abs}, nil
}

// GitStatus 给界面看当前整局。
func (a *API) GitStatus(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// GitDiff 读已暂存或工作区差异。
func (a *API) GitDiff(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "staged"
	}
	files, err := git.Diff(repo, co, scope)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// GitGraph 读近期分叉图。
func (a *API) GitGraph(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	graph, err := git.LogGraph(repo, 50)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
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
		writeError(w, err)
		return
	}
	if err := git.Stage(repo, co, req.Paths); err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	if err := git.Unstage(repo, co, req.Paths); err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	if len(req.Paths) > 0 {
		if err := git.Stage(repo, co, req.Paths); err != nil {
			writeError(w, err)
			return
		}
	}
	commit, err := git.CreateCommit(repo, co, req.Message)
	if err != nil {
		writeError(w, err)
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
	// TODO: mixed/hard 必须 confirm；Empty 时不能 HEAD~1。
	_ = req.Confirm
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := git.Reset(repo, co, req.Target, req.Mode); err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	commit, err := git.Revert(repo, co, git.Commit{ID: req.ID})
	if err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	if err := git.Push(repo, co); err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	if err := git.Pull(repo, co); err != nil {
		// TODO: ErrConflict 时写 ConflictSession 409。
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitListRemotes 列出已配置的 remote。
func (a *API) GitListRemotes(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	remotes, err := git.ListRemotes(repo)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remotes": remotes})
}

// GitListWorktrees 列出该仓库的全部检出。
func (a *API) GitListWorktrees(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite("")
	if err != nil {
		writeError(w, err)
		return
	}
	trees, err := git.ListWorktrees(repo)
	if err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	tree, err := git.AddWorktree(repo, req.Path, req.Branch, req.NewBranch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"worktree": tree})
}

// GitListBranches 给分支页看当前局面和近期分叉图。
func (a *API) GitListBranches(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeError(w, err)
		return
	}
	listed, err := git.ListBranches(repo)
	if err != nil {
		writeError(w, err)
		return
	}
	graph, err := git.LogGraph(repo, 50)
	if err != nil {
		writeError(w, err)
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
	repo, _, err := a.openSite(req.Checkout)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := git.CreateBranch(repo, req.Name, req.Start); err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	if err := git.SwitchBranch(repo, co, req.Name); err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	if err := git.DeleteBranch(repo, req.Name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitGetConflict 打开当前冲突会话。
func (a *API) GitGetConflict(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeError(w, err)
		return
	}
	sess := ConflictSession{Kind: state.Integrating, Items: []git.ConflictItem{}}
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
		writeError(w, err)
		return
	}
	// TODO: 写入工作区并 Stage 该路径。
	_ = repo
	_ = co
	_ = req
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
		writeError(w, err)
		return
	}
	if err := git.ContinueIntegrate(repo, co); err != nil {
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	if err := git.AbortIntegrate(repo, co); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitGetPrompt 读生成说明用的 system prompt。
func (a *API) GitGetPrompt(w http.ResponseWriter, r *http.Request) {
	// TODO: 读用户自定义 prompt；空则产品默认。
	_ = r
	writeJSON(w, http.StatusOK, PromptConfig{})
}

// GitSetPrompt 保存用户自定义的 system prompt。
func (a *API) GitSetPrompt(w http.ResponseWriter, r *http.Request) {
	var req gitPromptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	// TODO: 保存 PromptConfig。
	writeJSON(w, http.StatusOK, PromptConfig{SystemPrompt: req.SystemPrompt})
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
		writeError(w, err)
		return
	}
	files, err := git.Diff(repo, co, "staged")
	if err != nil {
		writeError(w, err)
		return
	}
	// TODO: 二进制不喂模型；一次短调用，不进 Agent Loop。
	_ = files
	writeJSON(w, http.StatusOK, MessageDraft{})
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
		writeError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeError(w, err)
		return
	}
	oid, err := git.CaptureWork(repo, co, req.AgentRun)
	if err != nil {
		writeError(w, err)
		return
	}
	// TODO: 持久化快照；HasUntracked 来自 Status 未跟踪文件。
	writeJSON(w, http.StatusOK, AgentSnapshot{Checkout: co, Head: state.Head, StashOID: oid, AgentRun: req.AgentRun})
}

// GitLatestSnapshot 取最近一份 Agent 快照。
func (a *API) GitLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	_, _, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	// TODO: 取最近一份；没有则空对象。
	writeJSON(w, http.StatusOK, AgentSnapshot{})
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
		writeError(w, err)
		return
	}
	// TODO: 用 Latest / ID 找到 Head 与 StashOID。
	if err := git.RestoreWork(repo, co, "", ""); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GitListUndo 按当前 SiteState 算出能点的撤销按钮。
func (a *API) GitListUndo(w http.ResponseWriter, r *http.Request) {
	repo, co, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	state, err := git.Status(repo, co)
	if err != nil {
		writeError(w, err)
		return
	}
	// TODO: 看 Ahead / Empty / Integrating / 是否有 Agent 快照。
	_ = state
	writeJSON(w, http.StatusOK, map[string]any{"buttons": []UndoButton{}})
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
		writeError(w, err)
		return
	}
	// TODO: 按按钮 Target 调 Reset / Revert / AbortIntegrate / RestoreWork。
	_ = repo
	_ = co
	_ = req
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
