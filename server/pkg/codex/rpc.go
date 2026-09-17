package codex

import (
	"context"
	"encoding/json"
)

// ThreadStartParams 是 thread/start 的入参。字段名跟官方 camelCase 对齐。
type ThreadStartParams struct {
	Cwd            string `json:"cwd,omitempty"`
	Model          string `json:"model,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	Sandbox        string `json:"sandbox,omitempty"`
	Ephemeral      bool   `json:"ephemeral,omitempty"`
}

// ThreadIDParams 只带 threadId。
type ThreadIDParams struct {
	ThreadID string `json:"threadId"`
}

// ThreadResumeParams 是 thread/resume 的入参。
type ThreadResumeParams struct {
	ThreadID       string `json:"threadId"`
	Cwd            string `json:"cwd,omitempty"`
	Model          string `json:"model,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	Sandbox        string `json:"sandbox,omitempty"`
}

// ThreadForkParams 是 thread/fork 的入参。
type ThreadForkParams struct {
	ThreadID  string `json:"threadId"`
	Cwd       string `json:"cwd,omitempty"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
}

// ThreadNameParams 是 thread/name/set 的入参。
type ThreadNameParams struct {
	ThreadID string `json:"threadId"`
	Name     string `json:"name"`
}

// ThreadListParams 是 thread/list 的入参。
type ThreadListParams struct {
	Archived *bool  `json:"archived,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// ThreadReadParams 是 thread/read 的入参。
type ThreadReadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns bool   `json:"includeTurns"`
}

// TurnStartParams 是 turn/start 的入参。
type TurnStartParams struct {
	ThreadID          string          `json:"threadId"`
	Input             []UserInput     `json:"input"`
	Model             string          `json:"model,omitempty"`
	Effort            string          `json:"effort,omitempty"`
	ApprovalPolicy    string          `json:"approvalPolicy,omitempty"`
	Cwd               string          `json:"cwd,omitempty"`
	SandboxPolicy     json.RawMessage `json:"sandboxPolicy,omitempty"`
	CollaborationMode json.RawMessage `json:"collaborationMode,omitempty"`
}

// TurnInterruptParams 是 turn/interrupt 的入参。
type TurnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

// ReviewStartParams 是 review/start 的入参。默认评审工作区未提交改动。
type ReviewStartParams struct {
	ThreadID string         `json:"threadId"`
	Target   map[string]any `json:"target"`
}

// ModelListParams 是 model/list 的入参。
type ModelListParams struct {
	Cursor        string `json:"cursor,omitempty"`
	IncludeHidden *bool  `json:"includeHidden,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

// CursorListParams 是带分页游标的列表入参。
type CursorListParams struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// ConfigReadParams 是 config/read 的入参。
type ConfigReadParams struct {
	Cwd string `json:"cwd,omitempty"`
}

// ThreadObject 是官方 thread 对象里本模块用到的字段。
type ThreadObject struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Preview   string          `json:"preview"`
	Cwd       string          `json:"cwd"`
	Ephemeral bool            `json:"ephemeral"`
	CreatedAt int64           `json:"createdAt"`
	UpdatedAt int64           `json:"updatedAt"`
	Turns     []TurnObject    `json:"turns"`
	Status    json.RawMessage `json:"status"`
}

// TurnObject 是官方 turn 对象里本模块用到的字段。
type TurnObject struct {
	ID     string            `json:"id"`
	Status string            `json:"status"`
	Error  json.RawMessage   `json:"error"`
	Items  []json.RawMessage `json:"items"`
}

// ThreadStartResult 是 thread/start、resume、fork 的响应。
type ThreadStartResult struct {
	Thread          ThreadObject    `json:"thread"`
	Model           string          `json:"model"`
	Cwd             string          `json:"cwd"`
	ApprovalPolicy  json.RawMessage `json:"approvalPolicy"`
	Sandbox         json.RawMessage `json:"sandbox"`
	ReasoningEffort json.RawMessage `json:"reasoningEffort"`
}

// ThreadListResult 是 thread/list 的响应。
type ThreadListResult struct {
	Data       []ThreadObject `json:"data"`
	NextCursor string         `json:"nextCursor"`
}

// ThreadReadResult 是 thread/read 的响应。
type ThreadReadResult struct {
	Thread ThreadObject `json:"thread"`
}

// TurnStartResult 是 turn/start 的响应。
type TurnStartResult struct {
	Turn TurnObject `json:"turn"`
}

// ModelListResult 是 model/list 的响应。
type ModelListResult struct {
	Data       []json.RawMessage `json:"data"`
	NextCursor string            `json:"nextCursor"`
}

// AccountResult 是 account/read 的响应。
type AccountResult struct {
	RequiresOpenaiAuth bool            `json:"requiresOpenaiAuth"`
	Account            json.RawMessage `json:"account"`
}

// ConfigReadResult 是 config/read 的响应。
type ConfigReadResult struct {
	Config map[string]json.RawMessage `json:"config"`
}

// PermissionProfileListResult 是 permissionProfile/list 的响应。
type PermissionProfileListResult struct {
	Data       []json.RawMessage `json:"data"`
	NextCursor string            `json:"nextCursor"`
}

// SandboxPolicy 把官方 sandbox 字符串编成 turn/start 要的对象。
func SandboxPolicy(mode string) json.RawMessage {
	switch mode {
	case "read-only":
		return json.RawMessage(`{"type":"readOnly"}`)
	case "workspace-write":
		return json.RawMessage(`{"type":"workspaceWrite"}`)
	case "danger-full-access":
		return json.RawMessage(`{"type":"dangerFullAccess"}`)
	default:
		return nil
	}
}

// CollaborationModeParams 编 Plan 模式。settings.model 官方必填。
func CollaborationModeParams(mode, model string) json.RawMessage {
	if mode == "" {
		return nil
	}
	if model == "" {
		model = "default"
	}
	body, err := json.Marshal(map[string]any{
		"mode":     mode,
		"settings": map[string]any{"model": model},
	})
	if err != nil {
		return nil
	}
	return body
}

// Initialize 发送握手请求。
func (c *Client) Initialize(ctx context.Context, info ClientInfo) (InitializeResult, error) {
	var out InitializeResult
	err := c.Call(ctx, MethodInitialize, InitializeParams{ClientInfo: info}, &out)
	return out, err
}

// ThreadStart 让 Codex 新建一条 thread。
func (c *Client) ThreadStart(ctx context.Context, params ThreadStartParams) (ThreadStartResult, error) {
	var out ThreadStartResult
	err := c.Call(ctx, MethodThreadStart, params, &out)
	return out, err
}

// ThreadResume 接上一条已有的 Codex thread。
func (c *Client) ThreadResume(ctx context.Context, params ThreadResumeParams) (ThreadStartResult, error) {
	var out ThreadStartResult
	err := c.Call(ctx, MethodThreadResume, params, &out)
	return out, err
}

// ThreadFork 按官方历史分出一条新 thread。
func (c *Client) ThreadFork(ctx context.Context, params ThreadForkParams) (ThreadStartResult, error) {
	var out ThreadStartResult
	err := c.Call(ctx, MethodThreadFork, params, &out)
	return out, err
}

// ThreadArchive 归档一条 thread。
func (c *Client) ThreadArchive(ctx context.Context, threadID string) error {
	return c.Call(ctx, MethodThreadArchive, ThreadIDParams{ThreadID: threadID}, nil)
}

// ThreadUnarchive 取消归档。
func (c *Client) ThreadUnarchive(ctx context.Context, threadID string) error {
	return c.Call(ctx, MethodThreadUnarchive, ThreadIDParams{ThreadID: threadID}, nil)
}

// ThreadSetName 改标题。
func (c *Client) ThreadSetName(ctx context.Context, threadID, name string) error {
	return c.Call(ctx, MethodThreadNameSet, ThreadNameParams{ThreadID: threadID, Name: name}, nil)
}

// ThreadList 列出 thread。
func (c *Client) ThreadList(ctx context.Context, params ThreadListParams) (ThreadListResult, error) {
	var out ThreadListResult
	err := c.Call(ctx, MethodThreadList, params, &out)
	return out, err
}

// ThreadRead 读一条 thread 及其可选历史。
func (c *Client) ThreadRead(ctx context.Context, params ThreadReadParams) (ThreadReadResult, error) {
	var out ThreadReadResult
	err := c.Call(ctx, MethodThreadRead, params, &out)
	return out, err
}

// ThreadCompact 让 Codex 自己压缩这条 thread。
func (c *Client) ThreadCompact(ctx context.Context, threadID string) error {
	return c.Call(ctx, MethodThreadCompact, ThreadIDParams{ThreadID: threadID}, nil)
}

// TurnStart 开一轮。
func (c *Client) TurnStart(ctx context.Context, params TurnStartParams) (TurnStartResult, error) {
	var out TurnStartResult
	err := c.Call(ctx, MethodTurnStart, params, &out)
	return out, err
}

// TurnInterrupt 打断一轮。
func (c *Client) TurnInterrupt(ctx context.Context, threadID, turnID string) error {
	return c.Call(ctx, MethodTurnInterrupt, TurnInterruptParams{ThreadID: threadID, TurnID: turnID}, nil)
}

// ReviewStart 让 Codex 评审当前工作目录改动。
func (c *Client) ReviewStart(ctx context.Context, threadID string) error {
	return c.Call(ctx, MethodReviewStart, ReviewStartParams{
		ThreadID: threadID,
		Target:   map[string]any{"type": "uncommittedChanges"},
	}, nil)
}

// ModelList 列出模型。
func (c *Client) ModelList(ctx context.Context, params ModelListParams) (ModelListResult, error) {
	var out ModelListResult
	err := c.Call(ctx, MethodModelList, params, &out)
	return out, err
}

// PermissionProfileList 列出权限预设。
func (c *Client) PermissionProfileList(ctx context.Context, params CursorListParams) (PermissionProfileListResult, error) {
	var out PermissionProfileListResult
	err := c.Call(ctx, MethodPermissionProfileList, params, &out)
	return out, err
}

// AccountRead 读本机授权。
func (c *Client) AccountRead(ctx context.Context) (AccountResult, error) {
	var out AccountResult
	err := c.Call(ctx, MethodAccountRead, map[string]any{}, &out)
	return out, err
}

// ConfigRead 读 Codex 当前生效配置。
func (c *Client) ConfigRead(ctx context.Context, cwd string) (ConfigReadResult, error) {
	var out ConfigReadResult
	err := c.Call(ctx, MethodConfigRead, ConfigReadParams{Cwd: cwd}, &out)
	return out, err
}

// MapThread 把官方 thread 映到本模块 Session。
func MapThread(th ThreadObject, archived bool, activeTurnID string) Session {
	title := th.Name
	return Session{
		ID:           th.ID,
		ThreadID:     th.ID,
		Title:        title,
		Preview:      th.Preview,
		Cwd:          th.Cwd,
		ActiveTurnID: activeTurnID,
		Archived:     archived,
		Ephemeral:    th.Ephemeral,
		CreatedAt:    th.CreatedAt,
		UpdatedAt:    th.UpdatedAt,
	}
}

// ParseModel 从 model/list 的一条里取出 ModelInfo。
func ParseModel(raw json.RawMessage) (ModelInfo, error) {
	var row struct {
		ID                        string `json:"id"`
		Model                     string `json:"model"`
		DisplayName               string `json:"displayName"`
		DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
		SupportedReasoningEfforts []any  `json:"supportedReasoningEfforts"`
		Hidden                    bool   `json:"hidden"`
		IsDefault                 bool   `json:"isDefault"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return ModelInfo{}, err
	}
	id := row.ID
	if id == "" {
		id = row.Model
	}
	efforts := make([]string, 0, len(row.SupportedReasoningEfforts))
	for _, item := range row.SupportedReasoningEfforts {
		if s := effortID(item); s != "" {
			efforts = append(efforts, s)
		}
	}
	return ModelInfo{
		ID:            id,
		DisplayName:   row.DisplayName,
		Efforts:       efforts,
		DefaultEffort: row.DefaultReasoningEffort,
		Hidden:        row.Hidden,
		IsDefault:     row.IsDefault,
	}, nil
}

func effortID(item any) string {
	switch v := item.(type) {
	case string:
		return v
	case map[string]any:
		for _, key := range []string{"reasoningEffort", "effort", "id"} {
			if s, _ := v[key].(string); s != "" {
				return s
			}
		}
	}
	return ""
}

// ParsePermissionProfile 从 permissionProfile/list 的一条里取出 ModeInfo。
func ParsePermissionProfile(raw json.RawMessage) (ModeInfo, error) {
	var row struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Allowed     bool   `json:"allowed"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return ModeInfo{}, err
	}
	return ModeInfo{
		ID:      row.ID,
		Label:   row.Description,
		Kind:    "permission",
		Allowed: row.Allowed,
	}, nil
}

// CollaborationModes 返回官方 Plan 档。0.149.0 没有 collaborationMode/list。
func CollaborationModes() []ModeInfo {
	return []ModeInfo{
		{ID: "default", Label: "Default", Kind: "collaboration", Allowed: true},
		{ID: "plan", Label: "Plan", Kind: "collaboration", Allowed: true},
	}
}

// ConfigString 从 config/read 里取字符串配置。
func ConfigString(cfg ConfigReadResult, key string) string {
	raw, ok := cfg.Config[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// Authorized 根据 account/read 判断能不能开回合。
func Authorized(account AccountResult) bool {
	if !account.RequiresOpenaiAuth {
		return true
	}
	if len(account.Account) == 0 || string(account.Account) == "null" {
		return false
	}
	return true
}
