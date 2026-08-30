package git

// Open 打开本地路径上的仓库句柄，不要求已经是 Git 仓库。
func Open(path string) (Repo, error) {
	// TODO: 规范化绝对路径，不向上找 .git。
	_ = path
	return Repo{}, nil
}

// Status 读这一份检出的整局：分支、跟踪、文件、remote。
func Status(repo Repo, checkout Checkout) (SiteState, error) {
	// TODO: git status --porcelain=v2 --branch；填 Empty / Detached / Upstream / Ahead / Behind / Integrating。
	_, _ = ListRemotes(repo)
	return SiteState{Path: checkoutDir(repo, checkout), Files: []FileStatus{}, Remotes: []Remote{}}, nil
}

// Diff 读已暂存或工作区差异；scope 为 staged | worktree。
func Diff(repo Repo, checkout Checkout, scope string) ([]DiffFile, error) {
	// TODO: git diff / git diff --cached，二进制不要当文本。
	_ = repo
	_ = checkout
	_ = scope
	return []DiffFile{}, nil
}

// LogGraph 读近期提交、装饰和父子边。
func LogGraph(repo Repo, limit int) (Graph, error) {
	// TODO: git log --all --decorate，只取近期 limit。
	_ = repo
	_ = limit
	return Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}}, nil
}

// Stage 把路径加入暂存区。
func Stage(repo Repo, checkout Checkout, paths []string) error {
	// TODO: git add -- paths
	_ = repo
	_ = checkout
	_ = paths
	return nil
}

// Unstage 把路径移出暂存区。
func Unstage(repo Repo, checkout Checkout, paths []string) error {
	// TODO: git reset HEAD -- paths；空仓用 git rm --cached。
	_ = repo
	_ = checkout
	_ = paths
	return nil
}

// CreateCommit 用已经写好的说明创建提交。
func CreateCommit(repo Repo, checkout Checkout, message string) (Commit, error) {
	// TODO: git commit -m；读 HEAD 的 id / parents / 作者 / 时间。
	_ = repo
	_ = checkout
	_ = message
	return Commit{}, nil
}

// Reset 软 / 混合 / 硬重置到目标提交。
func Reset(repo Repo, checkout Checkout, target, mode string) error {
	// TODO: 正在整合则拒绝；git reset --soft|--mixed|--hard。
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	_ = state
	_ = target
	_ = mode
	return nil
}

// Revert 用一次新提交回退指定提交。
func Revert(repo Repo, checkout Checkout, commit Commit) (Commit, error) {
	// TODO: git revert；冲突则返回 ErrConflict。
	_ = repo
	_ = checkout
	_ = commit
	return Commit{}, nil
}

// Push 推到已配置的 remote。
func Push(repo Repo, checkout Checkout) error {
	// TODO: 无 remote / 无 upstream / gone 时返回可读错误；git push。
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	_ = state
	return nil
}

// Pull 拉取并尝试整合；有冲突返回 ErrConflict。
func Pull(repo Repo, checkout Checkout) error {
	// TODO: git pull --no-rebase；失败再看 Integrating / ReadConflict。
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	_ = state
	_, _ = ReadConflict(repo, checkout, "")
	return nil
}
