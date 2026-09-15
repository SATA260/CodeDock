package main

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type hit struct {
	start int
	end   int
	kind  string
	value string
}

var (
	reAssignment = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])((?:AWS_SECRET|PRIVATE_KEY|DATABASE_URL|API_KEY|SECRET|TOKEN|PASSWORD)[A-Za-z0-9_]*)\s*[=:]\s*(\S+)`)
	reAWS        = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	reGitHub     = regexp.MustCompile(`(?:ghp_|github_pat_)[^\s]{20,}`)
	reOpenAI     = regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`)
	reSlack      = regexp.MustCompile(`xox[baprs]-[^\s]{10,}`)
	reConn       = regexp.MustCompile(`(?:postgres|mysql|mongodb\+srv|redis)://([^/\s:@]+):([^@\s]+)@`)
	rePEM        = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
)

// token 把原文收成稳定占位符；同值同 kind 永远同一记号。
func token(kind, value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("«REDACTED:%s:%x»", kind, sum[:2])
}

// maskText 按规则替换一段文本里的秘密，返回新文本和命中数。
func maskText(s string) (string, int) {
	if s == "" {
		return s, 0
	}
	hits := collectHits(s)
	if len(hits) == 0 {
		return s, 0
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].start != hits[j].start {
			return hits[i].start < hits[j].start
		}
		return (hits[i].end - hits[i].start) > (hits[j].end - hits[j].start)
	})
	adopted := make([]hit, 0, len(hits))
	for _, h := range hits {
		overlap := false
		for _, a := range adopted {
			if h.start < a.end && h.end > a.start {
				overlap = true
				break
			}
		}
		if !overlap {
			adopted = append(adopted, h)
		}
	}
	sort.Slice(adopted, func(i, j int) bool { return adopted[i].start > adopted[j].start })
	out := s
	for _, h := range adopted {
		out = out[:h.start] + token(h.kind, h.value) + out[h.end:]
	}
	return out, len(adopted)
}

// collectHits 收集全部正则命中，赋值行只记值的区间。
func collectHits(s string) []hit {
	var hits []hit
	for _, loc := range reAssignment.FindAllStringSubmatchIndex(s, -1) {
		if len(loc) < 6 {
			continue
		}
		valStart, valEnd := loc[4], loc[5]
		value := s[valStart:valEnd]
		if q := quoted(value); q != "" {
			value = q
		}
		hits = append(hits, hit{start: valStart, end: valEnd, kind: "assignment", value: value})
	}
	addWhole := func(re *regexp.Regexp, kind string) {
		for _, loc := range re.FindAllStringIndex(s, -1) {
			hits = append(hits, hit{start: loc[0], end: loc[1], kind: kind, value: s[loc[0]:loc[1]]})
		}
	}
	addWhole(reAWS, "aws_ak")
	addWhole(reGitHub, "github")
	addWhole(reOpenAI, "openai")
	addWhole(reSlack, "slack")
	addWhole(rePEM, "pem")
	for _, loc := range reConn.FindAllStringSubmatchIndex(s, -1) {
		if len(loc) < 6 {
			continue
		}
		user, pass := s[loc[2]:loc[3]], s[loc[4]:loc[5]]
		start := loc[2]
		end := loc[5] + 1
		if end > len(s) || s[loc[5]:end] != "@" {
			end = loc[1]
		}
		hits = append(hits, hit{start: start, end: end, kind: "conn", value: user + ":" + pass + "@"})
	}
	return hits
}

// quoted 若整段被成对引号包住则返回去掉引号的正文，否则空串。
func quoted(value string) string {
	if len(value) < 2 {
		return ""
	}
	if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
		(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
		return value[1 : len(value)-1]
	}
	return ""
}
