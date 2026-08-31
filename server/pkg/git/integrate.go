package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ReadConflict 读某文件的冲突种类和三方内容。
func ReadConflict(repo Repo, checkout Checkout, path string) (ConflictItem, error) {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return ConflictItem{}, err
	}
	clean, err := normalizePaths([]string{path})
	if err != nil {
		return ConflictItem{}, err
	}
	rel := clean[0]
	out, err := runGit(dir, "ls-files", "-u", "--", rel)
	if err != nil {
		return ConflictItem{}, err
	}
	stages := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		switch fields[2] {
		case "1":
			stages[1] = true
		case "2":
			stages[2] = true
		case "3":
			stages[3] = true
		}
	}
	item := ConflictItem{
		Path:   rel,
		Kind:   conflictKind(stages),
		Base:   showStage(dir, 1, rel),
		Ours:   showStage(dir, 2, rel),
		Theirs: showStage(dir, 3, rel),
	}
	return item, nil
}

func conflictKind(stages map[int]bool) string {
	switch {
	case stages[1] && stages[2] && stages[3]:
		return "both_modified"
	case stages[1] && stages[2] && !stages[3]:
		return "deleted_by_them"
	case stages[1] && !stages[2] && stages[3]:
		return "deleted_by_us"
	case !stages[1] && stages[2] && stages[3]:
		return "both_added"
	case stages[1] && !stages[2] && !stages[3]:
		return "both_deleted"
	case !stages[1] && stages[2] && !stages[3]:
		return "added_by_us"
	case !stages[1] && !stages[2] && stages[3]:
		return "added_by_them"
	default:
		return "both_modified"
	}
}

func showStage(dir string, stage int, path string) string {
	out, err := runGit(dir, "show", ":"+itoa(stage)+":"+path)
	if err != nil {
		return ""
	}
	return out
}

func itoa(n int) string {
	return string(rune('0' + n))
}

// ConflictNames 读当前整合双方的可读名，给对比视图用。
func ConflictNames(repo Repo, checkout Checkout) (ours, theirs string, err error) {
	dir := checkoutDir(repo, checkout)
	if !isRepo(dir) {
		return "", "", nil
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return "", "", err
	}
	ours = state.Branch
	if ours == "" {
		ours = "HEAD"
	}
	gd, err := gitDir(dir)
	if err != nil {
		return ours, "", nil //nolint:nilerr // rebase onto 可选，没有 git 目录仍返回 ours
	}
	switch state.Integrating {
	case "merge":
		theirs = nameOf(dir, "MERGE_HEAD")
	case "rebase":
		theirs = nameOf(dir, "REBASE_HEAD")
		if onto, readErr := os.ReadFile(filepath.Join(gd, "rebase-merge", "onto")); readErr == nil {
			ours = nameOf(dir, strings.TrimSpace(string(onto)))
		} else if onto, readErr := os.ReadFile(filepath.Join(gd, "rebase-apply", "onto")); readErr == nil {
			ours = nameOf(dir, strings.TrimSpace(string(onto)))
		}
	case "cherry_pick":
		theirs = nameOf(dir, "CHERRY_PICK_HEAD")
	case "revert":
		theirs = nameOf(dir, "REVERT_HEAD")
	}
	return ours, theirs, nil
}

// ContinueIntegrate 继续未完成的整合。
func ContinueIntegrate(repo Repo, checkout Checkout) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Integrating == "" {
		return errors.New("not integrating")
	}
	if hasUnmerged(state) {
		return ErrConflict
	}
	var args []string
	switch state.Integrating {
	case "merge":
		args = []string{"merge", "--continue"}
	case "rebase":
		args = []string{"rebase", "--continue"}
	case "cherry_pick":
		args = []string{"cherry-pick", "--continue"}
	case "revert":
		args = []string{"revert", "--continue"}
	default:
		return errors.New("unknown integrate kind")
	}
	_, err = runGit(dir, args...)
	return err
}

// AbortIntegrate 中止未完成的整合。
func AbortIntegrate(repo Repo, checkout Checkout) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Integrating == "" {
		return errors.New("not integrating")
	}
	var args []string
	switch state.Integrating {
	case "merge":
		args = []string{"merge", "--abort"}
	case "rebase":
		args = []string{"rebase", "--abort"}
	case "cherry_pick":
		args = []string{"cherry-pick", "--abort"}
	case "revert":
		args = []string{"revert", "--abort"}
	default:
		return errors.New("unknown integrate kind")
	}
	_, err = runGit(dir, args...)
	return err
}
