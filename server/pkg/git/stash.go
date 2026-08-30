package git

import (
	"errors"
	"strings"
)

// CaptureWork 用 stash create 复制暂存区与已跟踪工作区，不改现有内容；返回悬空提交 hash。
func CaptureWork(repo Repo, checkout Checkout, note string) (string, error) {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return "", err
	}
	args := []string{"stash", "create"}
	if strings.TrimSpace(note) != "" {
		args = append(args, note)
	}
	out, err := runGit(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// RestoreWork 回到指定提交并铺回该副本。
func RestoreWork(repo Repo, checkout Checkout, stashOID, head string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	if strings.TrimSpace(head) == "" {
		return errors.New("head is required")
	}
	if _, err := runGit(dir, "reset", "--hard", head); err != nil {
		return err
	}
	if strings.TrimSpace(stashOID) == "" {
		return nil
	}
	if _, err := runGit(dir, "stash", "apply", "--index", stashOID); err != nil {
		after, statusErr := Status(repo, checkout)
		if statusErr == nil && hasUnmerged(after) {
			return ErrConflict
		}
		return err
	}
	return nil
}
