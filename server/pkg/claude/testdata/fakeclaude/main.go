package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

func main() {
	if raw := os.Getenv("FAKE_CLAUDE_SLEEP"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			time.Sleep(d)
		}
	}
	args := os.Args[1:]
	if has(args, "-v") || has(args, "--version") {
		fmt.Println(getenv("FAKE_CLAUDE_VERSION", "2.1.0"))
		return
	}
	if len(args) >= 2 && args[0] == "auth" && args[1] == "status" {
		if os.Getenv("FAKE_CLAUDE_AUTH") == "0" {
			fmt.Println(`{"loggedIn":false}`)
			os.Exit(1)
		}
		fmt.Println(`{"loggedIn":true}`)
		return
	}

	sessionID := flagValue(args, "--session-id")
	resume := flagValue(args, "--resume")
	if resume == "" {
		resume = flagValue(args, "-r")
	}
	if sessionID == "" {
		sessionID = resume
	}
	if sessionID == "" || has(args, "--fork-session") {
		sessionID = uuid.NewString()
	}
	prompt := lastPositional(args)
	if has(args, "--fork-session") && resume != "" {
		copyJSONL(resume, sessionID)
	} else {
		writeJSONL(sessionID, prompt)
	}

	if os.Getenv("FAKE_CLAUDE_LOG") != "" {
		_ = os.WriteFile(os.Getenv("FAKE_CLAUDE_LOG"), []byte(strings.Join(args, " ")), 0o644)
	}

	if has(args, "stream-json") || flagValue(args, "--output-format") == "stream-json" {
		runStream(sessionID)
		return
	}
	fmt.Printf("{\"type\":\"result\",\"subtype\":\"success\",\"session_id\":%q,\"result\":\"ok\"}\n", sessionID)
}

func runStream(sessionID string) {
	got := make(chan struct{}, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "control_response") {
				select {
				case got <- struct{}{}:
				default:
				}
			}
		}
	}()
	emit(map[string]any{"type": "system", "subtype": "init", "session_id": sessionID})
	mode := os.Getenv("FAKE_CLAUDE_MODE")
	switch mode {
	case "ask":
		emit(controlAsk(sessionID, "Bash", map[string]any{"command": "ls"}))
		wait(got)
	case "edit":
		emit(controlAsk(sessionID, "Edit", map[string]any{"path": "a.go", "new_string": "x"}))
		wait(got)
	case "multiedit":
		emit(controlAsk(sessionID, "MultiEdit", map[string]any{"file_path": "b.go", "new_string": "y"}))
		wait(got)
	case "question":
		emit(controlAsk(sessionID, "AskUserQuestion", map[string]any{
			"question": "pick",
			"options":  []any{map[string]any{"label": "a"}, map[string]any{"label": "b"}},
		}))
		wait(got)
	case "form":
		emit(controlAsk(sessionID, "mcp__server__form", map[string]any{"prompt": "fill", "fields": []string{"name"}}))
		wait(got)
	case "form_values":
		emit(controlAsk(sessionID, "CustomForm", map[string]any{"prompt": "fill", "values": []string{"n"}}))
		wait(got)
	case "unknown":
		emit(controlAsk(sessionID, "BrandNewTool", map[string]any{"foo": "bar"}))
		wait(got)
	case "other":
		emit(map[string]any{
			"type":       "control_request",
			"request_id": "req-other",
			"session_id": sessionID,
			"request":    map[string]any{"subtype": "set_max_thinking_tokens"},
		})
		wait(got)
	case "hang":
		time.Sleep(30 * time.Second)
	case "fail":
		os.Exit(1)
	default:
		emit(map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "thinking", "thinking": "hmm"}, {"type": "text", "text": "ok"}},
			},
		})
		emit(map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "tool_use", "name": "Bash", "input": map[string]any{"command": "pwd"}}},
			},
		})
		emit(map[string]any{"type": "system", "subtype": "notice", "result": "note"})
	}
	emit(map[string]any{"type": "result", "subtype": "success", "session_id": sessionID, "result": "ok"})
}

func controlAsk(sessionID, tool string, input map[string]any) map[string]any {
	return map[string]any{
		"type":       "control_request",
		"request_id": "req-1",
		"session_id": sessionID,
		"request": map[string]any{
			"subtype":   "can_use_tool",
			"tool_name": tool,
			"input":     input,
		},
	}
}

func wait(got chan struct{}) {
	select {
	case <-got:
	case <-time.After(8 * time.Second):
	}
}

func emit(v any) {
	body, _ := json.Marshal(v)
	fmt.Println(string(body))
}

// copyJSONL 按 --fork-session 复制已落盘实录到新 session 文件。
func copyJSONL(from, to string) {
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" || from == "" || to == "" {
		return
	}
	wd, _ := os.Getwd()
	dir := filepath.Join(cfg, "projects", sanitize(wd))
	src, err := os.ReadFile(filepath.Join(dir, from+".jsonl"))
	if err != nil {
		writeJSONL(to, "")
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, to+".jsonl"), src, 0o644)
}

func writeJSONL(sessionID, prompt string) {
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" {
		return
	}
	wd, _ := os.Getwd()
	dir := filepath.Join(cfg, "projects", sanitize(wd))
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, sessionID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if prompt != "" {
		fmt.Fprintf(f, "{\"type\":\"user\",\"message\":{\"role\":\"user\",\"content\":%q}}\n", prompt)
	}
	fmt.Fprintln(f, `{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`)
}

func sanitize(cwd string) string {
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
	return b.String()
}

func has(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimPrefix(arg, name+"=")
		}
	}
	return ""
}

func lastPositional(args []string) string {
	for i := len(args) - 1; i >= 0; i-- {
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		if i > 0 && strings.HasPrefix(args[i-1], "-") && args[i-1] != "-p" && args[i-1] != "--print" {
			continue
		}
		return args[i]
	}
	return ""
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
