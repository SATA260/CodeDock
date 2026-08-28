package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

type CreateMessageRequest struct {
	Content   string                     `json:"content"`
	InputMode InputMode                  `json:"input_mode"`
	Mode      pkgagent.AgentMode         `json:"mode"`
	Config    *pkgagent.RunConfigSnapshot `json:"config,omitempty"`
}

type MessageResponse struct {
	Message pkgagent.Message `json:"message"`
}

type ListMessagesResponse struct {
	Messages     []pkgagent.Message `json:"messages"`
	AsOfEventSeq int64              `json:"as_of_event_seq"`
	PageInfo
}

type DeleteMessageResponse struct {
	OK bool `json:"ok"`
}

// CreateMessage 写入用户消息；若触发执行则在 Handler 内启动 Run。
func (a *API) CreateMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	var req CreateMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	resp, err := a.start(r.Context(), sessionID, StartRunRequest{
		Content:   req.Content,
		InputMode: req.InputMode,
		Mode:      req.Mode,
		Config:    req.Config,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	row, err := a.q(r.Context()).GetRun(r.Context(), resp.RunID)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	msg, err := a.q(r.Context()).GetMessage(r.Context(), mapRun(row).TriggerMessageID)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, MessageResponse{Message: mapMessage(msg)})
}

// ListMessages 分页查询消息。
func (a *API) ListMessages(w http.ResponseWriter, r *http.Request) {
	session, err := a.loadSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	page, err := ParsePageQuery(r, messagePageDefaults)
	if err != nil {
		writeError(w, err)
		return
	}
	q := a.q(r.Context())
	total, err := q.CountSessionMessages(r.Context(), session.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	rows, err := q.ListSessionMessagesPage(r.Context(), sqlite.ListSessionMessagesPageParams{
		SessionID: session.ID,
		SortBy:    page.SortBy,
		SortOrder: page.SortOrder,
		Limit:     int64(page.Limit()),
		Offset:    int64(page.Offset()),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	messages := make([]pkgagent.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, mapMessage(row))
	}
	writeJSON(w, http.StatusOK, ListMessagesResponse{
		Messages:     messages,
		AsOfEventSeq: session.LastEventSeq,
		PageInfo:     page.Info(total),
	})
}

// DeleteMessage 删除消息。
func (a *API) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	if err := a.q(r.Context()).DeleteMessage(r.Context(), chi.URLParam(r, "message_id")); err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, DeleteMessageResponse{OK: true})
}
