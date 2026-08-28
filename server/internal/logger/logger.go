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

// newHandler 创建带颜色的终端 slog Handler。
func newHandler(level string) slog.Handler {
	return tint.NewHandler(os.Stderr, &tint.Options{
		Level:      parseLevel(level),
		TimeFormat: "15:04:05.000",
		NoColor:    !isTerminal(os.Stderr),
	})
}

// Init 初始化全局 slog。level 为 debug、info、warn 或 error。
func Init(level string) {
	slog.SetDefault(slog.New(newHandler(level)))
}

// NewLogger 创建带 component 前缀的 slog logger。
func NewLogger(component string) *slog.Logger {
	return slog.Default().With("component", component)
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

// parseLevel 把配置字符串解析成 slog 级别，未知值按 debug。
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
