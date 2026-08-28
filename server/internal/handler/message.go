package handler

import (
	"net/http"

	pkgagent "codedock/pkg/agent"
)

type CreateMessageRequest struct {
	Content   string
	InputMode InputMode
}

type MessageResponse struct {
	Message pkgagent.Message
}

type ListMessagesResponse struct {
	Messages     []pkgagent.Message
	AsOfEventSeq int64
}

type DeleteMessageResponse struct {
	OK bool
}

// CreateMessage 写入用户消息；若触发执行则在 Handler 内启动 Run。
// TODO: 写入用户消息，并按 InputMode 决定是否启动 Run。
func (a *API) CreateMessage(w http.ResponseWriter, r *http.Request) {
	_ = CreateMessageRequest{}
	if a.queries != nil {
		_ = a.queries.InsertMessage
	}
	_, _ = a.start(r.Context(), "", StartRunRequest{InputMode: InputQueue})
	writeJSON(w, http.StatusOK, MessageResponse{})
}

// ListMessages 分页查询消息。
// TODO: 按 session_id 分页查询消息。
func (a *API) ListMessages(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListSessionMessages
	}
	writeJSON(w, http.StatusOK, ListMessagesResponse{})
}

// DeleteMessage 删除消息。
// TODO: 按 message_id 删除消息。
func (a *API) DeleteMessage(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.DeleteMessage
	}
	writeJSON(w, http.StatusOK, DeleteMessageResponse{OK: true})
}
