package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

type CreateMessageRequest struct {
	Content   string                      `json:"content"`
	InputMode InputMode                   `json:"input_mode"`
	Mode      pkgagent.AgentMode          `json:"mode"`
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

type UpdateMessageRequest struct {
	Content string `json:"content"`
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

// UpdateMessage 改排队中用户消息的正文。Run 一旦离开 queued 则拒绝。
func (a *API) UpdateMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	messageID := chi.URLParam(r, "message_id")
	var req UpdateMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, cderr.Invalid("content is required"))
		return
	}
	msg, err := a.q(r.Context()).GetMessage(r.Context(), messageID)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	if msg.SessionID != sessionID {
		writeError(w, cderr.NotFound("message not found"))
		return
	}
	if msg.Role != string(pkgagent.RoleUser) {
		writeError(w, cderr.Invalid("only user messages can be edited"))
		return
	}
	if !msg.RunID.Valid || msg.RunID.String == "" {
		writeError(w, cderr.Invalid("message has no run"))
		return
	}
	run, err := a.q(r.Context()).GetRun(r.Context(), msg.RunID.String)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	if run.TriggerMessageID != msg.ID {
		writeError(w, cderr.Invalid("only the trigger message can be edited"))
		return
	}
	if pkgagent.RunStatus(run.Status) != pkgagent.RunQueued {
		writeError(w, cderr.Conflict("only queued messages can be edited"))
		return
	}
	row, err := a.q(r.Context()).UpdateMessageContent(r.Context(), sqlite.UpdateMessageContentParams{
		Content: string(pkgagent.EncodeText(content)),
		ID:      msg.ID,
	})
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	updated := mapMessage(row)
	if a.runtime != nil {
		a.runtime.ReindexMessage(r.Context(), sessionID, updated)
	}
	writeJSON(w, http.StatusOK, MessageResponse{Message: updated})
}

// DeleteMessage 删除消息。
func (a *API) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	if err := a.q(r.Context()).DeleteMessage(r.Context(), chi.URLParam(r, "message_id")); err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, DeleteMessageResponse{OK: true})
}
