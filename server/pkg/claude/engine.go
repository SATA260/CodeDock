package claude

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func turnSettingsArgs(settings Settings) []string {
	var args []string
	over := map[string]bool{}
	for _, name := range settings.Overridden {
		over[name] = true
	}
	if over["model"] && settings.Model != "" {
		args = append(args, "--model", settings.Model)
	}
	if over["effort"] && settings.Effort != "" {
		args = append(args, "--effort", settings.Effort)
	}
	if over["permission_mode"] && settings.PermissionMode != "" {
		args = append(args, "--permission-mode", settings.PermissionMode)
	}
	return args
}

func printArgs(prompt string, extra ...string) []string {
	args := []string{"-p", "--bare", "--output-format", "json"}
	args = append(args, extra...)
	if prompt != "" {
		args = append(args, prompt)
	}
	return args
}

func runPrint(cwd, prompt string, extra ...string) (string, error) {
	out := runClaude(cwd, printArgs(prompt, extra...)...)
	if out.err != nil {
		return "", claudeErr(out)
	}
	return out.stdout, nil
}

// StartSession 让 Claude Code 新建一条 session。
func StartSession(cwd string, settings Settings) (string, error) {
	_ = settings
	if strings.TrimSpace(cwd) == "" {
		cwd = defaultCwd()
	}
	id := uuid.NewString()
	_ = cwd
	return id, nil
}

// ResumeSession 接上已有的 Claude session。
func ResumeSession(claudeSessionID string) error {
	if claudeSessionID == "" {
		return wrapErr(errInvalid, "claude session id is required")
	}
	return nil
}

// ForkSession 按官方 --resume <id> --fork-session：复制已落盘实录，换新 session ID，不发模型。
func ForkSession(claudeSessionID string) (string, error) {
	if claudeSessionID == "" {
		return "", wrapErr(errInvalid, "claude session id is required")
	}
	src := findSessionFile(claudeSessionID)
	if src == "" {
		return "", wrapErr(errNotFound, "no conversation found with session ID: %s", claudeSessionID)
	}
	return copyForkedSession(src)
}

// StartTurn 让 Claude Code 开始一轮。
func StartTurn(claudeSessionID string, input Input, settings Settings) (string, error) {
	turnID := uuid.NewString()
	sessionID := ""
	if found := sessionByClaudeID(claudeSessionID); found != nil {
		sessionID = found.ID
	}
	if err := startTurn(turnID, sessionID, claudeSessionID, input, settings); err != nil {
		return "", err
	}
	return turnID, nil
}

func startTurn(turnID, sessionID, claudeSessionID string, input Input, settings Settings) error {
	cwd := settings.Cwd
	if cwd == "" {
		cwd = defaultCwd()
	}
	args := []string{
		"-p", "--bare",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
		"--permission-prompt-tool", "stdio",
	}
	args = append(args, turnSettingsArgs(settings)...)
	if claudeSessionID != "" {
		if sessionFileExists(claudeSessionID) {
			args = append(args, "--resume", claudeSessionID)
		} else {
			args = append(args, "--session-id", claudeSessionID)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd, stdin, stdout, err := startClaude(cwd, args)
	if err != nil {
		cancel()
		return wrapErr(errUnavailable, "%s", err.Error())
	}
	rt.mu.Lock()
	rt.turns[turnID] = &memTurn{
		ID:              turnID,
		SessionID:       sessionID,
		ClaudeSessionID: claudeSessionID,
		Status:          TurnRunning,
		cmd:             cmd,
		stdin:           stdin,
		cancel:          cancel,
		waiters:         map[string]chan AskAnswer{},
	}
	if sessionID != "" {
		internLocked(sessionID).ActiveTurnID = turnID
	}
	rt.mu.Unlock()
	go func() {
		defer cancel()
		pumpTurn(ctx, turnID, stdout)
		_ = cmd.Wait()
		_ = stdin.Close()
		finishTurn(turnID)
	}()
	_ = writeUserMessage(stdin, input)
	return nil
}

func writeUserMessage(stdin io.WriteCloser, input Input) error {
	if stdin == nil {
		return nil
	}
	text := input.Text
	for _, mention := range input.Mentions {
		if mention == "" {
			continue
		}
		text = strings.TrimSpace(text + "\n@" + mention)
	}
	type part map[string]any
	parts := []part{{"type": "text", "text": text}}
	for _, image := range input.Images {
		block, err := imagePart(image)
		if err != nil {
			return err
		}
		parts = append(parts, block)
	}
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": parts,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = stdin.Write(append(body, '\n'))
	return err
}

func imagePart(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, wrapErr(errInvalid, "%s", err.Error())
	}
	media := "image/png"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		media = "image/jpeg"
	case ".gif":
		media = "image/gif"
	case ".webp":
		media = "image/webp"
	}
	return map[string]any{
		"type": "image",
		"source": map[string]any{
			"type":       "base64",
			"media_type": media,
			"data":       base64.StdEncoding.EncodeToString(data),
		},
	}, nil
}

func pumpTurn(ctx context.Context, turnID string, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		handleStreamLine(turnID, append([]byte{}, line...))
	}
}

func handleStreamLine(turnID string, line []byte) {
	if reqID, req, ok := parseControlRequest(line); ok {
		handleControlAsk(turnID, reqID, req)
		return
	}
	var parsed ndjsonLine
	if err := json.Unmarshal(line, &parsed); err != nil {
		return
	}
	if parsed.Type == "result" {
		if parsed.SessionID != "" {
			bindFromTurn(turnID, parsed.SessionID)
		}
		closeTurnStdin(turnID)
	}
	if parsed.Type == "system" && parsed.Subtype == "init" && parsed.SessionID != "" {
		bindFromTurn(turnID, parsed.SessionID)
	}
	sessID := turnSessionID(turnID)
	if item, ok := progressFromLine(line); ok {
		_ = AppendProgress(sessID, turnID, item)
	}
}

func bindFromTurn(turnID, claudeSessionID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	turn := rt.turns[turnID]
	if turn == nil {
		return
	}
	turn.ClaudeSessionID = claudeSessionID
	if turn.SessionID == "" {
		return
	}
	sess := internLocked(turn.SessionID)
	if sess.ClaudeSessionID == "" {
		sess.ClaudeSessionID = claudeSessionID
	}
}

func closeTurnStdin(turnID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	turn := rt.turns[turnID]
	if turn == nil || turn.stdin == nil {
		return
	}
	_ = turn.stdin.Close()
	turn.stdin = nil
}

func turnSessionID(turnID string) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if turn := rt.turns[turnID]; turn != nil {
		return turn.SessionID
	}
	return ""
}

func handleControlAsk(turnID, requestID string, req controlRequest) {
	if req.Subtype != "" && req.Subtype != "can_use_tool" {
		sessID := turnSessionID(turnID)
		_ = AppendProgress(sessID, turnID, Progress{Kind: ProgressKindNotice, Text: "不兼容的提问", Paths: emptyStrings()})
		_ = writeControlResponse(turnID, requestID, AskAnswer{Approved: false})
		return
	}
	ask, known := askFromTool(req)
	ask.ExternalRequestID = requestID
	if !known {
		sessID := turnSessionID(turnID)
		_ = AppendProgress(sessID, turnID, Progress{Kind: ProgressKindNotice, Text: "不兼容的提问", Paths: emptyStrings()})
		_ = writeControlResponse(turnID, requestID, AskAnswer{Approved: false})
		return
	}
	ch := make(chan AskAnswer, 1)
	rt.mu.Lock()
	if turn := rt.turns[turnID]; turn != nil {
		turn.Status = TurnWaitingApproval
		turn.waiters[requestID] = ch
	}
	rt.mu.Unlock()
	_, _ = Require(turnID, ask)
	select {
	case answer := <-ch:
		_ = writeControlResponse(turnID, requestID, answer)
	case <-time.After(10 * time.Minute):
		_ = writeControlResponse(turnID, requestID, AskAnswer{Approved: false})
	}
}

func writeControlResponse(turnID, requestID string, answer AskAnswer) error {
	behavior := "deny"
	message := "rejected"
	updated := map[string]any{}
	if answer.Approved {
		behavior = "allow"
		message = ""
		if answer.Choice != "" {
			updated["answers"] = map[string]string{"choice": answer.Choice}
		}
		if len(answer.Values) > 0 {
			updated["values"] = answer.Values
		}
	}
	payload := map[string]any{
		"type":       "control_response",
		"request_id": requestID,
		"response": map[string]any{
			"subtype": "success",
			"response": map[string]any{
				"behavior":     behavior,
				"message":      message,
				"updatedInput": updated,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	turn := rt.turns[turnID]
	stdin := io.WriteCloser(nil)
	if turn != nil {
		stdin = turn.stdin
		if turn.Status == TurnWaitingApproval {
			turn.Status = TurnRunning
		}
	}
	rt.mu.Unlock()
	if stdin == nil {
		return nil
	}
	_, err = stdin.Write(append(body, '\n'))
	return err
}

func finishTurn(turnID string) {
	rt.mu.Lock()
	turn := rt.turns[turnID]
	sessionID := ""
	if turn != nil {
		sessionID = turn.SessionID
		if turn.Status == TurnRunning || turn.Status == TurnWaitingApproval {
			turn.Status = TurnCompleted
		}
		if turn.stdin != nil {
			_ = turn.stdin.Close()
			turn.stdin = nil
		}
	}
	rt.mu.Unlock()
	if sessionID != "" {
		_ = ClearActiveTurn(sessionID, turnID)
		drainQueue(sessionID)
	}
}

// Interrupt 按用户请求打断 Claude Code 当前一轮。
func Interrupt(claudeSessionID, turnID string) error {
	_ = claudeSessionID
	rt.mu.Lock()
	turn := rt.turns[turnID]
	if turn == nil && claudeSessionID != "" {
		for _, item := range rt.turns {
			if item.ClaudeSessionID == claudeSessionID {
				turn = item
				break
			}
		}
	}
	var cmd *exec.Cmd
	if turn != nil {
		turn.Status = TurnCancelled
		if turn.cancel != nil {
			turn.cancel()
		}
		cmd = turn.cmd
	}
	rt.mu.Unlock()
	return signalInterrupt(cmd)
}

// Compact 让 Claude Code 自己压缩上下文。
func Compact(claudeSessionID string) error {
	if claudeSessionID == "" {
		return wrapErr(errInvalid, "claude session id is required")
	}
	_, err := runPrint(defaultCwd(), "/compact", "--resume", claudeSessionID)
	return err
}

// Review 让 Claude Code 评审当前工作区改动。
func Review(claudeSessionID string) error {
	if claudeSessionID == "" {
		return wrapErr(errInvalid, "claude session id is required")
	}
	_, err := runPrint(defaultCwd(), "/review", "--resume", claudeSessionID)
	return err
}

func renameClaude(claudeSessionID, title string) error {
	if claudeSessionID == "" {
		return nil
	}
	_, err := runPrint(defaultCwd(), "/rename "+title, "--resume", claudeSessionID)
	return err
}

// ReplyAsk 把人对已知反问的回答回给 Claude Code。
func ReplyAsk(requestID string, answer AskAnswer) error {
	if requestID == "" {
		return nil
	}
	rt.mu.Lock()
	ask := rt.asks[requestID]
	turnID := ""
	var ch chan AskAnswer
	if ask != nil {
		turnID = ask.TurnID
	}
	if turnID == "" {
		for id, turn := range rt.turns {
			if waiter, ok := turn.waiters[requestID]; ok {
				turnID = id
				ch = waiter
				break
			}
		}
	} else if turn := rt.turns[turnID]; turn != nil {
		ch = turn.waiters[requestID]
	}
	rt.mu.Unlock()
	if ch != nil {
		select {
		case ch <- answer:
		default:
		}
		return nil
	}
	if turnID != "" {
		return writeControlResponse(turnID, requestID, answer)
	}
	return nil
}

// RejectUnknown 官方新加、认不出的提问：提示不兼容并回包，避免卡死。
func RejectUnknown(requestID string) error {
	return ReplyAsk(requestID, AskAnswer{Approved: false})
}
