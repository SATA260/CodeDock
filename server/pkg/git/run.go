package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrNotRepo 表示路径还不是 Git 仓库。
	ErrNotRepo = errors.New("not a git repository")
	// ErrDirty 表示工作区不干净。
	ErrDirty = errors.New("dirty worktree")
	// ErrIntegrating 表示正在合并或变基。
	ErrIntegrating = errors.New("integrating")
	// ErrConflict 表示推拉或整合产生了未解决冲突。
	ErrConflict = errors.New("conflict")
	// ErrCurrentBranch 表示不能删除当前分支。
	ErrCurrentBranch = errors.New("cannot delete current branch")
)

func checkoutDir(repo Repo, checkout Checkout) string {
	if checkout.Path != "" {
		return checkout.Path
	}
	return repo.Path
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true", "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = strings.TrimSpace(stdout.String())
			}
			if msg == "" {
				msg = err.Error()
			}
			return "", errors.New(msg)
		}
		return stdout.String(), nil
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("git %s timed out", strings.Join(args, " "))
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func gitDir(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return filepath.Clean(p), nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}
