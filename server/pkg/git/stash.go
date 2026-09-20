package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CaptureWork 用 stash create 复制暂存区与已跟踪工作区，不改现有内容；干净树回落 HEAD。
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
	oid := strings.TrimSpace(out)
	if oid != "" {
		return oid, nil
	}
	head, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
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
	if strings.TrimSpace(stashOID) != "" {
		if _, err := runGit(dir, "rev-parse", "--verify", "--quiet", stashOID+"^{commit}"); err != nil {
			return fmt.Errorf("snapshot %s is unavailable", stashOID)
		}
	}
	if _, err := runGit(dir, "reset", "--hard", head); err != nil {
		return err
	}
	if strings.TrimSpace(stashOID) == "" || stashOID == head {
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

// ListUntracked 列出工作区未跟踪文件（不含忽略）。
func ListUntracked(repo Repo, checkout Checkout) ([]string, error) {
	state, err := Status(repo, checkout)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, file := range state.Files {
		if file.WorktreeStatus == "?" || file.StagedStatus == "?" {
			out = append(out, file.Path)
		}
	}
	return out, nil
}

// DiffNamesAgainst 列出相对某次提交的已跟踪改动路径。
func DiffNamesAgainst(repo Repo, checkout Checkout, oid string) ([]string, error) {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	if strings.TrimSpace(oid) == "" {
		return nil, nil
	}
	out, err := runGitAllow(dir, []int{1}, "diff", "--name-only", oid)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// DiffTextAgainst 读相对某次提交的 unified diff。
func DiffTextAgainst(repo Repo, checkout Checkout, oid string) (string, error) {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return "", err
	}
	if strings.TrimSpace(oid) == "" {
		return "", nil
	}
	tracked, err := runGitAllow(dir, []int{1}, "diff", oid)
	if err != nil {
		return "", err
	}
	return tracked, nil
}

// DiffUntrackedSince 把快照之后新出现的未跟踪文件拼成 unified diff。
func DiffUntrackedSince(repo Repo, checkout Checkout, keep []string) (string, error) {
	added, err := ListNewUntracked(repo, checkout, keep)
	if err != nil {
		return "", err
	}
	dir := checkoutDir(repo, checkout)
	var parts []string
	for _, rel := range added {
		body, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		if strings.IndexByte(string(body), 0) >= 0 {
			continue
		}
		parts = append(parts, newFileDiff(rel, string(body)))
	}
	return strings.Join(parts, ""), nil
}

// ListNewUntracked 列出相对快照新增的未跟踪路径。
func ListNewUntracked(repo Repo, checkout Checkout, keep []string) ([]string, error) {
	current, err := ListUntracked(repo, checkout)
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for _, item := range keep {
		allowed[item] = struct{}{}
	}
	var added []string
	for _, path := range current {
		if _, ok := allowed[path]; ok {
			continue
		}
		added = append(added, path)
	}
	return added, nil
}

// newFileDiff 把整文件写成新增文件的 unified diff。
func newFileDiff(rel, body string) string {
	rel = strings.ReplaceAll(rel, "\\", "/")
	var lines []string
	if body != "" {
		lines = strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n", rel, rel, rel)
	if len(lines) == 0 {
		b.WriteString("@@ -0,0 +0,0 @@\n")
		return b.String()
	}
	fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		b.WriteString("+")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// RemoveExtraUntracked 删除快照之后新出现的未跟踪文件。
func RemoveExtraUntracked(repo Repo, checkout Checkout, keep []string) error {
	current, err := ListUntracked(repo, checkout)
	if err != nil {
		return err
	}
	allowed := map[string]struct{}{}
	for _, item := range keep {
		allowed[item] = struct{}{}
	}
	dir := checkoutDir(repo, checkout)
	for _, path := range current {
		if _, ok := allowed[path]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, path)); err != nil {
			return err
		}
	}
	return nil
}
