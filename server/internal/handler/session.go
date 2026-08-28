package handler

import (
	"net/http"

	pkgagent "codedock/pkg/agent"
)

type CreateSessionRequest struct {
	TenantID string
	UserID   string
	AgentID  string
}

type UpdateSessionRequest struct {
	AgentID string
	Status  pkgagent.SessionStatus
}

type SessionResponse struct {
	Session pkgagent.Session
}

type ListSessionsResponse struct {
	Sessions []pkgagent.Session
}

// CreateSession 创建会话。
// TODO: 解析请求并写入 sessions。
func (a *API) CreateSession(w http.ResponseWriter, _ *http.Request) {
	_ = CreateSessionRequest{}
	if a.queries != nil {
		_ = a.queries.InsertSession
	}
	writeJSON(w, http.StatusOK, SessionResponse{})
}

// ListSessions 列出会话。
// TODO: 按查询条件列出 sessions。
func (a *API) ListSessions(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListSessions
	}
	writeJSON(w, http.StatusOK, ListSessionsResponse{})
}

// GetSession 查询单个会话。
// TODO: 按 session_id 读取并返回会话。
func (a *API) GetSession(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.GetSession
	}
	writeJSON(w, http.StatusOK, SessionResponse{})
}

// UpdateSession 更新会话。
// TODO: 按 session_id 更新会话字段。
func (a *API) UpdateSession(w http.ResponseWriter, _ *http.Request) {
	_ = UpdateSessionRequest{}
	if a.queries != nil {
		_ = a.queries.UpdateSession
	}
	writeJSON(w, http.StatusOK, SessionResponse{})
}

// ArchiveSession 归档会话。
// TODO: 将指定会话标记为 archived。
func (a *API) ArchiveSession(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ArchiveSession
	}
	writeJSON(w, http.StatusOK, SessionResponse{})
}
