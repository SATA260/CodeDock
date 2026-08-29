package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"codedock/internal/agent/memory"
	cderr "codedock/internal/errors"
)

type ListTextMemoriesRequest struct {
	UserID      string
	WorkspaceID string
}

type ListTextMemoriesResponse struct {
	Items []memory.TextMemory `json:"items"`
}

type GetTextMemoryRequest struct {
	Scope   memory.TextMemoryScope
	ScopeID string
	Name    string
}

type GetTextMemoryResponse struct {
	Memory  memory.TextMemory `json:"memory"`
	ByteLen int               `json:"byte_len"`
}

type DeleteTextMemoryRequest struct {
	Scope   memory.TextMemoryScope
	ScopeID string
	Name    string
	All     bool
}

type DeleteTextMemoryResponse struct {
	OK bool `json:"ok"`
}

// ListTextMemories 列出用户可见的 Markdown 记忆。
func (a *API) ListTextMemories(w http.ResponseWriter, r *http.Request) {
	req := ListTextMemoriesRequest{
		UserID:      r.URL.Query().Get("user_id"),
		WorkspaceID: r.URL.Query().Get("workspace_id"),
	}
	if req.UserID == "" && req.WorkspaceID == "" {
		writeError(w, cderr.Invalid("user_id or workspace_id is required"))
		return
	}
	var items []memory.TextMemory
	if req.UserID != "" {
		listed, err := memory.List(r.Context(), a.q(r.Context()), memory.ScopeUser, req.UserID)
		if err != nil {
			writeError(w, err)
			return
		}
		items = append(items, listed...)
	}
	if req.WorkspaceID != "" {
		listed, err := memory.List(r.Context(), a.q(r.Context()), memory.ScopeWorkspace, req.WorkspaceID)
		if err != nil {
			writeError(w, err)
			return
		}
		items = append(items, listed...)
	}
	if items == nil {
		items = []memory.TextMemory{}
	}
	writeJSON(w, http.StatusOK, ListTextMemoriesResponse{Items: items})
}

// GetTextMemory 查看一篇目录或专题。
func (a *API) GetTextMemory(w http.ResponseWriter, r *http.Request) {
	req := GetTextMemoryRequest{
		Scope:   memory.TextMemoryScope(chi.URLParam(r, "scope")),
		ScopeID: chi.URLParam(r, "scope_id"),
		Name:    r.URL.Query().Get("name"),
	}
	if req.Name == "" {
		req.Name = memory.NameIndex
	}
	item, err := memory.Get(r.Context(), a.q(r.Context()), memory.TextMemoryKey{
		Scope:   req.Scope,
		ScopeID: req.ScopeID,
		Kind:    memory.KindFromName(req.Name),
		Name:    req.Name,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, GetTextMemoryResponse{Memory: item, ByteLen: item.ByteLen})
}

// DeleteTextMemory 用户删除一篇目录或专题，或按 scope 清空。
func (a *API) DeleteTextMemory(w http.ResponseWriter, r *http.Request) {
	req := DeleteTextMemoryRequest{
		Scope:   memory.TextMemoryScope(chi.URLParam(r, "scope")),
		ScopeID: chi.URLParam(r, "scope_id"),
		Name:    r.URL.Query().Get("name"),
		All:     r.URL.Query().Get("all") == "1",
	}
	if req.All {
		if err := memory.DeleteByScope(r.Context(), a.q(r.Context()), req.Scope, req.ScopeID); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, DeleteTextMemoryResponse{OK: true})
		return
	}
	if req.Name == "" {
		req.Name = memory.NameIndex
	}
	if err := memory.Delete(r.Context(), a.q(r.Context()), memory.TextMemoryKey{
		Scope:   req.Scope,
		ScopeID: req.ScopeID,
		Kind:    memory.KindFromName(req.Name),
		Name:    req.Name,
	}); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, DeleteTextMemoryResponse{OK: true})
}
