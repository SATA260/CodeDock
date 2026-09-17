package codex

import (
	"fmt"
	"strconv"
	"strings"
)

// forkTitleStem 去掉末尾 (n)，避免 fork「ping (1)」变成「ping (1) (1)」。
func forkTitleStem(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	i := strings.LastIndex(title, " (")
	if i < 0 || !strings.HasSuffix(title, ")") {
		return title
	}
	raw := title[i+2 : len(title)-1]
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || strings.TrimSpace(title[:i]) == "" {
		return title
	}
	return strings.TrimSpace(title[:i])
}

// nextForkTitle 在同名对话上取下一个空位，得到「标题 (1)」「标题 (2)」。
func nextForkTitle(base string, titles []string) string {
	stem := forkTitleStem(base)
	if stem == "" {
		stem = "fork"
	}
	used := map[int]bool{}
	for _, title := range titles {
		title = strings.TrimSpace(title)
		n, ok := forkTitleIndex(title, stem)
		if ok {
			used[n] = true
		}
	}
	n := 1
	for used[n] {
		n++
	}
	return fmt.Sprintf("%s (%d)", stem, n)
}

// forkTitleIndex 判断标题是不是 stem (n)。
func forkTitleIndex(title, stem string) (int, bool) {
	prefix := stem + " ("
	if !strings.HasPrefix(title, prefix) || !strings.HasSuffix(title, ")") {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(title, prefix), ")")
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || title != fmt.Sprintf("%s (%d)", stem, n) {
		return 0, false
	}
	return n, true
}
