package codex

import (
	"encoding/json"
	"strings"
)

// ProgressEvent 是一条通知映出来的进展，带所属 thread/turn。
type ProgressEvent struct {
	ThreadID string
	TurnID   string
	Progress Progress
}

// MapNotification 把官方通知映成给人看的进展。对不上的返回 false。
func MapNotification(msg Message) (ProgressEvent, bool) {
	if msg.Kind != KindNotification {
		return ProgressEvent{}, false
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(msg.Params, &raw)
	ev := ProgressEvent{
		ThreadID: rawString(raw, "threadId"),
		TurnID:   rawString(raw, "turnId"),
	}
	switch msg.Method {
	case MethodAgentMessageDelta:
		ev.Progress = Progress{Kind: ProgressText, ItemID: rawString(raw, "itemId"), Text: rawString(raw, "delta")}
		return ev, ev.Progress.Text != ""
	case MethodReasoningTextDelta, MethodReasoningSummaryDelta:
		text := firstNonEmpty(rawString(raw, "delta"), rawString(raw, "text"))
		ev.Progress = Progress{Kind: ProgressReasoning, ItemID: rawString(raw, "itemId"), Text: text}
		return ev, text != ""
	case MethodCommandOutputDelta:
		ev.Progress = Progress{Kind: ProgressCommand, ItemID: rawString(raw, "itemId"), Text: rawString(raw, "delta")}
		return ev, ev.Progress.Text != ""
	case MethodFileChangeDelta:
		ev.Progress = Progress{Kind: ProgressFileChange, ItemID: rawString(raw, "itemId"), Diff: rawString(raw, "delta")}
		return ev, ev.Progress.Diff != ""
	case MethodPlanDelta:
		ev.Progress = Progress{Kind: ProgressPlan, ItemID: rawString(raw, "itemId"), Text: rawString(raw, "delta")}
		return ev, ev.Progress.Text != ""
	case MethodItemStarted, MethodItemCompleted:
		item, ok := MapItem(raw["item"])
		if !ok {
			return ProgressEvent{}, false
		}
		ev.Progress = item
		return ev, true
	case MethodError:
		ev.Progress = Progress{Kind: ProgressNotice, Text: firstNonEmpty(rawString(raw, "message"), string(msg.Params))}
		return ev, true
	default:
		if strings.HasPrefix(msg.Method, "item/") || msg.Method == MethodTurnCompleted || msg.Method == MethodTurnStarted {
			return ProgressEvent{}, false
		}
		return ProgressEvent{}, false
	}
}

// MapItem 把官方 ThreadItem 映成进展。
func MapItem(raw json.RawMessage) (Progress, bool) {
	if len(raw) == 0 {
		return Progress{}, false
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		return Progress{}, false
	}
	typ := rawString(item, "type")
	id := rawString(item, "id")
	switch typ {
	case "userMessage":
		return Progress{Kind: ProgressUser, ItemID: id, Text: userMessageText(item["content"])}, true
	case "agentMessage":
		return Progress{Kind: ProgressText, ItemID: id, Text: rawString(item, "text")}, true
	case "reasoning":
		return Progress{Kind: ProgressReasoning, ItemID: id, Text: strings.Join(rawStringSlice(item["summary"]), "\n")}, true
	case "commandExecution":
		return Progress{
			Kind:    ProgressCommand,
			ItemID:  id,
			Command: rawString(item, "command"),
			Text:    rawString(item, "aggregatedOutput"),
			Status:  rawString(item, "status"),
		}, true
	case "fileChange":
		paths, diff := fileChangeFromItem(item["changes"])
		return Progress{Kind: ProgressFileChange, ItemID: id, Paths: paths, Diff: diff, Status: rawString(item, "status")}, true
	case "plan":
		return Progress{Kind: ProgressPlan, ItemID: id, Text: rawString(item, "text")}, true
	default:
		if typ == "" {
			return Progress{}, false
		}
		return Progress{Kind: ProgressNotice, ItemID: id, Text: typ}, true
	}
}

// HydrateProgress 把 thread/read 里的 turns/items 编成可回放实录。
func HydrateProgress(turns []TurnObject) []Progress {
	out := make([]Progress, 0)
	for _, turn := range turns {
		for _, item := range turn.Items {
			if p, ok := MapItem(item); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

func userMessageText(content json.RawMessage) string {
	var items []UserInput
	if json.Unmarshal(content, &items) == nil {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func rawStringSlice(raw json.RawMessage) []string {
	var items []string
	if json.Unmarshal(raw, &items) == nil {
		return items
	}
	return nil
}

func fileChangeFromItem(raw json.RawMessage) ([]string, string) {
	var changes []struct {
		Path string `json:"path"`
		Diff string `json:"diff"`
	}
	if json.Unmarshal(raw, &changes) != nil {
		return nil, ""
	}
	paths := make([]string, 0, len(changes))
	var diff strings.Builder
	for _, ch := range changes {
		if ch.Path != "" {
			paths = append(paths, ch.Path)
		}
		diff.WriteString(ch.Diff)
	}
	return paths, diff.String()
}
