package handler

import (
	"net/http"

	"codedock/internal/agent/memory"
)

type ListTextMemoriesRequest struct {
	UserID    string
	ProjectID string
}

type ListTextMemoriesResponse struct {
	Items []memory.TextMemory
}

type GetTextMemoryRequest struct {
	Scope   memory.TextMemoryScope
	ScopeID string
}

type GetTextMemoryResponse struct {
	Memory  memory.TextMemory
	ByteLen int
}

type DeleteTextMemoryRequest struct {
	Scope   memory.TextMemoryScope
	ScopeID string
}

type DeleteTextMemoryResponse struct {
	OK bool
}

// ListTextMemories 列出用户可见的 Markdown 记忆。本阶段为空实现。
func (a *API) ListTextMemories(w http.ResponseWriter, _ *http.Request) {
	_ = ListTextMemoriesRequest{}
	if a.queries != nil {
		_ = a.queries.ListTextMemories
	}
	writeJSON(w, http.StatusOK, ListTextMemoriesResponse{})
}

// GetTextMemory 查看 Markdown 全文。本阶段为空实现。
func (a *API) GetTextMemory(w http.ResponseWriter, _ *http.Request) {
	_ = GetTextMemoryRequest{}
	if a.queries != nil {
		_ = a.queries.GetTextMemory
	}
	writeJSON(w, http.StatusOK, GetTextMemoryResponse{})
}

// DeleteTextMemory 用户清空某一 scope 的记忆。本阶段为空实现。
func (a *API) DeleteTextMemory(w http.ResponseWriter, _ *http.Request) {
	_ = DeleteTextMemoryRequest{}
	if a.queries != nil {
		_ = a.queries.DeleteTextMemory
	}
	writeJSON(w, http.StatusOK, DeleteTextMemoryResponse{OK: true})
}
