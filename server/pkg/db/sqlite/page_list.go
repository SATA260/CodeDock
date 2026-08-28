package sqlite

import (
	"context"
	"database/sql"
)

// sqlc 不会把 ORDER BY CASE WHEN 中的占位符收成参数，分页列表用手写绑定，禁止拼接列名。

type ListSessionsParams struct {
	SortBy    string
	SortOrder string
	Limit     int64
	Offset    int64
}

type ListSessionMessagesPageParams struct {
	SessionID string
	SortBy    string
	SortOrder string
	Limit     int64
	Offset    int64
}

type ListSessionApprovalsParams struct {
	SessionID string
	SortBy    string
	SortOrder string
	Limit     int64
	Offset    int64
}

type ListUsageByRunParams struct {
	RunID     string
	SortBy    string
	SortOrder string
	Limit     int64
	Offset    int64
}

type ListUsageBySessionParams struct {
	SessionID string
	SortBy    string
	SortOrder string
	Limit     int64
	Offset    int64
}

const listSessions = `
SELECT id, tenant_id, user_id, agent_id, status, active_run_id, last_event_seq, compaction_seq, created_at, updated_at, workspace_id
FROM sessions
ORDER BY
  CASE WHEN ? = 'updated_at' AND ? = 'asc' THEN updated_at END ASC,
  CASE WHEN ? = 'updated_at' AND ? = 'desc' THEN updated_at END DESC,
  CASE WHEN ? = 'created_at' AND ? = 'asc' THEN created_at END ASC,
  CASE WHEN ? = 'created_at' AND ? = 'desc' THEN created_at END DESC,
  id ASC
LIMIT ? OFFSET ?
`

const listSessionMessagesPage = `
SELECT id, session_id, run_id, turn_id, role, content, attachments, tool_calls, event_seq, created_at
FROM messages
WHERE session_id = ?
ORDER BY
  CASE WHEN ? = 'event_seq' AND ? = 'asc' THEN event_seq END ASC,
  CASE WHEN ? = 'event_seq' AND ? = 'desc' THEN event_seq END DESC,
  CASE WHEN ? = 'created_at' AND ? = 'asc' THEN created_at END ASC,
  CASE WHEN ? = 'created_at' AND ? = 'desc' THEN created_at END DESC,
  id ASC
LIMIT ? OFFSET ?
`

const listSessionApprovals = `
SELECT id, session_id, run_id, tool_call_id, scope, status, expires_at
FROM approvals
WHERE session_id = ?
ORDER BY
  CASE WHEN ? = 'id' AND ? = 'asc' THEN id END ASC,
  CASE WHEN ? = 'id' AND ? = 'desc' THEN id END DESC,
  CASE WHEN ? = 'status' AND ? = 'asc' THEN status END ASC,
  CASE WHEN ? = 'status' AND ? = 'desc' THEN status END DESC,
  CASE WHEN ? = 'expires_at' AND ? = 'asc' THEN expires_at END ASC,
  CASE WHEN ? = 'expires_at' AND ? = 'desc' THEN expires_at END DESC,
  id ASC
LIMIT ? OFFSET ?
`

const listUsageByRun = `
SELECT id, session_id, run_id, turn_id, request_id, provider, model, usage_type, cache_creation_input_tokens, cache_read_input_tokens, output_tokens, reasoning_tokens, total_tokens, estimated, raw_provider_usage, created_at
FROM usage_records
WHERE run_id = ?
ORDER BY
  CASE WHEN ? = 'created_at' AND ? = 'asc' THEN created_at END ASC,
  CASE WHEN ? = 'created_at' AND ? = 'desc' THEN created_at END DESC,
  id ASC
LIMIT ? OFFSET ?
`

const listUsageBySession = `
SELECT id, session_id, run_id, turn_id, request_id, provider, model, usage_type, cache_creation_input_tokens, cache_read_input_tokens, output_tokens, reasoning_tokens, total_tokens, estimated, raw_provider_usage, created_at
FROM usage_records
WHERE session_id = ?
ORDER BY
  CASE WHEN ? = 'created_at' AND ? = 'asc' THEN created_at END ASC,
  CASE WHEN ? = 'created_at' AND ? = 'desc' THEN created_at END DESC,
  id ASC
LIMIT ? OFFSET ?
`

func sortArgs(sortBy, sortOrder string, pairs int) []any {
	args := make([]any, 0, pairs*2)
	for i := 0; i < pairs; i++ {
		args = append(args, sortBy, sortOrder)
	}
	return args
}

func queryArgs(prefix []any, sortBy, sortOrder string, pairs int, limit, offset int64) []any {
	args := append(prefix, sortArgs(sortBy, sortOrder, pairs)...)
	return append(args, limit, offset)
}

// ListSessions 按白名单字段分页列出会话。
func (q *Queries) ListSessions(ctx context.Context, arg ListSessionsParams) ([]Session, error) {
	rows, err := q.db.QueryContext(ctx, listSessions, queryArgs(nil, arg.SortBy, arg.SortOrder, 4, arg.Limit, arg.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Session
	for rows.Next() {
		var i Session
		if err := rows.Scan(
			&i.ID,
			&i.TenantID,
			&i.UserID,
			&i.AgentID,
			&i.Status,
			&i.ActiveRunID,
			&i.LastEventSeq,
			&i.CompactionSeq,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.WorkspaceID,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// ListSessionMessagesPage 按白名单字段分页列出会话消息。
func (q *Queries) ListSessionMessagesPage(ctx context.Context, arg ListSessionMessagesPageParams) ([]Message, error) {
	rows, err := q.db.QueryContext(ctx, listSessionMessagesPage, queryArgs([]any{arg.SessionID}, arg.SortBy, arg.SortOrder, 4, arg.Limit, arg.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Message
	for rows.Next() {
		var i Message
		if err := rows.Scan(
			&i.ID,
			&i.SessionID,
			&i.RunID,
			&i.TurnID,
			&i.Role,
			&i.Content,
			&i.Attachments,
			&i.ToolCalls,
			&i.EventSeq,
			&i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// ListSessionApprovals 按白名单字段分页列出会话审批。
func (q *Queries) ListSessionApprovals(ctx context.Context, arg ListSessionApprovalsParams) ([]Approval, error) {
	rows, err := q.db.QueryContext(ctx, listSessionApprovals, queryArgs([]any{arg.SessionID}, arg.SortBy, arg.SortOrder, 6, arg.Limit, arg.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Approval
	for rows.Next() {
		var i Approval
		if err := rows.Scan(
			&i.ID,
			&i.SessionID,
			&i.RunID,
			&i.ToolCallID,
			&i.Scope,
			&i.Status,
			&i.ExpiresAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func scanUsageRows(rows *sql.Rows) ([]UsageRecord, error) {
	var items []UsageRecord
	for rows.Next() {
		var i UsageRecord
		if err := rows.Scan(
			&i.ID,
			&i.SessionID,
			&i.RunID,
			&i.TurnID,
			&i.RequestID,
			&i.Provider,
			&i.Model,
			&i.UsageType,
			&i.CacheCreationInputTokens,
			&i.CacheReadInputTokens,
			&i.OutputTokens,
			&i.ReasoningTokens,
			&i.TotalTokens,
			&i.Estimated,
			&i.RawProviderUsage,
			&i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// ListUsageByRun 按白名单字段分页列出 Run 用量。
func (q *Queries) ListUsageByRun(ctx context.Context, arg ListUsageByRunParams) ([]UsageRecord, error) {
	rows, err := q.db.QueryContext(ctx, listUsageByRun, queryArgs([]any{arg.RunID}, arg.SortBy, arg.SortOrder, 2, arg.Limit, arg.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsageRows(rows)
}

// ListUsageBySession 按白名单字段分页列出会话用量。
func (q *Queries) ListUsageBySession(ctx context.Context, arg ListUsageBySessionParams) ([]UsageRecord, error) {
	rows, err := q.db.QueryContext(ctx, listUsageBySession, queryArgs([]any{arg.SessionID}, arg.SortBy, arg.SortOrder, 2, arg.Limit, arg.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsageRows(rows)
}
