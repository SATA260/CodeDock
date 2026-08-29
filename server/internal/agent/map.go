package agent

import (
	"database/sql"
	"encoding/json"
	"time"

	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
)

// nullString 把空字符串转成无效的 sql.NullString。
func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// nullTime 把时间指针格式化成可空字符串列。
func nullTime(value *time.Time) sql.NullString {
	if value == nil || value.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: util.FormatTime(*value), Valid: true}
}

// ptrTime 把可空时间字符串解析成 *time.Time。
func ptrTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

// ptrString 把有效的可空字符串转成 *string。
func ptrString(value sql.NullString) *string {
	if !value.Valid || value.String == "" {
		return nil
	}
	v := value.String
	return &v
}

// parseTime 按 RFC3339 解析时间，失败返回零值。
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// marshalJSON 把值编码成 JSON 字符串，失败返回 "null"。
func marshalJSON(v any) string {
	if v == nil {
		return "null"
	}
	body, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(body)
}

// unmarshalJSON 把 JSON 字符串解到 dest，忽略空串与解析错误。
func unmarshalJSON[T any](raw string, dest *T) {
	if raw == "" {
		return
	}
	_ = json.Unmarshal([]byte(raw), dest)
}

// mapSession 把 sqlc Session 行映射为领域对象。
func mapSession(row sqlite.Session) pkgagent.Session {
	return pkgagent.Session{
		ID:            row.ID,
		TenantID:      row.TenantID,
		UserID:        row.UserID,
		AgentID:       row.AgentID,
		WorkspaceID:   row.WorkspaceID,
		Status:        pkgagent.SessionStatus(row.Status),
		ActiveRunID:   ptrString(row.ActiveRunID),
		LastEventSeq:  row.LastEventSeq,
		CompactionSeq: row.CompactionSeq,
		CreatedAt:     parseTime(row.CreatedAt),
		UpdatedAt:     parseTime(row.UpdatedAt),
	}
}

// mapRun 把 sqlc Run 行映射为领域对象。
func mapRun(row sqlite.Run) pkgagent.Run {
	var config pkgagent.RunConfigSnapshot
	unmarshalJSON(row.Config, &config)
	var reason *pkgagent.StopReason
	if row.StopReason.Valid && row.StopReason.String != "" {
		value := pkgagent.StopReason(row.StopReason.String)
		reason = &value
	}
	return pkgagent.Run{
		ID:               row.ID,
		SessionID:        row.SessionID,
		TriggerMessageID: row.TriggerMessageID,
		Mode:             pkgagent.AgentMode(row.Mode),
		Config:           config,
		Status:           pkgagent.RunStatus(row.Status),
		CurrentTurnID:    ptrString(row.CurrentTurnID),
		StopReason:       reason,
		CancelRequested:  row.CancelRequested != 0,
		StartedAt:        ptrTime(row.StartedAt),
		FinishedAt:       ptrTime(row.FinishedAt),
	}
}

// mapTurn 把 sqlc Turn 行映射为领域对象。
func mapTurn(row sqlite.Turn) pkgagent.Turn {
	return pkgagent.Turn{
		ID:             row.ID,
		RunID:          row.RunID,
		Number:         int(row.Number),
		Status:         pkgagent.TurnStatus(row.Status),
		FirstEventSeq:  row.FirstEventSeq,
		LastEventSeq:   row.LastEventSeq,
		AssistantMsgID: ptrString(row.AssistantMsgID),
		UsageID:        ptrString(row.UsageID),
		StartedAt:      ptrTime(row.StartedAt),
		FinishedAt:     ptrTime(row.FinishedAt),
	}
}

// mapMessage 把 sqlc Message 行映射为领域对象。
func mapMessage(row sqlite.Message) pkgagent.Message {
	var attachments []pkgagent.Attachment
	if row.Attachments.Valid {
		unmarshalJSON(row.Attachments.String, &attachments)
	}
	var calls []tool.Call
	if row.ToolCalls.Valid {
		unmarshalJSON(row.ToolCalls.String, &calls)
	}
	return pkgagent.Message{
		ID:          row.ID,
		SessionID:   row.SessionID,
		RunID:       ptrString(row.RunID),
		TurnID:      ptrString(row.TurnID),
		Role:        pkgagent.MessageRole(row.Role),
		Content:     json.RawMessage(row.Content),
		Attachments: attachments,
		ToolCalls:   calls,
		EventSeq:    row.EventSeq,
		CreatedAt:   parseTime(row.CreatedAt),
	}
}

// mapEvent 把 sqlc AgentEvent 行映射为领域对象。
func mapEvent(row sqlite.AgentEvent) pkgagent.AgentEvent {
	return pkgagent.AgentEvent{
		EventID:    row.EventID,
		SessionID:  row.SessionID,
		RunID:      row.RunID,
		TurnID:     ptrString(row.TurnID),
		Seq:        row.Seq,
		Type:       pkgagent.EventType(row.Type),
		Version:    int(row.Version),
		OccurredAt: parseTime(row.OccurredAt),
		Payload:    json.RawMessage(row.Payload),
	}
}

// mapApproval 把 sqlc Approval 行映射为领域对象。
func mapApproval(row sqlite.Approval) pkgagent.Approval {
	var calls []pkgagent.ApprovalToolCall
	unmarshalJSON(row.ToolCalls, &calls)
	if len(calls) == 0 && row.ToolCallID != "" {
		calls = []pkgagent.ApprovalToolCall{{ID: row.ToolCallID}}
	}
	first := row.ToolCallID
	if first == "" && len(calls) > 0 {
		first = calls[0].ID
	}
	return pkgagent.Approval{
		ID:         row.ID,
		SessionID:  row.SessionID,
		RunID:      row.RunID,
		ToolCallID: first,
		ToolCalls:  calls,
		Scope:      pkgagent.ApprovalScope(row.Scope),
		Status:     pkgagent.ApprovalStatus(row.Status),
		ExpiresAt:  parseTime(row.ExpiresAt),
	}
}

// mapUsage 把 sqlc UsageRecord 行映射为领域对象。
func mapUsage(row sqlite.UsageRecord) pkgagent.UsageRecord {
	return pkgagent.UsageRecord{
		ID:                       row.ID,
		SessionID:                row.SessionID,
		RunID:                    row.RunID,
		TurnID:                   row.TurnID,
		RequestID:                row.RequestID,
		Provider:                 row.Provider,
		Model:                    row.Model,
		UsageType:                row.UsageType,
		CacheCreationInputTokens: row.CacheCreationInputTokens,
		CacheReadInputTokens:     row.CacheReadInputTokens,
		OutputTokens:             row.OutputTokens,
		ReasoningTokens:          row.ReasoningTokens,
		TotalTokens:              row.TotalTokens,
		Estimated:                row.Estimated != 0,
		RawProviderUsage:         rawJSON(row.RawProviderUsage.String),
		CreatedAt:                parseTime(row.CreatedAt),
	}
}

// mapCheckpoint 把 sqlc CompactionCheckpoint 行映射为领域对象。
func mapCheckpoint(row sqlite.CompactionCheckpoint) pkgagent.CompactionCheckpoint {
	return pkgagent.CompactionCheckpoint{
		ID:           row.ID,
		SessionID:    row.SessionID,
		BaseEventSeq: row.BaseEventSeq,
		Summary:      row.Summary,
		CreatedByRun: row.CreatedByRun,
		CreatedAt:    parseTime(row.CreatedAt),
	}
}

// runUpdateParams 把领域 Run 转成 UpdateRun 参数。
func runUpdateParams(run pkgagent.Run) sqlite.UpdateRunParams {
	var reason sql.NullString
	if run.StopReason != nil {
		reason = nullString(string(*run.StopReason))
	}
	cancel := int64(0)
	if run.CancelRequested {
		cancel = 1
	}
	var turnID sql.NullString
	if run.CurrentTurnID != nil {
		turnID = nullString(*run.CurrentTurnID)
	}
	return sqlite.UpdateRunParams{
		Status:          string(run.Status),
		CurrentTurnID:   turnID,
		StopReason:      reason,
		CancelRequested: cancel,
		StartedAt:       nullTime(run.StartedAt),
		FinishedAt:      nullTime(run.FinishedAt),
		ID:              run.ID,
	}
}

// turnUpdateParams 把领域 Turn 转成 UpdateTurn 参数。
func turnUpdateParams(turn pkgagent.Turn) sqlite.UpdateTurnParams {
	var assistant, usage sql.NullString
	if turn.AssistantMsgID != nil {
		assistant = nullString(*turn.AssistantMsgID)
	}
	if turn.UsageID != nil {
		usage = nullString(*turn.UsageID)
	}
	return sqlite.UpdateTurnParams{
		Status:         string(turn.Status),
		FirstEventSeq:  turn.FirstEventSeq,
		LastEventSeq:   turn.LastEventSeq,
		AssistantMsgID: assistant,
		UsageID:        usage,
		StartedAt:      nullTime(turn.StartedAt),
		FinishedAt:     nullTime(turn.FinishedAt),
		ID:             turn.ID,
	}
}

// rawJSON 把空字符串规范成 JSON null。
func rawJSON(value string) json.RawMessage {
	if value == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(value)
}

// deref 解引用字符串指针，nil 返回空串。
func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
