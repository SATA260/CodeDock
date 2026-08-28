package handler

import (
	"encoding/json"
	"net/http"

	"codedock/internal/agent"
	"codedock/internal/events"
	"codedock/pkg/db/sqlite"
)

// API 承载大部分 HTTP 逻辑：CRUD、SSE、Start / Continue / Cancel，以及审批裁决。
type API struct {
	queries *sqlite.Queries
	runtime *agent.Runtime
	bus     *events.Bus
}

// New 创建 Handler 入口。本阶段不绑定具体业务实现。
func New(queries *sqlite.Queries, runtime *agent.Runtime, bus *events.Bus) *API {
	return &API{queries: queries, runtime: runtime, bus: bus}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}
