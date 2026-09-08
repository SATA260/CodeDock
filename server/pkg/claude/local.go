package claude

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode"
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
	ids := []string{"default", "acceptEdits", "plan", "auto", "dontAsk", "bypassPermissions"}
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

func parseTitleAndProgress(path string) (string, []Progress, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	title := ""
	items := make([]Progress, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := bytesTrim(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if t := titleFromLine(line); t != "" && title == "" {
			title = t
		}
		if item, ok := progressFromLine(line); ok {
			items = append(items, item)
			if title == "" && item.Kind == ProgressKindUser && item.Text != "" {
				title = clipTitle(item.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return title, items, err
	}
	return title, items, nil
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
		settings.PermissionMode = parsed.PermissionMode
	} else if parsed.DefaultMode != "" {
		settings.PermissionMode = parsed.DefaultMode
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
	if path != "" {
		parsed, _, err := parseTitleAndProgress(path)
		if err != nil {
			return Session{}, err
		}
		title = parsed
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
	id := claudeSessionID
	rt.mu.Lock()
	if sess, ok := rt.sessions[claudeSessionID]; ok && sess.ClaudeSessionID != "" {
		id = sess.ClaudeSessionID
	}
	rt.mu.Unlock()
	path := findSessionFile(id)
	if path == "" {
		return []Progress{}, nil
	}
	_, items, err := parseTitleAndProgress(path)
	if items == nil {
		items = []Progress{}
	}
	return items, err
}

// ReadSettings 从本机 Claude 读生效配置。
func ReadSettings(claudeSessionID string) (Settings, error) {
	_ = claudeSessionID
	return readSettingsFile(), nil
}
