package git

// ListBranches 列出本地和远程跟踪分支，含跟踪和 ahead/behind。
func ListBranches(repo Repo) ([]Branch, error) {
	// TODO: for-each-ref refs/heads 与 refs/remotes，分别填 IsRemote / Upstream / WorktreePath。
	_ = repo
	return []Branch{}, nil
}

// CreateBranch 从起点创建分支；start 空则从当前提交。
func CreateBranch(repo Repo, name, start string) error {
	// TODO: git branch name [start]
	_ = repo
	_ = name
	_ = start
	return nil
}

// SwitchBranch 切换到指定分支。脏工作区、正在整合或该分支已被其他检出占用时拒绝。
func SwitchBranch(repo Repo, checkout Checkout, name string) error {
	// TODO: 先 Status 与 ListBranches，再 git switch。
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	branches, err := ListBranches(repo)
	if err != nil {
		return err
	}
	_ = state
	_ = branches
	_ = name
	return nil
}

// DeleteBranch 删除本地分支；不能删当前分支。
func DeleteBranch(repo Repo, name string) error {
	// TODO: 当前分支返回 ErrCurrentBranch；git branch -d。
	_ = repo
	_ = name
	return nil
}
