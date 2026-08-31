package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Open 打开本地路径上的仓库句柄，不要求已经是 Git 仓库，不向上找 .git。
func Open(path string) (Repo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Repo{}, err
	}
	return Repo{Path: abs}, nil
}

// Status 读这一份检出的整局：分支、跟踪、文件、remote。不是仓库则 IsRepo=false。
func Status(repo Repo, checkout Checkout) (SiteState, error) {
	dir, err := filepath.Abs(checkoutDir(repo, checkout))
	if err != nil {
		return SiteState{}, err
	}
	state := SiteState{Path: dir, Files: []FileStatus{}, Remotes: []Remote{}}
	if !isRepo(dir) {
		return state, nil
	}
	state.IsRepo = true
	out, err := runGit(dir, "status", "--porcelain=v2", "--branch", "-uall")
	if err != nil {
		return SiteState{}, err
	}
	parseStatusV2(&state, out)
	state.Integrating = integrating(dir)
	state.DefaultBranch = defaultBranch(dir)
	if state.Upstream != "" {
		state.UpstreamGone = upstreamGone(dir, state.Upstream)
	}
	remotes, err := ListRemotes(repo)
	if err != nil {
		return SiteState{}, err
	}
	state.Remotes = remotes
	return state, nil
}

func parseStatusV2(state *SiteState, out string) {
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			oid := strings.TrimSpace(strings.TrimPrefix(line, "# branch.oid "))
			if oid == "(initial)" {
				state.Empty = true
				state.Head = ""
			} else {
				state.Head = oid
			}
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			if head == "(detached)" {
				state.Detached = true
				state.Branch = ""
			} else {
				state.Branch = head
			}
		case strings.HasPrefix(line, "# branch.upstream "):
			state.Upstream = strings.TrimSpace(strings.TrimPrefix(line, "# branch.upstream "))
		case strings.HasPrefix(line, "# branch.ab "):
			raw := strings.TrimSpace(strings.TrimPrefix(line, "# branch.ab "))
			fields := strings.Fields(raw)
			if len(fields) >= 2 {
				state.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
				state.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
			}
		case strings.HasPrefix(line, "1 "):
			if file, ok := parsePorcelain1(line); ok {
				state.Files = append(state.Files, file)
			}
		case strings.HasPrefix(line, "2 "):
			if file, ok := parsePorcelain2(line); ok {
				state.Files = append(state.Files, file)
			}
		case strings.HasPrefix(line, "u "):
			if file, ok := parsePorcelainU(line); ok {
				state.Files = append(state.Files, file)
			}
		case strings.HasPrefix(line, "? "):
			state.Files = append(state.Files, FileStatus{
				Path:           strings.TrimPrefix(line, "? "),
				StagedStatus:   " ",
				WorktreeStatus: "?",
			})
		}
	}
	if state.Empty {
		state.Detached = false
		state.Head = ""
	}
}

func parsePorcelain1(line string) (FileStatus, bool) {
	parts := strings.SplitN(line, " ", 9)
	if len(parts) < 9 || len(parts[1]) < 2 {
		return FileStatus{}, false
	}
	return FileStatus{
		Path:           parts[8],
		StagedStatus:   statusLetter(parts[1][0]),
		WorktreeStatus: statusLetter(parts[1][1]),
	}, true
}

func parsePorcelain2(line string) (FileStatus, bool) {
	parts := strings.SplitN(line, " ", 10)
	if len(parts) < 10 || len(parts[1]) < 2 {
		return FileStatus{}, false
	}
	path, orig, _ := strings.Cut(parts[9], "\t")
	return FileStatus{
		Path:           path,
		OrigPath:       orig,
		StagedStatus:   statusLetter(parts[1][0]),
		WorktreeStatus: statusLetter(parts[1][1]),
	}, true
}

func parsePorcelainU(line string) (FileStatus, bool) {
	parts := strings.SplitN(line, " ", 11)
	if len(parts) < 11 || len(parts[1]) < 2 {
		return FileStatus{}, false
	}
	return FileStatus{
		Path:           parts[10],
		StagedStatus:   statusLetter(parts[1][0]),
		WorktreeStatus: statusLetter(parts[1][1]),
		Unmerged:       true,
	}, true
}

// Diff 读已暂存或工作区差异；scope 为 staged | worktree。
func Diff(repo Repo, checkout Checkout, scope string) ([]DiffFile, error) {
	dir := checkoutDir(repo, checkout)
	if !isRepo(dir) {
		return []DiffFile{}, nil
	}
	if scope == "" {
		scope = "staged"
	}
	if scope != "staged" && scope != "worktree" {
		return nil, fmt.Errorf("scope must be staged or worktree")
	}
	args := []string{"diff", "--name-status", "--find-renames"}
	numArgs := []string{"diff", "--numstat", "--find-renames"}
	patchArgs := []string{"diff", "--find-renames"}
	if scope == "staged" {
		args = append(args, "--cached")
		numArgs = append(numArgs, "--cached")
		patchArgs = append(patchArgs, "--cached")
	}
	nameOut, err := runGitAllow(dir, []int{1}, args...)
	if err != nil {
		return nil, err
	}
	numOut, err := runGitAllow(dir, []int{1}, numArgs...)
	if err != nil {
		return nil, err
	}
	binary := map[string]bool{}
	for _, line := range strings.Split(numOut, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		path := fields[2]
		if strings.Contains(path, " => ") {
			path = strings.TrimSuffix(strings.Split(path, " => ")[1], "}")
			path = strings.TrimPrefix(path, "{")
		}
		binary[path] = fields[0] == "-" && fields[1] == "-"
	}
	files := []DiffFile{}
	for _, line := range strings.Split(nameOut, "\n") {
		if line == "" {
			continue
		}
		file, ok := parseNameStatus(line)
		if !ok {
			continue
		}
		file.Binary = binary[file.Path]
		if !file.Binary {
			patch, err := runGitAllow(dir, []int{1}, append(patchArgs, "--", file.Path)...)
			if err == nil {
				file.Patch = patch
			}
		}
		files = append(files, file)
	}
	if scope == "worktree" {
		state, err := Status(repo, checkout)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, file := range files {
			seen[file.Path] = true
		}
		for _, file := range state.Files {
			if file.WorktreeStatus != "?" || seen[file.Path] {
				continue
			}
			files = append(files, untrackedDiff(dir, file.Path))
		}
	}
	return files, nil
}

func parseNameStatus(line string) (DiffFile, bool) {
	code, rest, ok := strings.Cut(line, "\t")
	if !ok || code == "" {
		return DiffFile{}, false
	}
	kind := "modified"
	switch code[0] {
	case 'A':
		kind = "added"
	case 'M', 'T':
		kind = "modified"
	case 'D':
		kind = "deleted"
	case 'R', 'C':
		kind = "renamed"
	case 'U':
		kind = "unmerged"
	}
	file := DiffFile{Kind: kind}
	if kind == "renamed" {
		orig, path, ok := strings.Cut(rest, "\t")
		if !ok {
			file.Path = rest
			return file, true
		}
		file.OrigPath = orig
		file.Path = path
		return file, true
	}
	file.Path = rest
	return file, true
}

const maxUntrackedPatchBytes = 256 * 1024

func untrackedDiff(dir, rel string) DiffFile {
	file := DiffFile{Path: rel, Kind: "added"}
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return file
	}
	if info.Size() > maxUntrackedPatchBytes {
		file.Binary = true
		return file
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return file
	}
	if bytes.IndexByte(body, 0) >= 0 {
		file.Binary = true
		return file
	}
	patch, err := runGitAllow(dir, []int{1}, "diff", "--no-index", "--", os.DevNull, rel)
	if err == nil && patch != "" {
		file.Patch = patch
		return file
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- /dev/null\n+++ b/%s\n", rel, rel, rel)
	lines := strings.Split(string(body), "\n")
	fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		b.WriteByte('+')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	file.Patch = b.String()
	return file
}

// LogGraph 读近期提交、装饰和父子边。
func LogGraph(repo Repo, limit int) (Graph, error) {
	dir := repo.Path
	if !isRepo(dir) {
		return Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	out, err := runGitAllow(dir, []int{128}, "log", "--all", "--decorate=full", "-n", strconv.Itoa(limit), "--format=%H%x1f%P%x1f%s%x1f%an%x1f%aI%x1f%d%x1f%b%x1e")
	if err != nil {
		return Graph{}, err
	}
	graph := Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, "\x1f", 7)
		if len(parts) < 6 {
			continue
		}
		var parents []string
		if strings.TrimSpace(parts[1]) != "" {
			parents = strings.Fields(parts[1])
		} else {
			parents = []string{}
		}
		body := ""
		if len(parts) > 6 {
			body = strings.TrimSpace(parts[6])
		}
		commit := Commit{
			ID:      strings.TrimSpace(parts[0]),
			Parents: parents,
			Title:   parts[2],
			Body:    body,
			Author:  parts[3],
			Date:    strings.TrimSpace(parts[4]),
		}
		graph.Nodes = append(graph.Nodes, GraphNode{Commit: commit, Refs: parseDecorations(parts[5])})
		for _, parent := range parents {
			graph.Edges = append(graph.Edges, GraphEdge{Child: commit.ID, Parent: parent})
		}
	}
	return graph, nil
}

// Log 读这份检出当前线上的近期提交，从新到旧。不是 --all 的分叉图。
func Log(repo Repo, checkout Checkout, limit int) ([]Commit, error) {
	dir := checkoutDir(repo, checkout)
	if !isRepo(dir) {
		return []Commit{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return nil, err
	}
	if !state.IsRepo || state.Empty {
		return []Commit{}, nil
	}
	out, err := runGitAllow(dir, []int{128}, "log", "-n", strconv.Itoa(limit), "--format=%H%x1f%P%x1f%s%x1f%an%x1f%aI%x1f%b%x1e")
	if err != nil {
		return nil, err
	}
	commits := []Commit{}
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, "\x1f", 6)
		if len(parts) < 5 {
			continue
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
		commits = append(commits, Commit{
			ID:      strings.TrimSpace(parts[0]),
			Parents: parents,
			Title:   parts[2],
			Body:    body,
			Author:  parts[3],
			Date:    strings.TrimSpace(parts[4]),
		})
	}
	return commits, nil
}

func parseDecorations(raw string) []Ref {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "(")
	raw = strings.TrimSuffix(raw, ")")
	if raw == "" {
		return []Ref{}
	}
	refs := []Ref{}
	for _, token := range strings.Split(raw, ", ") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "HEAD -> ") {
			refs = append(refs, Ref{Name: "HEAD", Kind: "head"})
			token = strings.TrimPrefix(token, "HEAD -> ")
		}
		if token == "HEAD" {
			refs = append(refs, Ref{Name: "HEAD", Kind: "head"})
			continue
		}
		if name, ok := strings.CutPrefix(token, "tag: refs/tags/"); ok {
			refs = append(refs, Ref{Name: name, Kind: "tag"})
			continue
		}
		if name, ok := strings.CutPrefix(token, "tag: "); ok {
			refs = append(refs, Ref{Name: strings.TrimPrefix(name, "refs/tags/"), Kind: "tag"})
			continue
		}
		if name, ok := strings.CutPrefix(token, "refs/heads/"); ok {
			refs = append(refs, Ref{Name: name, Kind: "local"})
			continue
		}
		if name, ok := strings.CutPrefix(token, "refs/remotes/"); ok {
			refs = append(refs, Ref{Name: name, Kind: "remote"})
			continue
		}
		if name, ok := strings.CutPrefix(token, "refs/tags/"); ok {
			refs = append(refs, Ref{Name: name, Kind: "tag"})
			continue
		}
	}
	return refs
}

// Stage 把路径加入暂存区。
func Stage(repo Repo, checkout Checkout, paths []string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	clean, err := normalizePaths(paths)
	if err != nil {
		return err
	}
	_, err = runGit(dir, append([]string{"add", "--"}, clean...)...)
	return err
}

// Unstage 把路径移出暂存区。
func Unstage(repo Repo, checkout Checkout, paths []string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	clean, err := normalizePaths(paths)
	if err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Empty {
		_, err = runGit(dir, append([]string{"rm", "--cached", "-q", "--"}, clean...)...)
		return err
	}
	_, err = runGit(dir, append([]string{"restore", "--staged", "--"}, clean...)...)
	return err
}

// CreateCommit 用已经写好的说明创建提交。
func CreateCommit(repo Repo, checkout Checkout, message string) (Commit, error) {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return Commit{}, err
	}
	if strings.TrimSpace(message) == "" {
		return Commit{}, errors.New("message is required")
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return Commit{}, err
	}
	if state.Integrating != "" {
		return Commit{}, ErrIntegrating
	}
	if _, err := runGit(dir, "commit", "-m", message); err != nil {
		return Commit{}, err
	}
	return readCommit(dir, "HEAD")
}

// Reset 软 / 混合 / 硬重置到目标提交。
func Reset(repo Repo, checkout Checkout, target, mode string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Integrating != "" {
		return ErrIntegrating
	}
	if state.Empty {
		return errors.New("empty repository")
	}
	if strings.TrimSpace(target) == "" {
		return errors.New("target is required")
	}
	switch mode {
	case "soft", "mixed", "hard":
	case "":
		mode = "mixed"
	default:
		return fmt.Errorf("invalid reset mode %q", mode)
	}
	_, err = runGit(dir, "reset", "--"+mode, target)
	return err
}

// Revert 用一次新提交回退指定提交。
func Revert(repo Repo, checkout Checkout, commit Commit) (Commit, error) {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return Commit{}, err
	}
	if strings.TrimSpace(commit.ID) == "" {
		return Commit{}, errors.New("commit id is required")
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return Commit{}, err
	}
	if state.Integrating != "" {
		return Commit{}, ErrIntegrating
	}
	if _, err := runGit(dir, "revert", "--no-edit", commit.ID); err != nil {
		after, statusErr := Status(repo, checkout)
		if statusErr == nil && (after.Integrating != "" || hasUnmerged(after)) {
			return Commit{}, ErrConflict
		}
		return Commit{}, err
	}
	return readCommit(dir, "HEAD")
}

// Push 推到已配置的 remote。
func Push(ctx context.Context, repo Repo, checkout Checkout) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Integrating != "" {
		return ErrIntegrating
	}
	if len(state.Remotes) == 0 {
		return errors.New("no remote configured")
	}
	if state.Upstream == "" {
		return errors.New("no upstream")
	}
	if state.UpstreamGone {
		return errors.New("upstream is gone")
	}
	_, err = runGitCtx(ctx, dir, "push")
	return err
}

// Pull 拉取并尝试整合；有冲突返回 ErrConflict。
func Pull(ctx context.Context, repo Repo, checkout Checkout) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Integrating != "" {
		return ErrIntegrating
	}
	if isDirty(state) {
		return ErrDirty
	}
	if len(state.Remotes) == 0 {
		return errors.New("no remote configured")
	}
	if state.Upstream == "" {
		return errors.New("no upstream")
	}
	if state.UpstreamGone {
		return errors.New("upstream is gone")
	}
	if _, err := runGitCtx(ctx, dir, "pull", "--no-rebase"); err != nil {
		after, statusErr := Status(repo, checkout)
		if statusErr == nil && (after.Integrating != "" || hasUnmerged(after)) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// RestorePath 把路径还原成 HEAD 的暂存区和工作区内容。
func RestorePath(repo Repo, checkout Checkout, path string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	clean, err := normalizePaths([]string{path})
	if err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	if state.Empty {
		return errors.New("empty repository")
	}
	_, err = runGit(dir, "restore", "--source=HEAD", "--staged", "--worktree", "--", clean[0])
	return err
}

// Discard 丢掉工作区改动：已跟踪的还原成暂存区；未跟踪的删除。不改暂存区。
func Discard(repo Repo, checkout Checkout, paths []string) error {
	dir := checkoutDir(repo, checkout)
	if err := requireRepo(dir); err != nil {
		return err
	}
	clean, err := normalizePaths(paths)
	if err != nil {
		return err
	}
	state, err := Status(repo, checkout)
	if err != nil {
		return err
	}
	byPath := make(map[string]FileStatus, len(state.Files))
	for _, file := range state.Files {
		byPath[file.Path] = file
	}
	var tracked []string
	var untracked []string
	for _, path := range clean {
		if path == ".git" || strings.HasPrefix(path, ".git/") {
			return fmt.Errorf("invalid path %q", path)
		}
		file, ok := byPath[path]
		if !ok {
			continue
		}
		if file.Unmerged {
			return ErrConflict
		}
		if file.WorktreeStatus == "?" {
			untracked = append(untracked, path)
			continue
		}
		if letterDirty(file.WorktreeStatus) {
			tracked = append(tracked, path)
		}
	}
	if len(tracked) > 0 {
		if _, err := runGit(dir, append([]string{"restore", "--worktree", "--"}, tracked...)...); err != nil {
			return err
		}
	}
	for _, path := range untracked {
		if err := removeUntracked(dir, path); err != nil {
			return err
		}
	}
	return nil
}

func removeUntracked(dir, rel string) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	abs := filepath.Join(root, filepath.FromSlash(rel))
	back, err := filepath.Rel(root, abs)
	if err != nil || back == "." || strings.HasPrefix(back, "..") {
		return fmt.Errorf("invalid path %q", rel)
	}
	if err := os.RemoveAll(abs); err != nil {
		return err
	}
	sep := string(os.PathSeparator)
	for parent := filepath.Dir(abs); parent != root && strings.HasPrefix(parent, root+sep); parent = filepath.Dir(parent) {
		if err := os.Remove(parent); err != nil { //nolint:nilerr // 目录非空时停止上收
			break
		}
	}
	return nil
}
