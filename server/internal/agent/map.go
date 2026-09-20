package agent

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

// wrapDB 把 sql.ErrNoRows 转成 NotFound，其余错误原样返回。
func wrapDB(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return cderr.NotFound("%s", err.Error())
	}
	return err
}

// nullString 把空串转成无效 NullString，非空则 Valid。
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

// ptrString 把有效且非空的 NullString 转成 *string。
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

// ptrTime 把有效时间字符串转成 *time.Time，无效则 nil。
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

// boolInt 把 bool 编成 SQLite 整型：true=1，false=0。
func boolInt(ok bool) int64 {
	if ok {
		return 1
	}
	return 0
}

// mapSession 把 sqlc Session 行映射为领域 Session。
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
		Summary:       row.Summary,
		CreatedAt:     parseTime(row.CreatedAt),
		UpdatedAt:     parseTime(row.UpdatedAt),
	}
}

// mapRun 把 sqlc Run 行映射为领域 Run，并反序列化 config。
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
		Mode:             pkgagent.WorkMode(row.Mode),
		Approval:         config.Approval,
		Config:           config,
		Status:           pkgagent.RunStatus(row.Status),
		CurrentTurnID:    ptrString(row.CurrentTurnID),
		StopReason:       reason,
		CancelRequested:  row.CancelRequested != 0,
		StartedAt:        ptrTime(row.StartedAt),
		FinishedAt:       ptrTime(row.FinishedAt),
	}
}

// mapMessage 把 sqlc Message 行映射为领域 Message。
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

// mapEvent 把 sqlc AgentEvent 行映射为领域事件。
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

// mapTurn 把 sqlc Turn 行映射为领域 Turn。
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

// mapApproval 把 sqlc Approval 行映射为领域审批；无 tool_calls 时回退到 tool_call_id。
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
		Kind:       pkgagent.NormalizeApprovalKind(pkgagent.ApprovalKind(row.Kind)),
	}
}

// mapCompaction 把 sqlc 压缩检查点行映射为领域对象。
func mapCompaction(row sqlite.CompactionCheckpoint) pkgagent.CompactionCheckpoint {
	return pkgagent.CompactionCheckpoint{
		ID:           row.ID,
		SessionID:    row.SessionID,
		BaseEventSeq: row.BaseEventSeq,
		Summary:      row.Summary,
		CreatedByRun: row.CreatedByRun,
		CreatedAt:    parseTime(row.CreatedAt),
	}
}

// mapToolCheckpoint 把 sqlc 工具检查点行反序列化为 ToolCheckpoint。
func mapToolCheckpoint(row sqlite.RunToolCheckpoint) pkgagent.ToolCheckpoint {
	cp := pkgagent.ToolCheckpoint{TurnID: row.TurnID}
	if row.CompletedCalls != "" {
		_ = json.Unmarshal([]byte(row.CompletedCalls), &cp.Completed)
	}
	if row.ApprovedCalls != "" {
		_ = json.Unmarshal([]byte(row.ApprovedCalls), &cp.Approved)
	}
	if row.DeniedCalls != "" {
		_ = json.Unmarshal([]byte(row.DeniedCalls), &cp.Denied)
	}
	if row.PendingCalls != "" {
		_ = json.Unmarshal([]byte(row.PendingCalls), &cp.Pending)
	}
	if row.Results != "" {
		_ = json.Unmarshal([]byte(row.Results), &cp.Results)
	}
	return cp
}

// marshalJSON 序列化值为 JSON 字符串；失败或 null 时写 "[]"。
func marshalJSON(value any) string {
	if value == nil {
		return "[]"
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	if string(body) == "null" {
		return "[]"
	}
	return string(body)
}

// formatTimePtr 把非零时间格式化为 RFC3339 NullString。
func formatTimePtr(value *time.Time) sql.NullString {
	if value == nil || value.IsZero() {
		return sql.NullString{}
	}
	return nullString(value.UTC().Format(time.RFC3339))
}
