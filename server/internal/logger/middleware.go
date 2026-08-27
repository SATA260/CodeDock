package logger

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Middleware 记录请求方法、路径、状态、耗时和 request_id。
func Middleware(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqLog := log.With(RequestAttrs(r)...)
			writer := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(writer, r)

			reqLog.Info("request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", writer.Status(),
				"duration", time.Since(start),
			)
		})
	}
}
