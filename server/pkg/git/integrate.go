package git

// ReadConflict 读某文件的冲突种类和三方内容。
func ReadConflict(repo Repo, checkout Checkout, path string) (ConflictItem, error) {
	// TODO: ls-files -u 判断 Kind；git show :1/:2/:3:path。
	_ = repo
	_ = checkout
	return ConflictItem{Path: path}, nil
}

// ContinueIntegrate 继续未完成的整合。
func ContinueIntegrate(repo Repo, checkout Checkout) error {
	// TODO: 仍有 Unmerged 则 ErrConflict；按 Integrating 跑 merge/rebase/cherry-pick/revert --continue。
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	_ = state
	return nil
}

// AbortIntegrate 中止未完成的整合。
func AbortIntegrate(repo Repo, checkout Checkout) error {
	// TODO: 按 Integrating 跑 --abort。
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	_ = state
	return nil
}
