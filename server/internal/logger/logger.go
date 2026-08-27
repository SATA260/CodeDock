package logger

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/lmittmann/tint"
)

// isTerminal 判断文件描述符是否连接到终端，避免重定向到文件时输出 ANSI 颜色。
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func newHandler() slog.Handler {
	return tint.NewHandler(os.Stderr, &tint.Options{
		Level:      parseLevel(os.Getenv("LOG_LEVEL")),
		TimeFormat: "15:04:05.000",
		NoColor:    !isTerminal(os.Stderr),
	})
}

// Init 初始化全局 slog。读取 LOG_LEVEL（debug、info、warn、error），默认 debug。
func Init() {
	slog.SetDefault(slog.New(newHandler()))
}

// NewLogger 创建带 component 前缀的 slog logger。
func NewLogger(component string) *slog.Logger {
	return slog.New(newHandler()).With("component", component)
}

// RequestAttrs 提取 request_id，供 handler 与访问日志使用相同的观察字段。
func RequestAttrs(r *http.Request) []any {
	attrs := make([]any, 0, 2)
	if requestID := chimw.GetReqID(r.Context()); requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	return attrs
}

// FromRequest 返回带请求字段的 logger，没有 request_id 时回退到全局 slog。
func FromRequest(r *http.Request) *slog.Logger {
	return slog.Default().With(RequestAttrs(r)...)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
