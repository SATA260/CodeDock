package claude

import (
	"encoding/json"
	"strings"
)

type ndjsonLine struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"session_id"`
	RequestID string          `json:"request_id"`
	Result    string          `json:"result"`
	Request   json.RawMessage `json:"request"`
	Message   json.RawMessage `json:"message"`
	Event     json.RawMessage `json:"event"`
	Title     string          `json:"title"`
	Timestamp string          `json:"timestamp"` // Claude 实录行上的 ISO 时间。
}

type contentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	ID       string          `json:"id"`
	Input    json.RawMessage `json:"input"`
}

type messageBody struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   tokenUsageBody  `json:"usage"`
}

type tokenUsageBody struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type controlRequest struct {
	Subtype   string          `json:"subtype"`
	ToolName  string          `json:"tool_name"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
}

const archivedSessionTag = "archived"

// tagFromLine 认官方实录里的 type=tag 行，最近一条有效。
func tagFromLine(line []byte) (string, bool) {
	var parsed struct {
		Type string `json:"type"`
		Tag  string `json:"tag"`
	}
	if err := json.Unmarshal(line, &parsed); err != nil || parsed.Type != "tag" {
		return "", false
	}
	return parsed.Tag, true
}

func titleFromLine(line []byte) string {
	var parsed ndjsonLine
	if err := json.Unmarshal(line, &parsed); err != nil {
		return ""
	}
	if parsed.Title != "" {
		return parsed.Title
	}
	if parsed.Type == "system" && parsed.Subtype == "title" {
		return parsed.Title
	}
	return ""
}

// usageFromLine 读官方 assistant.message.usage，used 不含 output。
func usageFromLine(line []byte) (TokenUsage, bool) {
	var parsed ndjsonLine
	if err := json.Unmarshal(line, &parsed); err != nil || parsed.Type != "assistant" {
		return TokenUsage{}, false
	}
	var body messageBody
	if err := json.Unmarshal(parsed.Message, &body); err != nil {
		return TokenUsage{}, false
	}
	used := body.Usage.InputTokens + body.Usage.CacheCreationInputTokens + body.Usage.CacheReadInputTokens
	if used <= 0 {
		return TokenUsage{}, false
	}
	return TokenUsage{Used: used, Window: contextWindowSize(body.Model)}, true
}

// contextWindowSize 对齐官方 vp/JL：默认 200k，模型名带 [1m] 则 1M。
func contextWindowSize(model string) int64 {
	if strings.Contains(strings.ToLower(model), "[1m]") {
		return 1_000_000
	}
	return 200_000
}

func progressFromLine(line []byte) (Progress, bool) {
	var parsed ndjsonLine
	if err := json.Unmarshal(line, &parsed); err != nil {
		return Progress{}, false
	}
	switch parsed.Type {
	case "user":
		text := messageText(parsed.Message)
		if text == "" {
			return Progress{}, false
		}
		return Progress{Kind: ProgressKindUser, Text: text, Paths: emptyStrings()}, true
	case "assistant":
		return assistantProgress(parsed.Message)
	case "system":
		if parsed.Subtype == "notice" || parsed.Subtype == "permission_denied" {
			return Progress{Kind: ProgressKindNotice, Text: parsed.Result, Paths: emptyStrings()}, true
		}
	}
	return Progress{}, false
}

func messageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var body messageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	}
	blocks := decodeBlocks(body.Content)
	var parts []string
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 && len(body.Content) > 0 && body.Content[0] == '"' {
		var s string
		_ = json.Unmarshal(body.Content, &s)
		return s
	}
	return strings.Join(parts, "\n")
}

func decodeBlocks(raw json.RawMessage) []contentBlock {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			return []contentBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}
	var one contentBlock
	if err := json.Unmarshal(raw, &one); err == nil && one.Type != "" {
		return []contentBlock{one}
	}
	return nil
}

func assistantProgress(raw json.RawMessage) (Progress, bool) {
	var body messageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return Progress{}, false
	}
	for _, block := range decodeBlocks(body.Content) {
		switch block.Type {
		case "thinking":
			text := block.Thinking
			if text == "" {
				text = block.Text
			}
			return Progress{Kind: ProgressKindReasoning, Text: text, Paths: emptyStrings()}, true
		case "text":
			if block.Text == "" {
				continue
			}
			return Progress{Kind: ProgressKindText, Text: block.Text, Paths: emptyStrings()}, true
		case "tool_use":
			return toolProgress(block), true
		}
	}
	return Progress{}, false
}

func toolProgress(block contentBlock) Progress {
	item := Progress{Kind: ProgressKindCommand, Command: block.Name, Paths: emptyStrings()}
	var input map[string]any
	_ = json.Unmarshal(block.Input, &input)
	switch block.Name {
	case "Bash":
		item.Kind = ProgressKindCommand
		item.Command = stringField(input, "command")
		item.Text = stringField(input, "description")
	case "Edit", "Write", "MultiEdit":
		item.Kind = ProgressKindFileChange
		path := stringField(input, "path")
		if path == "" {
			path = stringField(input, "file_path")
		}
		if path != "" {
			item.Paths = []string{path}
		}
		item.Diff = stringField(input, "new_string")
	case "TodoWrite", "Plan", "ExitPlanMode":
		item.Kind = ProgressKindPlan
		item.Text = stringField(input, "plan")
		if item.Text == "" {
			item.Text = block.Name
		}
	default:
		item.Kind = ProgressKindCommand
		item.Command = block.Name
	}
	return item
}

func stringField(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	v, ok := input[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func parseControlRequest(line []byte) (string, controlRequest, bool) {
	var parsed ndjsonLine
	if err := json.Unmarshal(line, &parsed); err != nil {
		return "", controlRequest{}, false
	}
	if parsed.Type != "control_request" {
		return "", controlRequest{}, false
	}
	var req controlRequest
	if err := json.Unmarshal(parsed.Request, &req); err != nil {
		return parsed.RequestID, req, false
	}
	return parsed.RequestID, req, true
}

func parseResultSessionID(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var parsed ndjsonLine
	if err := json.Unmarshal([]byte(body), &parsed); err == nil && parsed.SessionID != "" {
		return parsed.SessionID
	}
	scannerLines := strings.Split(body, "\n")
	for i := len(scannerLines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(scannerLines[i])
		if line == "" {
			continue
		}
		parsed = ndjsonLine{}
		if err := json.Unmarshal([]byte(line), &parsed); err == nil && parsed.SessionID != "" {
			return parsed.SessionID
		}
	}
	return ""
}

func askFromTool(req controlRequest) (ApprovalAsk, bool) {
	ask := ApprovalAsk{
		ExternalRequestID: "",
		Paths:             emptyStrings(),
		Options:           emptyStrings(),
		Fields:            emptyStrings(),
	}
	var input map[string]any
	_ = json.Unmarshal(req.Input, &input)
	switch req.ToolName {
	case "Bash":
		ask.Kind = AskKindCommand
		ask.Command = stringField(input, "command")
		return ask, true
	case "Edit", "Write", "MultiEdit":
		ask.Kind = AskKindFileChange
		path := stringField(input, "path")
		if path == "" {
			path = stringField(input, "file_path")
		}
		if path != "" {
			ask.Paths = []string{path}
		}
		ask.Diff = stringField(input, "new_string")
		return ask, true
	case "AskUserQuestion":
		ask.Kind = AskKindQuestion
		ask.Prompt = stringField(input, "question")
		if ask.Prompt == "" {
			ask.Prompt = stringField(input, "prompt")
		}
		ask.Options = stringList(input, "options")
		return ask, true
	default:
		if strings.HasPrefix(req.ToolName, "mcp__") || looksLikeForm(input) {
			ask.Kind = AskKindForm
			ask.Prompt = stringField(input, "prompt")
			ask.Fields = stringList(input, "fields")
			if len(ask.Fields) == 0 {
				for key := range input {
					ask.Fields = append(ask.Fields, key)
				}
			}
			return ask, true
		}
	}
	return ask, false
}

func looksLikeForm(input map[string]any) bool {
	if input == nil {
		return false
	}
	if _, ok := input["fields"]; ok {
		return true
	}
	_, hasPrompt := input["prompt"]
	_, hasValues := input["values"]
	return hasPrompt && hasValues
}

func stringList(input map[string]any, key string) []string {
	out := []string{}
	if input == nil {
		return out
	}
	raw, ok := input[key]
	if !ok {
		return out
	}
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			switch n := item.(type) {
			case string:
				out = append(out, n)
			case map[string]any:
				if label, ok := n["label"].(string); ok {
					out = append(out, label)
				}
			}
		}
	case []string:
		out = append(out, v...)
	}
	return out
}
