package claude

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

func configDir() string {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

func defaultCwd() string {
	if repo := strings.TrimSpace(os.Getenv("GIT_REPO")); repo != "" {
		if abs, err := filepath.Abs(repo); err == nil {
			return abs
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func sanitizeProject(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	var b strings.Builder
	for _, r := range abs {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := b.String()
	if len(name) > 200 {
		sum := sha1.Sum([]byte(abs))
		name = name[:200] + hex.EncodeToString(sum[:4])
	}
	return name
}

func projectDir(cwd string) string {
	return filepath.Join(configDir(), "projects", sanitizeProject(cwd))
}

func emptyStrings() []string {
	return []string{}
}

func catalogModels() []ModelInfo {
	efforts := []string{"low", "medium", "high", "xhigh", "max"}
	return []ModelInfo{
		{ID: "sonnet", Efforts: append([]string{}, efforts...), DefaultEffort: "medium", IsDefault: true},
		{ID: "opus", Efforts: append([]string{}, efforts...), DefaultEffort: "high"},
		{ID: "haiku", Efforts: []string{"low", "medium", "high"}, DefaultEffort: "medium"},
		{ID: "fable", Efforts: []string{"low", "medium", "high"}, DefaultEffort: "medium", Hidden: true},
	}
}

func catalogModes() []ModeInfo {
	ids := []string{"default", "acceptEdits", "plan", "bypassPermissions", "auto", "dontAsk"}
	out := make([]ModeInfo, 0, len(ids))
	for _, id := range ids {
		out = append(out, ModeInfo{ID: id, Kind: "permission"})
	}
	return out
}

func findSessionFile(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	root := filepath.Join(configDir(), "projects")
	matches, err := filepath.Glob(filepath.Join(root, "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func sessionFileExists(sessionID string) bool {
	return findSessionFile(sessionID) != ""
}

// copyForkedSession 按官方 --fork-session 落盘：同目录复制实录，换新 session ID。
func copyForkedSession(src string) (string, error) {
	from := strings.TrimSuffix(filepath.Base(src), ".jsonl")
	id := uuid.NewString()
	dst := filepath.Join(filepath.Dir(src), id+".jsonl")
	body, err := os.ReadFile(src)
	if err != nil {
		return "", wrapErr(errUnavailable, "%s", err.Error())
	}
	if err := os.WriteFile(dst, rewriteSessionIDs(body, from, id), 0o644); err != nil {
		return "", wrapErr(errUnavailable, "%s", err.Error())
	}
	return id, nil
}

// rewriteSessionIDs 只改实录行顶层 sessionId / session_id，不改正文里的编号。
func rewriteSessionIDs(body []byte, from, to string) []byte {
	if from == "" || from == to {
		return body
	}
	lines := strings.Split(string(body), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			out = append(out, line)
			continue
		}
		changed := false
		for _, key := range []string{"sessionId", "session_id"} {
			if value, ok := obj[key].(string); ok && value == from {
				obj[key] = to
				changed = true
			}
		}
		if !changed {
			out = append(out, line)
			continue
		}
		rewritten, err := json.Marshal(obj)
		if err != nil {
			out = append(out, line)
			continue
		}
		out = append(out, string(rewritten))
	}
	return []byte(strings.Join(out, "\n"))
}

// applySessionTitle 把官方 system/title 行写进实录，fork 后的序号才能留下。
func applySessionTitle(path, title string) error {
	title = strings.TrimSpace(title)
	if path == "" || title == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return wrapErr(errUnavailable, "%s", err.Error())
	}
	encoded, err := json.Marshal(map[string]any{
		"type":    "system",
		"subtype": "title",
		"title":   title,
	})
	if err != nil {
		return wrapErr(errUnavailable, "%s", err.Error())
	}
	lines := strings.Split(string(body), "\n")
	out := make([]string, 0, len(lines)+1)
	replaced := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			continue
		}
		if isTitleLine([]byte(trimmed)) {
			if !replaced {
				out = append(out, string(encoded))
				replaced = true
			}
			continue
		}
		out = append(out, line)
	}
	if !replaced {
		out = append([]string{string(encoded)}, out...)
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}

// tagClaudeSession 按官方 tagSession 往本机实录追加 type=tag。还没有实录时只改内存。
func tagClaudeSession(sess Session, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return wrapErr(errInvalid, "tag is required")
	}
	resumeID := sess.ClaudeSessionID
	if resumeID == "" {
		resumeID = sess.ID
	}
	path := findSessionFile(resumeID)
	if path == "" {
		path = findSessionFile(sess.ID)
	}
	if path == "" {
		return nil
	}
	return applySessionTag(path, tag)
}

// applySessionTag 往官方 JSONL 追加一条 tag，后写的覆盖先写的。
func applySessionTag(path, tag string) error {
	if path == "" {
		return wrapErr(errNotFound, "session file is required")
	}
	encoded, err := json.Marshal(map[string]any{"type": "tag", "tag": tag})
	if err != nil {
		return wrapErr(errUnavailable, "%s", err.Error())
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return wrapErr(errUnavailable, "%s", err.Error())
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return wrapErr(errUnavailable, "%s", err.Error())
	}
	return nil
}

// isTitleLine 认官方实录里的会话标题行。
func isTitleLine(line []byte) bool {
	var parsed ndjsonLine
	if err := json.Unmarshal(line, &parsed); err != nil {
		return false
	}
	return parsed.Type == "system" && parsed.Subtype == "title"
}

type parsedSessionFile struct {
	Title     string
	Items     []Progress
	Usage     TokenUsage
	CreatedAt int64
	UpdatedAt int64
	Archived  bool // 官方 tagSession("archived") 打在实录上。
}

// parseSessionFile 读本机 JSONL：标题、给人看的实录，以及首末 timestamp。
func parseSessionFile(path string) (parsedSessionFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return parsedSessionFile{}, err
	}
	defer file.Close()
	created, updated := fileUnixTimes(path)
	firstTS, lastTS := int64(0), int64(0)
	out := parsedSessionFile{Items: make([]Progress, 0), CreatedAt: created, UpdatedAt: updated}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := bytesTrim(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if ts := timestampFromLine(line); ts > 0 {
			if firstTS == 0 {
				firstTS = ts
			}
			lastTS = ts
		}
		if t := titleFromLine(line); t != "" && out.Title == "" {
			out.Title = t
		}
		if item, ok := progressFromLine(line); ok {
			out.Items = append(out.Items, item)
			if out.Title == "" && item.Kind == ProgressKindUser && item.Text != "" {
				out.Title = clipTitle(item.Text)
			}
		}
		if usage, ok := usageFromLine(line); ok {
			out.Usage = usage
		}
		if tag, ok := tagFromLine(line); ok {
			out.Archived = strings.EqualFold(strings.TrimSpace(tag), archivedSessionTag)
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	if firstTS > 0 {
		out.CreatedAt = firstTS
	}
	if lastTS > 0 {
		out.UpdatedAt = lastTS
	}
	return out, nil
}

// parseTitleAndProgress 兼容旧调用，只取标题和实录。
func parseTitleAndProgress(path string) (string, []Progress, error) {
	parsed, err := parseSessionFile(path)
	return parsed.Title, parsed.Items, err
}

// fileUnixTimes 用文件 mtime 当创建/更新回落，单位 Unix 秒。
func fileUnixTimes(path string) (created, updated int64) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0
	}
	updated = info.ModTime().Unix()
	return updated, updated
}

// timestampFromLine 读 Claude 实录行上的 ISO timestamp。
func timestampFromLine(line []byte) int64 {
	var parsed ndjsonLine
	if err := json.Unmarshal(line, &parsed); err != nil {
		return 0
	}
	raw := strings.TrimSpace(parsed.Timestamp)
	if raw == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.Unix()
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.Unix()
	}
	return 0
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func clipTitle(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if len([]rune(text)) > 80 {
		return string([]rune(text)[:80])
	}
	return text
}

type fileSettings struct {
	Model          string `json:"model"`
	PermissionMode string `json:"permissionMode"`
	DefaultMode    string `json:"defaultMode"`
	EffortLevel    string `json:"effortLevel"`
}

func readSettingsFile() Settings {
	settings := Settings{
		Model:          "sonnet",
		Effort:         "medium",
		PermissionMode: "default",
		Cwd:            defaultCwd(),
		Overridden:     emptyStrings(),
	}
	body, err := os.ReadFile(filepath.Join(configDir(), "settings.json"))
	if err != nil {
		return settings
	}
	var parsed fileSettings
	if err := json.Unmarshal(body, &parsed); err != nil {
		return settings
	}
	if parsed.Model != "" {
		settings.Model = parsed.Model
	}
	if parsed.EffortLevel != "" {
		settings.Effort = parsed.EffortLevel
	}
	if parsed.PermissionMode != "" {
		settings.PermissionMode = canonicalPermissionMode(parsed.PermissionMode)
	} else if parsed.DefaultMode != "" {
		settings.PermissionMode = canonicalPermissionMode(parsed.DefaultMode)
	}
	return settings
}

func authLoggedIn(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		lower := strings.ToLower(body)
		return strings.Contains(lower, "logged in") || strings.Contains(lower, "logged-in")
	}
	for _, key := range []string{"loggedIn", "logged_in", "authenticated"} {
		switch v := payload[key].(type) {
		case bool:
			if v {
				return true
			}
		case string:
			if strings.EqualFold(v, "true") || strings.EqualFold(v, "logged in") {
				return true
			}
		}
	}
	if status, ok := payload["status"].(string); ok && strings.EqualFold(status, "logged_in") {
		return true
	}
	return false
}

// ReadEngine 从本机 Claude 读是否可用、是否已取得授权。
func ReadEngine() (EngineStatus, error) {
	status := EngineStatus{}
	if err := lookClaude(); err != nil {
		status.Hint = "未安装 Claude Code"
		return status, nil
	}
	status.Available = true
	ver := runClaude("", "-v")
	status.Version = strings.TrimSpace(ver.stdout)
	if status.Version == "" {
		status.Version = strings.TrimSpace(ver.stderr)
	}
	auth := runClaude("", "auth", "status")
	if auth.err == nil || authLoggedIn(auth.stdout) || authLoggedIn(auth.stderr) {
		status.Authorized = true
		return status, nil
	}
	status.Hint = "未取得 Claude 授权"
	return status, nil
}

// ReadModels 从本机 Claude 读模型及各自支持的推理强度。
func ReadModels() ([]ModelInfo, error) {
	if _, err := ReadEngine(); err != nil {
		return []ModelInfo{}, err
	}
	return catalogModels(), nil
}

// ReadModes 从本机 Claude 读官方权限档。
func ReadModes() ([]ModeInfo, error) {
	return catalogModes(), nil
}

// ReadSession 从本机 Claude 读一条 session。
func ReadSession(claudeSessionID string) (Session, error) {
	if claudeSessionID == "" {
		return Session{}, wrapErr(errInvalid, "session_id is required")
	}
	rt.mu.Lock()
	sess := internLocked(claudeSessionID)
	claudeID := sess.ClaudeSessionID
	rt.mu.Unlock()
	path := findSessionFile(claudeSessionID)
	if path == "" && claudeID != "" {
		path = findSessionFile(claudeID)
	}
	title := ""
	fileID := ""
	createdAt, updatedAt := int64(0), int64(0)
	archived := false
	if path != "" {
		parsed, err := parseSessionFile(path)
		if err != nil {
			return Session{}, err
		}
		title = parsed.Title
		createdAt = parsed.CreatedAt
		updatedAt = parsed.UpdatedAt
		archived = parsed.Archived
		fileID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess = internLocked(claudeSessionID)
	if sess.Title == "" && title != "" {
		sess.Title = title
	}
	if sess.ClaudeSessionID == "" && fileID != "" {
		sess.ClaudeSessionID = fileID
	}
	if createdAt > 0 {
		sess.CreatedAt = createdAt
	}
	if updatedAt > 0 {
		sess.UpdatedAt = updatedAt
	}
	if archived {
		sess.Archived = true
	}
	out := sess.snapshot()
	if out.Title == "" {
		out.Title = title
	}
	if out.ClaudeSessionID == "" {
		out.ClaudeSessionID = fileID
	}
	return out, nil
}

// ReadSessions 从本机 Claude 读对话列表。
func ReadSessions() ([]Session, error) {
	seen := map[string]Session{}
	dir := projectDir(defaultCwd())
	entries, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	all, _ := filepath.Glob(filepath.Join(configDir(), "projects", "*", "*.jsonl"))
	if len(entries) == 0 {
		entries = all
	}
	for _, path := range entries {
		id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		sess, err := ReadSession(id)
		if err != nil {
			continue
		}
		seen[id] = sess
	}
	rt.mu.Lock()
	for id, sess := range rt.sessions {
		snap := sess.snapshot()
		if snap.ID == "" {
			snap.ID = id
		}
		seen[snap.ID] = snap
	}
	rt.mu.Unlock()
	out := make([]Session, 0, len(seen))
	for _, sess := range seen {
		out = append(out, sess)
	}
	return out, nil
}

// ReadTranscript 从本机 Claude 读给人看的实录。
func ReadTranscript(claudeSessionID string) ([]Progress, error) {
	items, _, err := ReadTranscriptAndUsage(claudeSessionID)
	return items, err
}

// ReadTranscriptAndUsage 回放实录，并带上官方最后一次用量。
func ReadTranscriptAndUsage(claudeSessionID string) ([]Progress, TokenUsage, error) {
	id := claudeSessionID
	rt.mu.Lock()
	if sess, ok := rt.sessions[claudeSessionID]; ok && sess.ClaudeSessionID != "" {
		id = sess.ClaudeSessionID
	}
	rt.mu.Unlock()
	path := findSessionFile(id)
	if path == "" {
		return []Progress{}, TokenUsage{}, nil
	}
	parsed, err := parseSessionFile(path)
	items := parsed.Items
	if items == nil {
		items = []Progress{}
	}
	return items, parsed.Usage, err
}

// ReadSettings 从本机 Claude 读生效配置。
func ReadSettings(claudeSessionID string) (Settings, error) {
	_ = claudeSessionID
	return readSettingsFile(), nil
}
