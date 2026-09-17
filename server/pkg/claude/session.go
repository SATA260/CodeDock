package claude

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Create 创建一条只走 Claude Code 的对话。Claude session 编号在首次开回合后才有。
func Create(userID string) (Session, error) {
	_ = userID
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(uuid.NewString())
	return sess.snapshot(), nil
}

// Get 读取对话。
func Get(sessionID string) (Session, error) {
	if sessionID == "" {
		return Session{}, wrapErr(errInvalid, "session_id is required")
	}
	return ReadSession(sessionID)
}

// ListSessions 从本机 Claude 列对话。
func ListSessions() ([]Session, error) {
	return ReadSessions()
}

// BindClaudeSession 记下 Claude session 编号，只能写一次。本模块不落库，编号以本机 Claude 为准。
func BindClaudeSession(sessionID, claudeSessionID string) error {
	if sessionID == "" || claudeSessionID == "" {
		return wrapErr(errInvalid, "session id is required")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(sessionID)
	if sess.ClaudeSessionID != "" && sess.ClaudeSessionID != claudeSessionID {
		return wrapErr(errConflict, "claude session already bound")
	}
	sess.ClaudeSessionID = claudeSessionID
	return nil
}

// Archive 归档，之后不能再向 Claude Code 开回合。不删本机 Claude 记录。
func Archive(sessionID string) error {
	sess, err := Get(sessionID)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	internLocked(sess.ID).Archived = true
	return nil
}

// Rename 改对话标题。
func Rename(sessionID, title string) error {
	sess, err := Get(sessionID)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	internLocked(sess.ID).Title = title
	claudeID := internLocked(sess.ID).ClaudeSessionID
	rt.mu.Unlock()
	return renameClaude(claudeID, title)
}

// Fork 按官方 --fork-session 复制已落盘实录，换新 Claude session，原对话不动。
func Fork(sessionID string) (Session, error) {
	sess, err := Get(sessionID)
	if err != nil {
		return Session{}, err
	}
	resumeID := sess.ClaudeSessionID
	if resumeID == "" {
		resumeID = sess.ID
	}
	newID, err := ForkSession(resumeID)
	if err != nil {
		return Session{}, err
	}
	title := nextForkTitle(sess.Title, sessionTitles())
	if path := findSessionFile(newID); path != "" && title != "" {
		if err := applySessionTitle(path, title); err != nil {
			return Session{}, err
		}
	}
	rt.mu.Lock()
	child := internLocked(newID)
	child.ClaudeSessionID = newID
	child.Title = title
	out := child.snapshot()
	rt.mu.Unlock()
	return out, nil
}

// sessionTitles 收集本机已有对话标题，用来给 fork 编号。
func sessionTitles() []string {
	list, err := ListSessions()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item.Title != "" {
			out = append(out, item.Title)
		}
	}
	return out
}

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

// ClaimActiveTurn 标成当前执行；同时只能有一个。
func ClaimActiveTurn(sessionID, turnID string) error {
	sess, err := Get(sessionID)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	mem := internLocked(sess.ID)
	if mem.Archived {
		return wrapErr(errArchived, "session is archived")
	}
	if mem.ActiveTurnID != "" && mem.ActiveTurnID != turnID {
		return wrapErr(errConflict, "turn already running")
	}
	mem.ActiveTurnID = turnID
	return nil
}

// ClearActiveTurn 清掉当前执行标记。
func ClearActiveTurn(sessionID, turnID string) error {
	if sessionID == "" {
		rt.mu.Lock()
		if turn, ok := rt.turns[turnID]; ok {
			sessionID = turn.SessionID
		}
		rt.mu.Unlock()
	}
	if sessionID == "" {
		return nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	mem := internLocked(sessionID)
	if mem.ActiveTurnID == turnID || turnID == "" {
		mem.ActiveTurnID = ""
	}
	return nil
}
