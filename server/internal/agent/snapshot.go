package agent

import (
	"errors"
	"strings"
	"time"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/git"
)

// gitSnapshot 用本机 Git 实现快照口。
type gitSnapshot struct{}

// Take 在首次写操作前用 stash create 拍轻量快照。
func (gitSnapshot) Take(workspaceRoot, runID string) (pkgagent.WorkspaceSnapshot, error) {
	repo, checkout, err := openWorkspace(workspaceRoot)
	if err != nil {
		return pkgagent.WorkspaceSnapshot{}, err
	}
	state, err := git.Status(repo, checkout)
	if err != nil {
		return pkgagent.WorkspaceSnapshot{}, err
	}
	if !state.IsRepo {
		return pkgagent.WorkspaceSnapshot{}, errors.New("当前目录不是 Git 仓库，无法记录快照")
	}
	untracked, err := git.ListUntracked(repo, checkout)
	if err != nil {
		return pkgagent.WorkspaceSnapshot{}, err
	}
	oid, err := git.CaptureWork(repo, checkout, "codedock-run-"+runID)
	if err != nil {
		return pkgagent.WorkspaceSnapshot{}, err
	}
	return pkgagent.WorkspaceSnapshot{
		SnapshotOID:    oid,
		Head:           state.Head,
		UntrackedFiles: untracked,
		RunID:          runID,
		CreatedAt:      time.Now().UTC().UnixMilli(),
	}, nil
}

// Restore 按快照还原工作区并清掉后来新增的未跟踪文件。
func (gitSnapshot) Restore(workspaceRoot string, snap pkgagent.WorkspaceSnapshot, mode pkgagent.RestoreMode) error {
	if mode == pkgagent.RestoreMessages {
		return nil
	}
	repo, checkout, err := openWorkspace(workspaceRoot)
	if err != nil {
		return err
	}
	head := snap.Head
	if head == "" {
		state, statusErr := git.Status(repo, checkout)
		if statusErr != nil {
			return statusErr
		}
		head = state.Head
	}
	if err := git.RestoreWork(repo, checkout, snap.SnapshotOID, head); err != nil {
		return err
	}
	return git.RemoveExtraUntracked(repo, checkout, snap.UntrackedFiles)
}

// ChangedFiles 列出相对快照的已跟踪改动与新增未跟踪路径。
func (gitSnapshot) ChangedFiles(workspaceRoot string, snap pkgagent.WorkspaceSnapshot) ([]string, error) {
	repo, checkout, err := openWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	names, err := git.DiffNamesAgainst(repo, checkout, snapshotBase(snap))
	if err != nil {
		return nil, err
	}
	added, err := git.ListNewUntracked(repo, checkout, snap.UntrackedFiles)
	if err != nil {
		return nil, err
	}
	return append(names, added...), nil
}

// Diff 读相对快照的 unified diff，并把快照后新出现的未跟踪文件拼进去。
func (gitSnapshot) Diff(workspaceRoot string, snap pkgagent.WorkspaceSnapshot) (string, error) {
	repo, checkout, err := openWorkspace(workspaceRoot)
	if err != nil {
		return "", err
	}
	tracked, err := git.DiffTextAgainst(repo, checkout, snapshotBase(snap))
	if err != nil {
		return "", err
	}
	untracked, err := git.DiffUntrackedSince(repo, checkout, snap.UntrackedFiles)
	if err != nil {
		return "", err
	}
	tracked = strings.TrimRight(tracked, "\n")
	untracked = strings.TrimRight(untracked, "\n")
	if tracked == "" {
		return untracked, nil
	}
	if untracked == "" {
		return tracked, nil
	}
	return tracked + "\n" + untracked, nil
}

// snapshotBase 优先用 stash 对象，否则用拍快照时的 HEAD。
func snapshotBase(snap pkgagent.WorkspaceSnapshot) string {
	if strings.TrimSpace(snap.SnapshotOID) != "" {
		return snap.SnapshotOID
	}
	return strings.TrimSpace(snap.Head)
}

// openWorkspace 打开会话冻结目录上的 Git 句柄。
func openWorkspace(workspaceRoot string) (git.Repo, git.Checkout, error) {
	repo, err := git.Open(workspaceRoot)
	if err != nil {
		return git.Repo{}, git.Checkout{}, err
	}
	return repo, git.Checkout{Path: repo.Path}, nil
}
