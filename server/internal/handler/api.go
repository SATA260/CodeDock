package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"codedock/internal/agent"
	cderr "codedock/internal/errors"
	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

// API 承载大部分 HTTP 逻辑：CRUD、SSE、Start / Continue / Cancel，以及审批裁决。
type API struct {
	db       db.Client
	queries  *sqlite.Queries
	runtime  *agent.Runtime
	bus      *events.Bus
	defaults pkgagent.RunConfigSnapshot
}

// New 创建 Handler 入口。
func New(client db.Client, queries *sqlite.Queries, runtime *agent.Runtime, bus *events.Bus, defaults pkgagent.RunConfigSnapshot) *API {
	return &API{db: client, queries: queries, runtime: runtime, bus: bus, defaults: defaults}
}

// q 返回当前上下文可用的 Queries；事务内自动切到 WithTx。
func (a *API) q(ctx context.Context) *sqlite.Queries {
	if a.queries == nil {
		return nil
	}
	if tx, ok := db.TxFromContext(ctx); ok {
		return a.queries.WithTx(tx)
	}
	return a.queries
}

// writeJSON 以 JSON 写出 HTTP 响应。
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

// writeError 按领域错误类型映射状态码并写出 {"error":...}。
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case cderr.IsNotFound(err):
		status = http.StatusNotFound
	case cderr.IsConflict(err):
		status = http.StatusConflict
	case cderr.IsInvalid(err):
		status = http.StatusBadRequest
	case cderr.IsUnauthorized(err):
		status = http.StatusUnauthorized
	case cderr.IsUnavailable(err):
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// decodeJSON 解析请求体；空 body 视为成功。
func decodeJSON(r *http.Request, dest any) error {
	if r.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dest); err != nil && err != io.EOF {
		return cderr.Invalid("%s", err.Error())
	}
	return nil
}
