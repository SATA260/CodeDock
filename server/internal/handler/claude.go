package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"codedock/pkg/claude"
)

// Claude HTTP 契约。路径参数与 JSON 字段定死后不要改。
// 路径参数：session_id、turn_id、approval_id、request_id。

type ClaudeCreateSessionRequest struct {
	UserID string `json:"user_id"`
}

type ClaudeRenameSessionRequest struct {
	SessionID string `json:"-"`
	Title     string `json:"title"`
}

type ClaudeInvokeRequest struct {
	SessionID string `json:"-"`
	Name      string `json:"name"`
	Args      string `json:"args"`
}

type ClaudePathRequest struct {
	SessionID string `json:"-"`
	Path      string `json:"path"`
}

type ClaudeInput struct {
	Text     string   `json:"text"`
	Mentions []string `json:"mentions"`
	Images   []string `json:"images"`
}

type ClaudeStartTurnRequest struct {
	SessionID string      `json:"-"`
	Content   string      `json:"content"`
	Input     ClaudeInput `json:"input"`
	Mode      string      `json:"mode"` // start | queue
}

type ClaudeApplySettingsRequest struct {
	SessionID      string   `json:"-"`
	Model          string   `json:"model"`
	Effort         string   `json:"effort"`
	PermissionMode string   `json:"permission_mode"`
	Cwd            string   `json:"cwd"`
	Overridden     []string `json:"overridden"`
}

type ClaudeDecideRequest struct {
	ApprovalID string   `json:"-"`
	Approved   bool     `json:"approved"`
	Scope      string   `json:"scope"` // once | session
	Choice     string   `json:"choice"`
	Values     []string `json:"values"`
}

type ClaudeRejectUnknownRequest struct {
	RequestID string `json:"-"`
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id"`
}

type ClaudeStatusResponse struct {
	Available  bool   `json:"available"`
	Authorized bool   `json:"authorized"`
	Version    string `json:"version"`
	Hint       string `json:"hint"`
}

type ClaudeModel struct {
	ID            string   `json:"id"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort"`
	Hidden        bool     `json:"hidden"`
	IsDefault     bool     `json:"is_default"`
}

type ClaudeListModelsResponse struct {
	Models []ClaudeModel `json:"models"`
}

type ClaudeMode struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type ClaudeListModesResponse struct {
	Modes []ClaudeMode `json:"modes"`
}

type ClaudeCommand struct {
	Name   string `json:"name"`
	Action string `json:"action"`
	Hint   string `json:"hint"`
}

type ClaudeListCommandsResponse struct {
	Commands []ClaudeCommand `json:"commands"`
}

type ClaudeSession struct {
	ID              string `json:"id"`
	ClaudeSessionID string `json:"claude_session_id"`
	Title           string `json:"title"`
	ActiveTurnID    string `json:"active_turn_id"`
	Archived        bool   `json:"archived"`
	CreatedAt       int64  `json:"created_at"` // 本机实录第一条时间，Unix 秒。
	UpdatedAt       int64  `json:"updated_at"` // 本机实录最近一条时间，Unix 秒。
}

type ClaudeSessionResponse struct {
	Session ClaudeSession `json:"session"`
}

type ClaudeListSessionsResponse struct {
	Sessions []ClaudeSession `json:"sessions"`
}

type ClaudeSettingsResponse struct {
	Model          string   `json:"model"`
	Effort         string   `json:"effort"`
	PermissionMode string   `json:"permission_mode"`
	Cwd            string   `json:"cwd"`
	Overridden     []string `json:"overridden"`
}

type ClaudeInvokeResponse struct {
	Hint string `json:"hint"`
}

type ClaudeStartTurnResponse struct {
	TurnID string `json:"turn_id"`
}

type ClaudeProgress struct {
	Kind    string   `json:"kind"`
	Text    string   `json:"text"`
	Command string   `json:"command"`
	Paths   []string `json:"paths"`
	Diff    string   `json:"diff"`
}

type ClaudeTranscriptResponse struct {
	Items []ClaudeProgress  `json:"items"`
	Usage *ClaudeTokenUsage `json:"usage,omitempty"`
}

type ClaudeTokenUsage struct {
	Used   int64 `json:"used"`
	Window int64 `json:"window"`
}

type ClaudeOKResponse struct {
	OK bool `json:"ok"`
}

func claudeOK() ClaudeOKResponse {
	return ClaudeOKResponse{OK: true}
}

func claudeSessionID(r *http.Request) string {
	return chi.URLParam(r, "session_id")
}

func claudeTurnID(r *http.Request) string {
	return chi.URLParam(r, "turn_id")
}

func claudeApprovalID(r *http.Request) string {
	return chi.URLParam(r, "approval_id")
}

func claudeRequestID(r *http.Request) string {
	return chi.URLParam(r, "request_id")
}

// ClaudeProbe 查看本机 Claude Code 是否可用、是否已取得授权。
func (a *API) ClaudeProbe(w http.ResponseWriter, r *http.Request) {
	status, err := claude.Probe()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeStatusResponse{
		Available:  status.Available,
		Authorized: status.Authorized,
		Version:    status.Version,
		Hint:       status.Hint,
	})
}

// ClaudeListModels 列出 Claude Code 模型及各自支持的推理强度。
func (a *API) ClaudeListModels(w http.ResponseWriter, r *http.Request) {
	models, err := claude.ListModels()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeListModelsResponse{Models: mapClaudeModels(models)})
}

// ClaudeListModes 列出 Claude Code 的官方权限档。
func (a *API) ClaudeListModes(w http.ResponseWriter, r *http.Request) {
	modes, err := claude.ListModes()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeListModesResponse{Modes: mapClaudeModes(modes)})
}

// ClaudeListCommands 列出与 Claude 斜杠、官方扩展按钮共用的命令。
func (a *API) ClaudeListCommands(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ClaudeListCommandsResponse{Commands: mapClaudeCommands(claude.ListCommands())})
}

// ClaudeCreateSession 创建一条只走 Claude Code 的对话。
func (a *API) ClaudeCreateSession(w http.ResponseWriter, r *http.Request) {
	var req ClaudeCreateSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	sess, err := claude.Create(req.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeSessionResponse{Session: mapClaudeSession(sess)})
}

// ClaudeListSessions 从本机 Claude 列未归档对话。
func (a *API) ClaudeListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := claude.ListSessions()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeListSessionsResponse{Sessions: mapClaudeSessions(sessions)})
}

// ClaudeGetSession 读取对话。
func (a *API) ClaudeGetSession(w http.ResponseWriter, r *http.Request) {
	sess, err := claude.Get(claudeSessionID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeSessionResponse{Session: mapClaudeSession(sess)})
}

// ClaudeRenameSession 改对话标题。
func (a *API) ClaudeRenameSession(w http.ResponseWriter, r *http.Request) {
	var req ClaudeRenameSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.SessionID = claudeSessionID(r)
	if err := claude.Rename(req.SessionID, req.Title); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

// ClaudeArchiveSession 按官方 tagSession 归档，之后不能再向 Claude Code 开回合。
func (a *API) ClaudeArchiveSession(w http.ResponseWriter, r *http.Request) {
	if err := claude.Archive(claudeSessionID(r)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

// ClaudeForkSession 按官方 --fork-session 复制已落盘实录，换新 Claude session。
func (a *API) ClaudeForkSession(w http.ResponseWriter, r *http.Request) {
	sess, err := claude.Fork(claudeSessionID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeSessionResponse{Session: mapClaudeSession(sess)})
}

// ClaudeEffective 返回 Claude 默认与用户覆盖合并后的生效配置。
func (a *API) ClaudeEffective(w http.ResponseWriter, r *http.Request) {
	settings, err := claude.Effective(claudeSessionID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapClaudeSettings(settings))
}

// ClaudeApplySettings 只记下用户改过的 Claude 项。
func (a *API) ClaudeApplySettings(w http.ResponseWriter, r *http.Request) {
	var req ClaudeApplySettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.SessionID = claudeSessionID(r)
	settings, err := claude.Apply(req.SessionID, claude.Settings{
		Model:          req.Model,
		Effort:         req.Effort,
		PermissionMode: req.PermissionMode,
		Cwd:            req.Cwd,
		Overridden:     claudeStrings(req.Overridden),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapClaudeSettings(settings))
}

// ClaudeInvoke 执行同名动作，或返回去终端改 Claude 配置的提示。
func (a *API) ClaudeInvoke(w http.ResponseWriter, r *http.Request) {
	var req ClaudeInvokeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.SessionID = claudeSessionID(r)
	result, err := claude.Invoke(req.SessionID, req.Name, req.Args)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeInvokeResponse{Hint: result.Hint})
}

// ClaudeMention 把仓库内文件挂到待发给 Claude Code 的内容上。
func (a *API) ClaudeMention(w http.ResponseWriter, r *http.Request) {
	var req ClaudePathRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.SessionID = claudeSessionID(r)
	if err := claude.Mention(req.SessionID, req.Path); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

// ClaudeAttachImage 把本地图片挂到待发给 Claude Code 的内容上。
func (a *API) ClaudeAttachImage(w http.ResponseWriter, r *http.Request) {
	var req ClaudePathRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.SessionID = claudeSessionID(r)
	if err := claude.AttachImage(req.SessionID, req.Path); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

// ClaudeStartTurn 空闲则向 Claude Code 开新一轮；进行中则排队，不打断。
func (a *API) ClaudeStartTurn(w http.ResponseWriter, r *http.Request) {
	var req ClaudeStartTurnRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.SessionID = claudeSessionID(r)
	mode := claude.InputModeStart
	if req.Mode == string(claude.InputModeQueue) {
		mode = claude.InputModeQueue
	}
	turnID, err := claude.Start(req.SessionID, req.Content, claude.Input{
		Text:     req.Input.Text,
		Mentions: claudeStrings(req.Input.Mentions),
		Images:   claudeStrings(req.Input.Images),
	}, mode)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeStartTurnResponse{TurnID: turnID})
}

// ClaudeHydrate 按本机 Claude 已落下的记录回放。
func (a *API) ClaudeHydrate(w http.ResponseWriter, r *http.Request) {
	items, usage, err := claude.HydrateDetail(claudeSessionID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ClaudeTranscriptResponse{
		Items: mapClaudeProgress(items),
		Usage: mapClaudeUsage(usage),
	})
}

// ClaudeCancelTurn 用户手动打断当前一轮，并向 Claude Code 传播取消。
func (a *API) ClaudeCancelTurn(w http.ResponseWriter, r *http.Request) {
	if err := claude.Cancel(claudeTurnID(r)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

// ClaudeContinueTurn 反问有了结果后让 Claude Code 继续。
func (a *API) ClaudeContinueTurn(w http.ResponseWriter, r *http.Request) {
	if err := claude.Continue(claudeTurnID(r)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

// ClaudeDecide 按人对已知反问的作答记下结果。
func (a *API) ClaudeDecide(w http.ResponseWriter, r *http.Request) {
	var req ClaudeDecideRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.ApprovalID = claudeApprovalID(r)
	if err := claude.Decide(req.ApprovalID, claude.AskAnswer{
		Approved: req.Approved,
		Scope:    claude.DecisionScope(req.Scope),
		Choice:   req.Choice,
		Values:   claudeStrings(req.Values),
	}); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

// ClaudeRejectUnknown 官方新加、认不出的提问：提示不兼容并回包。
func (a *API) ClaudeRejectUnknown(w http.ResponseWriter, r *http.Request) {
	var req ClaudeRejectUnknownRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.RequestID = claudeRequestID(r)
	if err := claude.AppendProgress(req.SessionID, req.TurnID, claude.Progress{Kind: claude.ProgressKindNotice}); err != nil {
		writeError(w, err)
		return
	}
	if err := claude.RejectUnknown(req.RequestID); err != nil {
		writeError(w, err)
		return
	}
	if err := claude.Continue(req.TurnID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claudeOK())
}

func claudeStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func mapClaudeSession(sess claude.Session) ClaudeSession {
	return ClaudeSession{
		ID:              sess.ID,
		ClaudeSessionID: sess.ClaudeSessionID,
		Title:           sess.Title,
		ActiveTurnID:    sess.ActiveTurnID,
		Archived:        sess.Archived,
		CreatedAt:       sess.CreatedAt,
		UpdatedAt:       sess.UpdatedAt,
	}
}

func mapClaudeSessions(items []claude.Session) []ClaudeSession {
	out := make([]ClaudeSession, 0, len(items))
	for _, item := range items {
		out = append(out, mapClaudeSession(item))
	}
	return out
}

func mapClaudeModels(items []claude.ModelInfo) []ClaudeModel {
	out := make([]ClaudeModel, 0, len(items))
	for _, item := range items {
		out = append(out, ClaudeModel{
			ID:            item.ID,
			Efforts:       claudeStrings(item.Efforts),
			DefaultEffort: item.DefaultEffort,
			Hidden:        item.Hidden,
			IsDefault:     item.IsDefault,
		})
	}
	return out
}

func mapClaudeModes(items []claude.ModeInfo) []ClaudeMode {
	out := make([]ClaudeMode, 0, len(items))
	for _, item := range items {
		out = append(out, ClaudeMode{ID: item.ID, Kind: item.Kind})
	}
	return out
}

func mapClaudeCommands(items []claude.CommandSpec) []ClaudeCommand {
	out := make([]ClaudeCommand, 0, len(items))
	for _, item := range items {
		out = append(out, ClaudeCommand{Name: item.Name, Action: string(item.Action), Hint: item.Hint})
	}
	return out
}

func mapClaudeSettings(settings claude.Settings) ClaudeSettingsResponse {
	return ClaudeSettingsResponse{
		Model:          settings.Model,
		Effort:         settings.Effort,
		PermissionMode: settings.PermissionMode,
		Cwd:            settings.Cwd,
		Overridden:     claudeStrings(settings.Overridden),
	}
}

func mapClaudeProgress(items []claude.Progress) []ClaudeProgress {
	out := make([]ClaudeProgress, 0, len(items))
	for _, item := range items {
		out = append(out, ClaudeProgress{
			Kind:    string(item.Kind),
			Text:    item.Text,
			Command: item.Command,
			Paths:   claudeStrings(item.Paths),
			Diff:    item.Diff,
		})
	}
	return out
}

// mapClaudeUsage 只在官方实录里有用量时带上 used / window。
func mapClaudeUsage(usage claude.TokenUsage) *ClaudeTokenUsage {
	if usage.Used <= 0 || usage.Window <= 0 {
		return nil
	}
	return &ClaudeTokenUsage{Used: usage.Used, Window: usage.Window}
}
