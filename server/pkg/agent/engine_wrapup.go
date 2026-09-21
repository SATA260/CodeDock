package agent

import (
	"encoding/json"
	"strings"
)

const wrapUpDeveloperNote = "验证和复审已通过。用几段话说明改了什么、改了哪些路径、怎么验证。不要再调工具。"

// shouldComposeWrapUp 判断收束时是否还缺一条面向用户的收尾说明。
func shouldComposeWrapUp(state AgentState, payload FinishPayload) bool {
	if payload.Status == RunCancelled || payload.Status == RunFailed {
		return false
	}
	if payload.Reason == StopCancelled || payload.Reason == StopApprovalDenied {
		return false
	}
	if state.WrapUpPending {
		return false
	}
	return state.HadSideEffects || strings.TrimSpace(state.LastEvaluateSummary) != ""
}

// composeWrapUpText 用改动文件、验证摘要和复审摘要拼一段收尾说明。
func composeWrapUpText(e *Engine, state AgentState, messages []Message) string {
	var b strings.Builder
	b.WriteString("已完成。")
	files := collectChangedPaths(e, state, messages)
	if len(files) > 0 {
		b.WriteString("\n改动文件：")
		for _, path := range files {
			b.WriteString("\n- ")
			b.WriteString(path)
		}
	}
	if summary := strings.TrimSpace(state.LastVerifySummary); summary != "" {
		b.WriteString("\n验证：")
		b.WriteString(clipWrapUpPart(summary, 200))
	}
	if summary := strings.TrimSpace(state.LastEvaluateSummary); summary != "" {
		b.WriteString("\n复审：")
		b.WriteString(clipWrapUpPart(summary, 200))
	}
	return b.String()
}

// collectChangedPaths 优先读快照改动，没有则从本 Run 的 write/edit 调用收集路径。
func collectChangedPaths(e *Engine, state AgentState, messages []Message) []string {
	if e != nil && e.snapshots != nil && strings.TrimSpace(state.WorkspaceRoot) != "" {
		if files, err := e.snapshots.ChangedFiles(state.WorkspaceRoot, SnapshotFromState(state)); err == nil && len(files) > 0 {
			return files
		}
	}
	return collectWriteEditPaths(messages)
}

// collectWriteEditPaths 从助手工具调用里抽出 write/edit 的 path。
func collectWriteEditPaths(messages []Message) []string {
	seen := make(map[string]bool)
	var files []string
	for _, msg := range messages {
		if msg.Role != RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			if call.Name != "write" && call.Name != "edit" {
				continue
			}
			var args struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(call.Arguments, &args) != nil {
				continue
			}
			path := strings.TrimSpace(args.Path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			files = append(files, path)
		}
	}
	return files
}

// wrapUpAssistantFacts 把拼装出的收尾说明写成助手消息和对应事件。
func wrapUpAssistantFacts(state AgentState, text string) (Message, []Fact) {
	msgID := newEntityID()
	msg := Message{
		ID:        msgID,
		SessionID: state.SessionID,
		RunID:     ptrValue(state.RunID),
		TurnID:    state.TurnID,
		Role:      RoleAssistant,
		Content:   EncodeText(text),
	}
	return msg, []Fact{
		{
			Type:    EventAssistantStarted,
			TurnID:  state.TurnID,
			Payload: MarshalPayload(AssistantStartedPayload{MessageID: msgID}),
		},
		{
			Type:   EventAssistantCompleted,
			TurnID: state.TurnID,
			Payload: MarshalPayload(AssistantCompletedPayload{
				MessageID: msgID,
				Text:      text,
			}),
		},
	}
}

// clipWrapUpPart 把过长的验证/复审摘要截到可读长度。
func clipWrapUpPart(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "…"
}
