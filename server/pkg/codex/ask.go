package codex

import (
	"encoding/json"
	"strings"
)

// ParseAsk 把一条服务端请求编成本模块的反问。认不出则 ok=false。
func ParseAsk(msg Message) (ApprovalAsk, bool) {
	if msg.Kind != KindRequest || !KnownAskMethod(msg.Method) {
		return ApprovalAsk{}, false
	}
	ask := ApprovalAsk{
		ID:                msg.ID.String(),
		Method:            msg.Method,
		ExternalRequestID: msg.ID.String(),
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(msg.Params, &raw)
	ask.ThreadID = rawString(raw, "threadId")
	ask.TurnID = rawString(raw, "turnId")
	switch msg.Method {
	case MethodItemCommandApproval, MethodExecCommandApproval:
		ask.Kind = AskCommand
		ask.Command = firstNonEmpty(rawString(raw, "command"), commandFromNested(raw["command"]))
		if ask.Command == "" {
			ask.Command = rawString(raw, "cmd")
		}
	case MethodItemFileApproval, MethodApplyPatchApproval:
		ask.Kind = AskFileChange
		ask.Paths, ask.Diff = fileChangeFromParams(raw, msg.Params)
	case MethodItemPermissionsApproval:
		ask.Kind = AskPermissions
		ask.Prompt = firstNonEmpty(rawString(raw, "reason"), "Codex 请求额外权限")
	case MethodItemToolUserInput:
		ask.Kind = AskQuestion
		ask.Questions = questionsFromParams(raw)
		if len(ask.Questions) > 0 {
			ask.Prompt = firstNonEmpty(ask.Questions[0].Header, ask.Questions[0].Prompt)
		}
		for _, q := range ask.Questions {
			if q.ID != "" {
				ask.Fields = append(ask.Fields, q.ID)
			}
			for _, opt := range q.Options {
				ask.Options = append(ask.Options, firstNonEmpty(opt.Label, opt.ID))
			}
		}
	case MethodMCPElicitation:
		ask.Kind = AskForm
		ask.Prompt = firstNonEmpty(rawString(raw, "serverName"), "MCP 表单")
		ask.Fields = formFieldsFromParams(raw, msg.Params)
	}
	return ask, true
}

// ReplyBody 按官方 schema 把人的作答编成回包。
func ReplyBody(ask ApprovalAsk, answer AskAnswer) any {
	accepted := answer.Approved
	session := answer.Scope == ScopeSession
	switch ask.Method {
	case MethodItemCommandApproval:
		return map[string]any{"decision": commandDecision(accepted, session)}
	case MethodItemFileApproval:
		return map[string]any{"decision": fileDecision(accepted, session)}
	case MethodItemPermissionsApproval:
		return map[string]any{
			"permissions": map[string]any{},
			"scope":       permissionScope(accepted, session),
		}
	case MethodItemToolUserInput:
		return map[string]any{"answers": userInputAnswers(ask, answer)}
	case MethodMCPElicitation:
		action := "decline"
		if accepted {
			action = "accept"
		}
		body := map[string]any{"action": action}
		if accepted {
			body["content"] = formContent(ask, answer)
		}
		return body
	case MethodExecCommandApproval, MethodApplyPatchApproval:
		decision := "denied"
		if accepted {
			if session {
				decision = "approved_for_session"
			} else {
				decision = "approved"
			}
		}
		return map[string]any{"decision": decision}
	default:
		return map[string]any{"decision": "decline"}
	}
}

func commandDecision(accepted, session bool) any {
	if !accepted {
		return "decline"
	}
	if session {
		return "acceptForSession"
	}
	return "accept"
}

func fileDecision(accepted, session bool) any {
	if !accepted {
		return "decline"
	}
	if session {
		return "acceptForSession"
	}
	return "accept"
}

func permissionScope(accepted, session bool) string {
	if !accepted {
		return "turn"
	}
	if session {
		return "session"
	}
	return "turn"
}

func userInputAnswers(ask ApprovalAsk, answer AskAnswer) map[string]any {
	out := map[string]any{}
	if len(answer.Answers) > 0 {
		if len(ask.Questions) > 0 {
			for _, q := range ask.Questions {
				id := firstNonEmpty(q.ID, "answer")
				if text := strings.TrimSpace(answer.Answers[q.ID]); text != "" {
					out[id] = map[string]any{"answers": []string{text}}
				} else if text := strings.TrimSpace(answer.Answers[id]); text != "" {
					out[id] = map[string]any{"answers": []string{text}}
				}
			}
			if len(out) > 0 {
				return out
			}
		}
		for id, text := range answer.Answers {
			if strings.TrimSpace(text) == "" {
				continue
			}
			out[firstNonEmpty(id, "answer")] = map[string]any{"answers": []string{text}}
		}
		if len(out) > 0 {
			return out
		}
	}
	values := answer.Values
	if answer.Choice != "" {
		values = []string{answer.Choice}
	}
	if len(ask.Fields) == 0 {
		out["answer"] = map[string]any{"answers": values}
		return out
	}
	for i, field := range ask.Fields {
		item := []string{}
		if i < len(values) {
			item = []string{values[i]}
		} else if answer.Choice != "" && i == 0 {
			item = []string{answer.Choice}
		}
		out[field] = map[string]any{"answers": item}
	}
	return out
}

func formContent(ask ApprovalAsk, answer AskAnswer) map[string]any {
	out := map[string]any{}
	for i, field := range ask.Fields {
		if i < len(answer.Values) {
			out[field] = answer.Values[i]
		}
	}
	if len(out) == 0 && answer.Choice != "" {
		out["value"] = answer.Choice
	}
	return out
}

func rawString(raw map[string]json.RawMessage, key string) string {
	b, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		return s
	}
	return ""
}

func commandFromNested(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if s, _ := obj["command"].(string); s != "" {
		return s
	}
	return ""
}

func fileChangeFromParams(raw map[string]json.RawMessage, params json.RawMessage) ([]string, string) {
	var paths []string
	var diff string
	if b, ok := raw["grantRoot"]; ok {
		var root string
		if json.Unmarshal(b, &root) == nil && root != "" {
			paths = append(paths, root)
		}
	}
	var envelope struct {
		Changes []struct {
			Path string `json:"path"`
			Diff string `json:"diff"`
		} `json:"changes"`
		FileChanges []struct {
			Path string `json:"path"`
			Diff string `json:"diff"`
		} `json:"fileChanges"`
	}
	_ = json.Unmarshal(params, &envelope)
	for _, ch := range envelope.Changes {
		if ch.Path != "" {
			paths = append(paths, ch.Path)
		}
		if ch.Diff != "" {
			diff += ch.Diff
		}
	}
	for _, ch := range envelope.FileChanges {
		if ch.Path != "" {
			paths = append(paths, ch.Path)
		}
		if ch.Diff != "" {
			diff += ch.Diff
		}
	}
	return paths, diff
}

func questionsFromParams(raw map[string]json.RawMessage) []UserQuestion {
	b, ok := raw["questions"]
	if !ok {
		prompt := rawString(raw, "prompt")
		if prompt == "" {
			return nil
		}
		return []UserQuestion{{Prompt: prompt}}
	}
	var questions []map[string]any
	if err := json.Unmarshal(b, &questions); err != nil {
		return nil
	}
	out := make([]UserQuestion, 0, len(questions))
	for _, q := range questions {
		id, _ := q["id"].(string)
		header, _ := q["header"].(string)
		question, _ := q["question"].(string)
		item := UserQuestion{
			ID:     id,
			Header: header,
			Prompt: question,
		}
		if opts, ok := q["options"].([]any); ok {
			for _, opt := range opts {
				if parsed, ok := parseAskOption(opt); ok {
					item.Options = append(item.Options, parsed)
				}
			}
		}
		if item.ID != "" || item.Header != "" || item.Prompt != "" || len(item.Options) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func parseAskOption(raw any) (AskOption, bool) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return AskOption{}, false
		}
		return AskOption{ID: v, Label: v, Other: isOtherLabel(v)}, true
	case map[string]any:
		id, _ := v["id"].(string)
		label, _ := v["label"].(string)
		recommended, _ := v["recommended"].(bool)
		other, _ := v["isOther"].(bool)
		if !other {
			other, _ = v["other"].(bool)
		}
		label = firstNonEmpty(label, id)
		id = firstNonEmpty(id, label)
		if label == "" {
			return AskOption{}, false
		}
		return AskOption{
			ID:          id,
			Label:       label,
			Recommended: recommended,
			Other:       other || isOtherLabel(label),
		}, true
	default:
		return AskOption{}, false
	}
}

func isOtherLabel(label string) bool {
	switch strings.TrimSpace(label) {
	case "别的意思", "其他", "其它", "Other", "other":
		return true
	default:
		return false
	}
}

func formFieldsFromParams(raw map[string]json.RawMessage, params json.RawMessage) []string {
	if b, ok := raw["requestedSchema"]; ok {
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		if json.Unmarshal(b, &schema) == nil {
			fields := make([]string, 0, len(schema.Properties))
			for k := range schema.Properties {
				fields = append(fields, k)
			}
			return fields
		}
	}
	var envelope map[string]any
	_ = json.Unmarshal(params, &envelope)
	if msg, ok := envelope["message"].(map[string]any); ok {
		if schema, ok := msg["requestedSchema"].(map[string]any); ok {
			if props, ok := schema["properties"].(map[string]any); ok {
				fields := make([]string, 0, len(props))
				for k := range props {
					fields = append(fields, k)
				}
				return fields
			}
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
