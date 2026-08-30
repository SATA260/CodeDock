package git

// CaptureWork 用 stash create 复制暂存区与已跟踪工作区，不改现有内容；返回悬空提交 hash。
func CaptureWork(repo Repo, checkout Checkout, note string) (string, error) {
	// TODO: git stash create；不含未跟踪文件。
	state, err := Status(repo, checkout)
	if err != nil {
		return "", err
	}
	_ = state
	_ = note
	return "", nil
}

// RestoreWork 回到指定提交并铺回该副本。
func RestoreWork(repo Repo, checkout Checkout, stashOID, head string) error {
	// TODO: 先回到 head，再把 stashOID 铺回工作区；未跟踪文件不会出现。
	_ = repo
	_ = checkout
	_ = stashOID
	_ = head
	return nil
}
