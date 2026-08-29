package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"codedock/internal/agent/memory"
	cderr "codedock/internal/errors"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
)

// OverBudgetFunc 在目录超限写入后入队后台压缩，不阻塞工具返回。
type OverBudgetFunc func(key memory.TextMemoryKey)

type readTool struct {
	q *sqlite.Queries
}

type writeTool struct {
	q            *sqlite.Queries
	onOverBudget OverBudgetFunc
}

type searchTool struct {
	q *sqlite.Queries
}

type memoryReadInput struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

type memoryWriteInput struct {
	Scope   string `json:"scope"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type memorySearchInput struct {
	Query string `json:"query"`
}

type memoryItemOutput struct {
	Scope      string `json:"scope"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Content    string `json:"content"`
	ByteLen    int    `json:"byte_len"`
	OverBudget bool   `json:"over_budget"`
}

type memorySearchOutput struct {
	Hits []memorySearchHit `json:"hits"`
}

type memorySearchHit struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	SessionID   string  `json:"session_id"`
	RunID       string  `json:"run_id,omitempty"`
	Role        string  `json:"role"`
	Content     string  `json:"content"`
	Rank        float64 `json:"rank"`
}

// ReadTool 按 scope 与 name 读取目录或专题。
func ReadTool(q *sqlite.Queries) tool.Tool {
	return readTool{q: q}
}

// WriteTool 覆盖写入一篇目录或专题；目录超限时回调 onOverBudget。
func WriteTool(q *sqlite.Queries, onOverBudget OverBudgetFunc) tool.Tool {
	return writeTool{q: q, onOverBudget: onOverBudget}
}

// SearchTool 按关键词检索同一工作区的 context message。
func SearchTool(q *sqlite.Queries) tool.Tool {
	return searchTool{q: q}
}

func (readTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "memory_read",
		Prompt:           "Read a memory index or topic by scope and name. Use name \"index\" for the directory.",
		ParametersSchema: schemaOf[memoryReadInput](),
		OutputSchema:     schemaOf[memoryItemOutput](),
		Permission: tool.Permission{
			Capabilities: []tool.Capability{tool.CapabilityMemory},
		},
		Version: "1",
	}
}

func (writeTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "memory_write",
		Prompt:           "Overwrite a memory index or topic by scope and name. Use name \"index\" for the directory.",
		ParametersSchema: schemaOf[memoryWriteInput](),
		OutputSchema:     schemaOf[memoryItemOutput](),
		Permission: tool.Permission{
			Capabilities:     []tool.Capability{tool.CapabilityMemory},
			RequiresApproval: true,
		},
		Version: "1",
	}
}

func (searchTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "memory_search",
		Prompt:           "Search indexed messages in the current workspace by keyword.",
		ParametersSchema: schemaOf[memorySearchInput](),
		OutputSchema:     schemaOf[memorySearchOutput](),
		Permission: tool.Permission{
			Capabilities: []tool.Capability{tool.CapabilityMemory},
		},
		Version: "1",
	}
}

func (t readTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	args, err := unmarshalArgs[memoryReadInput](input.Call.Arguments)
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	scopeID, err := scopeIDFromSession(ctx, t.q, input.SessionID, memory.TextMemoryScope(args.Scope))
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	item, err := memory.Get(ctx, t.q, memory.TextMemoryKey{
		Scope:   memory.TextMemoryScope(args.Scope),
		ScopeID: scopeID,
		Name:    args.Name,
	})
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	return okResult(input, t.Definition().Name, memoryItemOutput{
		Scope:      string(item.Scope),
		Name:       item.Name,
		Kind:       string(item.Kind),
		Content:    item.Content,
		ByteLen:    item.ByteLen,
		OverBudget: item.OverBudget,
	})
}

func (t writeTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	args, err := unmarshalArgs[memoryWriteInput](input.Call.Arguments)
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	scope := memory.TextMemoryScope(args.Scope)
	scopeID, err := scopeIDFromSession(ctx, t.q, input.SessionID, scope)
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	item, err := memory.Upsert(ctx, t.q, memory.TextMemory{
		Scope:   scope,
		ScopeID: scopeID,
		Kind:    memory.KindFromName(args.Name),
		Name:    args.Name,
		Content: args.Content,
	})
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	result, err := okResult(input, t.Definition().Name, memoryItemOutput{
		Scope:      string(item.Scope),
		Name:       item.Name,
		Kind:       string(item.Kind),
		Content:    item.Content,
		ByteLen:    item.ByteLen,
		OverBudget: item.OverBudget,
	})
	if item.Kind == memory.KindIndex && item.OverBudget && t.onOverBudget != nil {
		t.onOverBudget(memory.TextMemoryKey{Scope: item.Scope, ScopeID: item.ScopeID, Kind: item.Kind, Name: item.Name})
	}
	return result, err
}

func (t searchTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	args, err := unmarshalArgs[memorySearchInput](input.Call.Arguments)
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	session, err := t.q.GetSession(ctx, input.SessionID)
	if err != nil {
		return failResult(input, t.Definition().Name, wrapSessionErr(err)), err
	}
	workspaceID := session.WorkspaceID
	if workspaceID == "" {
		workspaceID = "default"
	}
	hits, err := memory.SearchMessages(ctx, t.q, memory.Search{WorkspaceID: workspaceID, Query: args.Query})
	if err != nil {
		return failResult(input, t.Definition().Name, err), err
	}
	out := memorySearchOutput{Hits: make([]memorySearchHit, 0, len(hits))}
	for _, hit := range hits {
		out.Hits = append(out.Hits, memorySearchHit{
			ID:          hit.Message.ID,
			WorkspaceID: hit.Message.WorkspaceID,
			SessionID:   hit.Message.SessionID,
			RunID:       hit.Message.RunID,
			Role:        hit.Message.Role,
			Content:     hit.Message.Content,
			Rank:        hit.Rank,
		})
	}
	return okResult(input, t.Definition().Name, out)
}

func unmarshalArgs[T any](raw json.RawMessage) (T, error) {
	var args T
	if len(raw) == 0 {
		return args, cderr.Invalid("arguments required")
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return args, cderr.Invalid("%s", err.Error())
	}
	return args, nil
}

func scopeIDFromSession(ctx context.Context, q *sqlite.Queries, sessionID string, scope memory.TextMemoryScope) (string, error) {
	if q == nil {
		return "", cderr.Invalid("queries required")
	}
	if sessionID == "" {
		return "", cderr.Invalid("session_id is required")
	}
	session, err := q.GetSession(ctx, sessionID)
	if err != nil {
		return "", wrapSessionErr(err)
	}
	switch scope {
	case memory.ScopeUser:
		return session.UserID, nil
	case memory.ScopeWorkspace:
		if session.WorkspaceID == "" {
			return "default", nil
		}
		return session.WorkspaceID, nil
	default:
		return "", cderr.Invalid("invalid memory scope")
	}
}

func wrapSessionErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) || cderr.IsNotFound(err) {
		return cderr.NotFound("session not found")
	}
	return err
}

func failResult(input tool.Input, name string, err error) tool.Result {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return tool.Result{CallID: input.Call.ID, Name: name, Success: false, Error: msg}
}

func okResult(input tool.Input, name string, body any) (tool.Result, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return failResult(input, name, err), err
	}
	return tool.Result{CallID: input.Call.ID, Name: name, Success: true, Output: raw}, nil
}
