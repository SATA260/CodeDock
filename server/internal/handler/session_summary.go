package handler

import "strings"

const sessionSummaryMaxRunes = 200

// firstUserSummary 取首次用户输入的首行，作为会话列表摘要。
func firstUserSummary(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if i := strings.IndexAny(content, "\r\n"); i >= 0 {
		content = strings.TrimSpace(content[:i])
	}
	runes := []rune(content)
	if len(runes) > sessionSummaryMaxRunes {
		return string(runes[:sessionSummaryMaxRunes])
	}
	return content
}
