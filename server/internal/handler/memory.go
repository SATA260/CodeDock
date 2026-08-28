package handler

import (
	"net/http"

	"codedock/internal/agent/memory"
)

type ListTextMemoriesRequest struct {
	UserID      string
	WorkspaceID string
}

type ListTextMemoriesResponse struct {
	Items []memory.TextMemory
}

type GetTextMemoryRequest struct {
	Scope   memory.TextMemoryScope
	ScopeID string
	Name    string
}

type GetTextMemoryResponse struct {
	Memory  memory.TextMemory
	ByteLen int
}

type DeleteTextMemoryRequest struct {
	Scope   memory.TextMemoryScope
	ScopeID string
	Name    string
}

type DeleteTextMemoryResponse struct {
	OK bool
}

// ListTextMemories 列出用户可见的 Markdown 记忆。
// TODO: 解析 user_id / workspace_id，按 user / workspace scope 列出目录与专题。
func (a *API) ListTextMemories(w http.ResponseWriter, _ *http.Request) {
	_ = ListTextMemoriesRequest{}
	if a.queries != nil {
		_ = a.queries.ListTextMemories
	}
	writeJSON(w, http.StatusOK, ListTextMemoriesResponse{})
}

// GetTextMemory 查看一篇目录或专题。
// TODO: 按 path 中的 scope / scope_id 与 query name（默认目录）读取全文并返回 ByteLen。
func (a *API) GetTextMemory(w http.ResponseWriter, _ *http.Request) {
	_ = GetTextMemoryRequest{Name: memory.NameIndex}
	if a.queries != nil {
		_ = a.queries.GetTextMemory
	}
	writeJSON(w, http.StatusOK, GetTextMemoryResponse{})
}

// DeleteTextMemory 用户删除一篇目录或专题。
// TODO: 按 path 中的 scope / scope_id 与 query name（默认目录）删除。
func (a *API) DeleteTextMemory(w http.ResponseWriter, _ *http.Request) {
	_ = DeleteTextMemoryRequest{Name: memory.NameIndex}
	if a.queries != nil {
		_ = a.queries.DeleteTextMemory
	}
	writeJSON(w, http.StatusOK, DeleteTextMemoryResponse{OK: true})
}
