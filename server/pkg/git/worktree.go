package git

// ListWorktrees 列出该仓库的全部检出。
func ListWorktrees(repo Repo) ([]Worktree, error) {
	// TODO: git worktree list --porcelain；填 Detached / Locked。
	_ = repo
	return []Worktree{}, nil
}

// AddWorktree 创建一份检出。newBranch 空则挂到已有分支。
func AddWorktree(repo Repo, path, branch, newBranch string) (Worktree, error) {
	// TODO: git worktree add [-b newBranch] path [branch]
	_ = path
	_ = branch
	_ = newBranch
	trees, err := ListWorktrees(repo)
	if err != nil {
		return Worktree{}, err
	}
	_ = trees
	return Worktree{Path: path, Branch: firstNonEmpty(newBranch, branch)}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
