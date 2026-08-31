package git

import "strings"

// ListRemotes 列出已配置的 remote。
func ListRemotes(repo Repo) ([]Remote, error) {
	dir := repo.Path
	if !isRepo(dir) {
		return []Remote{}, nil
	}
	out, err := runGit(dir, "remote", "-v")
	if err != nil {
		return nil, err
	}
	byName := map[string]*Remote{}
	order := []string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name, url, kind := fields[0], fields[1], strings.Trim(fields[2], "()")
		item, ok := byName[name]
		if !ok {
			item = &Remote{Name: name, FetchURL: url, PushURL: url}
			byName[name] = item
			order = append(order, name)
		}
		switch kind {
		case "fetch":
			item.FetchURL = url
		case "push":
			item.PushURL = url
		}
	}
	remotes := make([]Remote, 0, len(order))
	for _, name := range order {
		remotes = append(remotes, *byName[name])
	}
	return remotes, nil
}
