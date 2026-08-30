package git

// ListRemotes 列出已配置的 remote。
func ListRemotes(repo Repo) ([]Remote, error) {
	// TODO: git remote -v
	_ = repo
	return []Remote{}, nil
}
