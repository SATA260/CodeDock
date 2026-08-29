package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
)

// wrapHandlerDB 把 sql.ErrNoRows 转成 NotFound。
func wrapHandlerDB(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return cderr.NotFound("%s", err.Error())
	}
	return err
}

// nullString 把空字符串转成无效的 sql.NullString。
func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// deref 解引用字符串指针，nil 返回空串。
func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
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
	if row.Config != "" {
		_ = json.Unmarshal([]byte(row.Config), &config)
	}
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

// mapMessage 把 sqlc Message 行映射为领域对象。
func mapMessage(row sqlite.Message) pkgagent.Message {
	var attachments []pkgagent.Attachment
	if row.Attachments.Valid && row.Attachments.String != "" {
		_ = json.Unmarshal([]byte(row.Attachments.String), &attachments)
	}
	var calls []tool.Call
	if row.ToolCalls.Valid && row.ToolCalls.String != "" {
		_ = json.Unmarshal([]byte(row.ToolCalls.String), &calls)
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
	if row.ToolCalls != "" {
		_ = json.Unmarshal([]byte(row.ToolCalls), &calls)
	}
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

// rawJSON 把空字符串规范成 JSON null。
func rawJSON(value string) json.RawMessage {
	if value == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(value)
}

// ptrTime 把可空时间字符串解析成 *time.Time。
func ptrTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed := parseTime(value.String)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}
