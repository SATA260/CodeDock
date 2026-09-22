package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"codedock/internal/agent"
	"codedock/internal/board"
	intcodex "codedock/internal/codex"
	"codedock/internal/config"
	cderr "codedock/internal/errors"
	"codedock/internal/events"
	"codedock/internal/logger"
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
	cfg      config.Config
	log      *slog.Logger
	board    *board.Service
	codex    *intcodex.Runtime
}

// New 创建 Handler 入口。log 为 nil 时回退到 slog.Default。
func New(client db.Client, queries *sqlite.Queries, runtime *agent.Runtime, bus *events.Bus, defaults pkgagent.RunConfigSnapshot, cfg config.Config, log *slog.Logger) *API {
	if log == nil {
		log = slog.Default()
	}
	api := &API{db: client, queries: queries, runtime: runtime, bus: bus, defaults: defaults, cfg: cfg, log: log}
	api.refreshBoard()
	return api
}

// SetCodex 注入本机 Codex 运行时，供看板 Inbox / StartInDir 转给已有裁决。
func (a *API) SetCodex(rt *intcodex.Runtime) {
	if a == nil {
		return
	}
	a.codex = rt
	a.refreshBoard()
}

// refreshBoard 按当前依赖重装看板服务。
func (a *API) refreshBoard() {
	if a == nil {
		return
	}
	a.board = board.New(a.queries, a.boardPorts())
}

// logger 返回 Handler 日志；API 或字段为空时回退到 slog.Default。
func (a *API) logger() *slog.Logger {
	if a == nil || a.log == nil {
		return slog.Default()
	}
	return a.log
}

// requestLog 返回带 request_id 的请求日志。
func (a *API) requestLog(r *http.Request) *slog.Logger {
	return a.logger().With(logger.RequestAttrs(r)...)
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

// publishEvent 把已落库的 AgentEvent 发到进程内总线，供 SSE 订阅。
func (a *API) publishEvent(ev pkgagent.AgentEvent) {
	if a == nil || a.bus == nil || ev.EventID == "" {
		return
	}
	ev.Payload = redactJSONSecrets(ev.Payload)
	a.bus.Publish(events.Event{
		Type:          string(ev.Type),
		ChatSessionID: ev.SessionID,
		Payload:       ev,
	})
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
	if err == nil || isClientGone(err) {
		return
	}
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
	if status >= http.StatusInternalServerError {
		slog.Default().Error("handler error", "status", status, "error", err)
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func isClientGone(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded")
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
