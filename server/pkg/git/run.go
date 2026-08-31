package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	if strings.TrimSpace(checkout.Path) != "" {
		return checkout.Path
	}
	return repo.Path
}

func isRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func requireRepo(dir string) error {
	if !isRepo(dir) {
		return ErrNotRepo
	}
	return nil
}

type gitOutput struct {
	stdout string
	stderr string
	code   int
	err    error
}

func runGitCmd(dir string, args ...string) gitOutput {
	return runGitCmdCtx(context.Background(), dir, args...)
}

func runGitCmdCtx(ctx context.Context, dir string, args ...string) gitOutput {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true", "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := gitOutput{stdout: stdout.String(), stderr: stderr.String(), err: err}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			out.err = fmt.Errorf("git %s timed out", strings.Join(args, " "))
			out.code = -1
			return out
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			out.code = ee.ExitCode()
		} else {
			out.code = -1
		}
	}
	return out
}

func gitErr(out gitOutput) error {
	if out.err == nil {
		return nil
	}
	msg := strings.TrimSpace(out.stderr)
	if msg == "" {
		msg = strings.TrimSpace(out.stdout)
	}
	if msg == "" {
		msg = out.err.Error()
	}
	return errors.New(msg)
}

func runGit(dir string, args ...string) (string, error) {
	return runGitCtx(context.Background(), dir, args...)
}

func runGitCtx(ctx context.Context, dir string, args ...string) (string, error) {
	out := runGitCmdCtx(ctx, dir, args...)
	if out.err != nil {
		return "", gitErr(out)
	}
	return out.stdout, nil
}

func runGitAllow(dir string, codes []int, args ...string) (string, error) {
	out := runGitCmd(dir, args...)
	if out.err == nil {
		return out.stdout, nil
	}
	for _, code := range codes {
		if out.code == code {
			return out.stdout, nil
		}
	}
	return "", gitErr(out)
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

// CommonDir 返回主仓 git 目录（各 worktree 共享）。
func CommonDir(repo Repo) (string, error) {
	if !isRepo(repo.Path) {
		return "", ErrNotRepo
	}
	out, err := runGit(repo.Path, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(repo.Path, p)
	}
	return filepath.Clean(p), nil
}

func canonPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(resolved)
}

func samePath(a, b string) bool {
	return canonPath(a) == canonPath(b)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizePaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		path = strings.TrimPrefix(path, "./")
		if path == "" {
			continue
		}
		cleaned := filepath.ToSlash(filepath.Clean(path))
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") || filepath.IsAbs(path) || strings.HasPrefix(cleaned, "/") {
			return nil, fmt.Errorf("invalid path %q", path)
		}
		out = append(out, cleaned)
	}
	if len(out) == 0 {
		return nil, errors.New("paths required")
	}
	return out, nil
}

func statusLetter(ch byte) string {
	if ch == '.' {
		return " "
	}
	return string(ch)
}

func letterDirty(letter string) bool {
	return letter != "" && letter != " " && letter != "."
}

func isDirty(state SiteState) bool {
	for _, file := range state.Files {
		if file.Unmerged {
			return true
		}
		if letterDirty(file.StagedStatus) {
			return true
		}
		if letterDirty(file.WorktreeStatus) && file.WorktreeStatus != "?" {
			return true
		}
	}
	return false
}

func hasUnmerged(state SiteState) bool {
	for _, file := range state.Files {
		if file.Unmerged {
			return true
		}
	}
	return false
}

func parseTrack(raw string) (ahead, behind int, gone bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "[]")
	if raw == "" {
		return 0, 0, false
	}
	if raw == "gone" {
		return 0, 0, true
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "ahead "):
			ahead, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "ahead ")))
		case strings.HasPrefix(part, "behind "):
			behind, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "behind ")))
		case part == "gone":
			gone = true
		}
	}
	return ahead, behind, gone
}

func integrating(dir string) string {
	gd, err := gitDir(dir)
	if err != nil {
		return ""
	}
	switch {
	case fileExists(filepath.Join(gd, "rebase-merge")) || fileExists(filepath.Join(gd, "rebase-apply")) || fileExists(filepath.Join(gd, "REBASE_HEAD")):
		return "rebase"
	case fileExists(filepath.Join(gd, "CHERRY_PICK_HEAD")):
		return "cherry_pick"
	case fileExists(filepath.Join(gd, "REVERT_HEAD")):
		return "revert"
	case fileExists(filepath.Join(gd, "MERGE_HEAD")):
		return "merge"
	default:
		return ""
	}
}

func readCommit(dir, rev string) (Commit, error) {
	out, err := runGit(dir, "log", "-1", "--format=%H%x1f%P%x1f%s%x1f%an%x1f%aI%x1f%b", rev, "--")
	if err != nil {
		return Commit{}, err
	}
	parts := strings.SplitN(strings.TrimSuffix(out, "\n"), "\x1f", 6)
	if len(parts) < 5 {
		return Commit{}, fmt.Errorf("cannot parse commit %s", rev)
	}
	var parents []string
	if strings.TrimSpace(parts[1]) != "" {
		parents = strings.Fields(parts[1])
	} else {
		parents = []string{}
	}
	body := ""
	if len(parts) > 5 {
		body = strings.TrimSpace(parts[5])
	}
	return Commit{
		ID:      strings.TrimSpace(parts[0]),
		Parents: parents,
		Title:   parts[2],
		Body:    body,
		Author:  parts[3],
		Date:    strings.TrimSpace(parts[4]),
	}, nil
}

func nameOf(dir, rev string) string {
	out, err := runGit(dir, "name-rev", "--name-only", "--no-undefined", rev)
	if err != nil {
		out, err = runGit(dir, "rev-parse", "--short", rev)
		if err != nil {
			return rev
		}
	}
	return strings.TrimSpace(out)
}

func defaultBranch(dir string) string {
	out, err := runGit(dir, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(out)
	return strings.TrimPrefix(ref, "refs/remotes/origin/")
}

func upstreamGone(dir, upstream string) bool {
	if strings.TrimSpace(upstream) == "" {
		return false
	}
	_, err := runGit(dir, "rev-parse", "--verify", "--quiet", "refs/remotes/"+upstream)
	return err != nil
}
