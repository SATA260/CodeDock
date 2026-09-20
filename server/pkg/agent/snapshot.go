package agent

// RestoreMode 快照回滚粒度。
type RestoreMode string

const (
	RestoreFiles    RestoreMode = "restore_files"    // 只还原工作区，保留对话
	RestoreMessages RestoreMode = "restore_messages" // 只截断失败消息，不动代码
	RestoreAll      RestoreMode = "restore_all"      // 代码与对话一起回到动手前
)

// WorkspaceSnapshot 任务动手前的工作区轻量快照。
type WorkspaceSnapshot struct {
	SnapshotOID    string   `json:"snapshot_oid"`              // git stash create 产出的悬空提交
	Head           string   `json:"head,omitempty"`            // 拍快照时的 HEAD
	UntrackedFiles []string `json:"untracked_files,omitempty"` // 当时已有的未跟踪文件
	RunID          string   `json:"run_id"`
	CreatedAt      int64    `json:"created_at"`
}

// SnapshotPort 由运行时注入的 Git 快照能力；pkg/agent 不直接调 git。
type SnapshotPort interface {
	Take(workspaceRoot, runID string) (WorkspaceSnapshot, error)
	Restore(workspaceRoot string, snap WorkspaceSnapshot, mode RestoreMode) error
	ChangedFiles(workspaceRoot string, snap WorkspaceSnapshot) ([]string, error)
	Diff(workspaceRoot string, snap WorkspaceSnapshot) (string, error)
}

// SnapshotFromState 用状态里已保存的字段拼回快照对象。
func SnapshotFromState(state AgentState) WorkspaceSnapshot {
	return WorkspaceSnapshot{
		SnapshotOID:    state.SnapshotID,
		Head:           state.SnapshotHead,
		UntrackedFiles: append([]string(nil), state.UntrackedFiles...),
		RunID:          state.RunID,
	}
}
