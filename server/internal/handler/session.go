package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
	"codedock/internal/util"
)

type CreateSessionRequest struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	AgentID  string `json:"agent_id"`
}

type UpdateSessionRequest struct {
	AgentID string                 `json:"agent_id"`
	Status  pkgagent.SessionStatus `json:"status"`
}

type SessionResponse struct {
	Session pkgagent.Session `json:"session"`
}

type ListSessionsResponse struct {
	Sessions []pkgagent.Session `json:"sessions"`
	PageInfo
}

// CreateSession 创建会话。
func (a *API) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.UserID == "" {
		writeError(w, cderr.Invalid("user_id is required"))
		return
	}
	if req.TenantID == "" {
		req.TenantID = "default"
	}
	if req.AgentID == "" {
		req.AgentID = "default"
	}
	now := util.FormatTime(util.Now())
	row, err := a.q(r.Context()).InsertSession(r.Context(), sqlite.InsertSessionParams{
		ID:            util.NewID(),
		TenantID:      req.TenantID,
		UserID:        req.UserID,
		AgentID:       req.AgentID,
		Status:        string(pkgagent.SessionActive),
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SessionResponse{Session: mapSession(row)})
}

// ListSessions 分页列出会话。
func (a *API) ListSessions(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePageQuery(r, sessionPageDefaults)
	if err != nil {
		writeError(w, err)
		return
	}
	q := a.q(r.Context())
	total, err := q.CountSessions(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	rows, err := q.ListSessions(r.Context(), sqlite.ListSessionsParams{
		SortBy:    page.SortBy,
		SortOrder: page.SortOrder,
		Limit:     int64(page.Limit()),
		Offset:    int64(page.Offset()),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	sessions := make([]pkgagent.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, mapSession(row))
	}
	writeJSON(w, http.StatusOK, ListSessionsResponse{Sessions: sessions, PageInfo: page.Info(total)})
}

// GetSession 查询单个会话。
func (a *API) GetSession(w http.ResponseWriter, r *http.Request) {
	session, err := a.loadSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SessionResponse{Session: session})
}

// UpdateSession 更新会话。
func (a *API) UpdateSession(w http.ResponseWriter, r *http.Request) {
	session, err := a.loadSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req UpdateSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.AgentID != "" {
		session.AgentID = req.AgentID
	}
	if req.Status != "" {
		session.Status = req.Status
	}
	row, err := a.q(r.Context()).UpdateSession(r.Context(), sqlite.UpdateSessionParams{
		AgentID:       session.AgentID,
		Status:        string(session.Status),
		ActiveRunID:   nullString(deref(session.ActiveRunID)),
		LastEventSeq:  session.LastEventSeq,
		CompactionSeq: session.CompactionSeq,
		UpdatedAt:     util.FormatTime(util.Now()),
		ID:            session.ID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SessionResponse{Session: mapSession(row)})
}

// ArchiveSession 归档会话。
func (a *API) ArchiveSession(w http.ResponseWriter, r *http.Request) {
	session, err := a.loadSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if session.ActiveRunID != nil {
		writeError(w, cderr.Conflict("cannot archive session with active run"))
		return
	}
	if err := a.q(r.Context()).ArchiveSession(r.Context(), sqlite.ArchiveSessionParams{
		UpdatedAt: util.FormatTime(util.Now()),
		ID:        session.ID,
	}); err != nil {
		writeError(w, err)
		return
	}
	session.Status = pkgagent.SessionArchived
	writeJSON(w, http.StatusOK, SessionResponse{Session: session})
}

// loadSession 从路径参数读取并映射会话。
func (a *API) loadSession(r *http.Request) (pkgagent.Session, error) {
	id := chi.URLParam(r, "session_id")
	if id == "" {
		return pkgagent.Session{}, cderr.Invalid("session_id is required")
	}
	row, err := a.q(r.Context()).GetSession(r.Context(), id)
	if err != nil {
		return pkgagent.Session{}, wrapHandlerDB(err)
	}
	return mapSession(row), nil
}
