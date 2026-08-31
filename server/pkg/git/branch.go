package git

import (
	"errors"
	"fmt"
	"strings"
)

// ListBranches 列出本地和远程跟踪分支，含跟踪和 ahead/behind。
func ListBranches(repo Repo) ([]Branch, error) {
	if !isRepo(repo.Path) {
		return []Branch{}, nil
	}
	out, err := runGit(repo.Path, "for-each-ref",
		"--format=%(objectname)%00%(refname)%00%(upstream:short)%00%(upstream:track)%00%(contents:subject)%00%(HEAD)",
		"refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	trees, err := ListWorktrees(repo)
	if err != nil {
		return nil, err
	}
	occupied := map[string]string{}
	for _, tree := range trees {
		if tree.Branch != "" {
			occupied[tree.Branch] = tree.Path
		}
	}
	branches := []Branch{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 6 {
			continue
		}
		refname := parts[1]
		branch := Branch{
			Head:  parts[0],
			Title: parts[4],
		}
		switch {
		case strings.HasPrefix(refname, "refs/heads/"):
			branch.Name = strings.TrimPrefix(refname, "refs/heads/")
			branch.IsRemote = false
			branch.Upstream = parts[2]
			branch.Ahead, branch.Behind, branch.UpstreamGone = parseTrack(parts[3])
			branch.IsCurrent = parts[5] == "*"
			branch.WorktreePath = occupied[branch.Name]
		case strings.HasPrefix(refname, "refs/remotes/"):
			name := strings.TrimPrefix(refname, "refs/remotes/")
			if strings.HasSuffix(name, "/HEAD") {
				continue
			}
			branch.Name = name
			branch.IsRemote = true
		default:
			continue
		}
		branches = append(branches, branch)
	}
	return branches, nil
}

// CreateBranch 从起点创建分支；start 空则从当前检出的 HEAD。
func CreateBranch(repo Repo, checkout Checkout, name, start string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("branch name is required")
	}
	args := []string{"branch", name}
	if strings.TrimSpace(start) != "" {
		args = append(args, start)
	}
	_, err := runGit(dir, args...)
	return err
}

// SwitchBranch 切换到指定分支。脏工作区、正在整合或该分支已被其他检出占用时拒绝。
func SwitchBranch(repo Repo, checkout Checkout, name string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errEmptyName()
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Integrating != "" {
		return ErrIntegrating
	}
	if isDirty(state) {
		return ErrDirty
	}
	branches, err := ListBranches(repo)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch.Name != name || branch.IsRemote {
			continue
		}
		if branch.WorktreePath != "" && !samePath(branch.WorktreePath, dir) {
			return errBranchBusy(name, branch.WorktreePath)
		}
	}
	_, err = runGit(dir, "switch", name)
	return err
}

// DeleteBranch 删除本地分支；不能删当前分支。
func DeleteBranch(repo Repo, name string) error {
	if err := requireRepo(repo.Path); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errEmptyName()
	}
	state, err := Status(repo, Checkout{Path: repo.Path})
	if err != nil {
		return err
	}
	if state.Branch == name {
		return ErrCurrentBranch
	}
	_, err = runGit(repo.Path, "branch", "-d", name)
	return err
}

func errEmptyName() error {
	return errors.New("branch name is required")
}

func errBranchBusy(name, path string) error {
	return fmt.Errorf("branch %s is checked out at %s", name, path)
}
