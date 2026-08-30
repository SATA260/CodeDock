package git

import (
	"errors"
	"path/filepath"
	"strings"
)

// ListWorktrees 列出该仓库的全部检出。
func ListWorktrees(repo Repo) ([]Worktree, error) {
	if !isRepo(repo.Path) {
		return []Worktree{}, nil
	}
	out, err := runGit(repo.Path, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	trees := []Worktree{}
	var current Worktree
	flush := func() {
		if current.Path == "" {
			return
		}
		trees = append(trees, current)
		current = Worktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "detached":
			current.Detached = true
			current.Branch = ""
		case strings.HasPrefix(line, "locked"):
			current.Locked = true
		case line == "":
			flush()
		}
	}
	flush()
	return trees, nil
}

// AddWorktree 创建一份检出。newBranch 空则挂到已有分支。
func AddWorktree(repo Repo, path, branch, newBranch string) (Worktree, error) {
	if err := requireRepo(repo.Path); err != nil {
		return Worktree{}, err
	}
	if strings.TrimSpace(path) == "" {
		return Worktree{}, errors.New("worktree path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Worktree{}, err
	}
	args := []string{"worktree", "add"}
	if strings.TrimSpace(newBranch) != "" {
		args = append(args, "-b", newBranch, abs)
		if strings.TrimSpace(branch) != "" {
			args = append(args, branch)
		}
	} else {
		if strings.TrimSpace(branch) == "" {
			return Worktree{}, errors.New("branch is required")
		}
		args = append(args, abs, branch)
	}
	if _, err := runGit(repo.Path, args...); err != nil {
		return Worktree{}, err
	}
	trees, err := ListWorktrees(repo)
	if err != nil {
		return Worktree{}, err
	}
	for _, tree := range trees {
		if samePath(tree.Path, abs) {
			return tree, nil
		}
	}
	return Worktree{Path: abs, Branch: firstNonEmpty(newBranch, branch)}, nil
}
