package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Issue 是 gh issue view 的只读快照。
type Issue struct {
	Repo   string `json:"repo"`
	Number int64  `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Body   string `json:"body"`
	State  string `json:"state"`
}

// Pull 是 gh pr view 的只读快照。
type Pull struct {
	Repo   string `json:"repo"`
	Number int64  `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Body   string `json:"body"`
	State  string `json:"state"`
}

// ViewIssue 用本机 gh 读一条 Issue，不落 token。
func ViewIssue(ctx context.Context, repo string, number int64) (Issue, error) {
	raw, err := runGH(ctx, repo, "issue", "view", strconv.FormatInt(number, 10), "--json", "number,title,url,body,state")
	if err != nil {
		return Issue{}, err
	}
	var out Issue
	if err := json.Unmarshal(raw, &out); err != nil {
		return Issue{}, err
	}
	out.Repo = repo
	if out.Number == 0 {
		out.Number = number
	}
	return out, nil
}

// ViewPull 用本机 gh 读一条 PR，不落 token、不合并。
func ViewPull(ctx context.Context, repo string, number int64) (Pull, error) {
	raw, err := runGH(ctx, repo, "pr", "view", strconv.FormatInt(number, 10), "--json", "number,title,url,body,state")
	if err != nil {
		return Pull{}, err
	}
	var out Pull
	if err := json.Unmarshal(raw, &out); err != nil {
		return Pull{}, err
	}
	out.Repo = repo
	if out.Number == 0 {
		out.Number = number
	}
	return out, nil
}

// runGH 调本机 gh，超时 30s，不弹交互。
func runGH(ctx context.Context, repo string, args ...string) ([]byte, error) {
	if numberArg(args) == "0" {
		return nil, errors.New("number is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if strings.TrimSpace(repo) != "" {
		args = append(args, "--repo", repo)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(), "GH_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return stdout.Bytes(), nil
}

// numberArg 取出 view 后面的编号参数。
func numberArg(args []string) string {
	for i, arg := range args {
		if arg == "view" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// FormatRef 把 owner/repo#n 或 GitHub 链接收成 repo 与编号。
func FormatRef(raw string) (repo string, number int, err error) {
	repo, number, _, err = ParseLink(raw)
	return repo, number, err
}

// ParseLink 从 GitHub 链接或 owner/repo#n 取出仓库、编号和种类。种类为 issue、pull，或空表示链接里看不出来。
func ParseLink(raw string) (repo string, number int, kind string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, "", errors.New("link is required")
	}
	if strings.Contains(raw, "://") || strings.HasPrefix(strings.ToLower(raw), "github.com/") || strings.HasPrefix(strings.ToLower(raw), "www.github.com/") {
		return parseGitHubURL(raw)
	}
	repo, rest := "", raw
	if i := strings.LastIndex(raw, "#"); i >= 0 {
		repo = strings.TrimSpace(raw[:i])
		rest = raw[i+1:]
	}
	n, convErr := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(rest), "#"))
	if convErr != nil || n <= 0 {
		return "", 0, "", errors.New("invalid issue or pull number")
	}
	return repo, n, "", nil
}

// parseGitHubURL 认出 /issues/n 与 /pull/n。
func parseGitHubURL(raw string) (repo string, number int, kind string, err error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", 0, "", errors.New("invalid link")
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
	if host != "github.com" {
		return "", 0, "", errors.New("link must be a github.com URL")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] == "" {
		return "", 0, "", errors.New("invalid github link")
	}
	switch parts[2] {
	case "issues", "issue":
		kind = "issue"
	case "pull", "pulls":
		kind = "pull"
	default:
		return "", 0, "", errors.New("link must point at an issue or pull request")
	}
	n, convErr := strconv.Atoi(parts[3])
	if convErr != nil || n <= 0 {
		return "", 0, "", errors.New("invalid issue or pull number")
	}
	return parts[0] + "/" + parts[1], n, kind, nil
}
